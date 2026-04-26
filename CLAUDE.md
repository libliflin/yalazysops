# Notes for AI coding assistants

This file is read by Claude Code, and (via the `AGENTS.md` symlink) by
Codex, Cursor, and other tools that follow the AGENTS.md convention. It
condenses what an assistant needs to know to make safe, on-spec changes
to yalazysops without having to re-derive the constraints from scratch.

The authoritative spec is [README.md](README.md). When this file and the
README disagree, the README wins — and please open a PR fixing the drift.

## What this tool is

A screen-safe TUI for sops-encrypted secret files. The headline invariant:
**plaintext never reaches stdout, stderr, scrollback, `$EDITOR` temp files,
swap files, shell history, or terminal multiplexer logs.** It does reach
the system clipboard (with auto-clear) and process memory (overwritten
after use). Everything else is downstream of that invariant.

## Common commands

```sh
make build          # → ./yls
make test           # go test -race ./...
make lint           # golangci-lint run
make fmt            # gofmt + goimports
make run FILE=...   # iterate against a local fixture
```

There is no checked-in fixture; see CONTRIBUTING.md for the local-fixture
recipe (`age-keygen` + `sops -e`).

## Hard rules — do not violate without explicit user approval

1. **No re-implementing crypto.** All decryption, encryption, and key
   handling shells out to the `sops` binary. The `internal/sopsx` package
   is the only place that calls `sops`; the rest of the code uses its
   public API.
2. **Plaintext lives only inside `*secure.Buffer`.** The moment you call
   anything that returns plaintext (`Decrypt`, `Extract`, `ShowAt`),
   `defer buf.Wipe()` immediately. Never convert to `string` for logging,
   error messages, or comparison outside of explicit test fixtures.
3. **Set values via stdin, never argv.** `sopsx.Client.Set` already does
   this with `sops set --value-stdin`; if you add new write paths,
   preserve the property. Plaintext in argv leaks via `/proc/<pid>/cmdline`
   and `ps`.
4. **Never invoke `$EDITOR`.** Avoiding the editor boundary is the whole
   product. All editing happens inside the TUI's masked input fields.
5. **Don't construct sops bracket paths by hand.** Use `sopsx.Path` and
   its `.Extract()` / `.Display()` methods. Hand-rolled `["a"]["b"]`
   strings will mishandle keys with quotes, escapes, or special chars.
6. **Don't add features past v0.5 without discussion.** The README's
   "Anti-features" section lists deliberate non-goals (browser GUI,
   multi-file orchestration, cloud sync, custom crypto). PRs adding any
   of these will be closed.

## Testing seam

`internal/tui` depends on `sopsBackend`, `gitBackend`, and `clipboardFunc`
interfaces — not the concrete `*sopsx.Client` or `secure.CopyToClipboard`.
This is deliberate so the TUI can be tested without a real terminal,
real sops, real git, or a real clipboard.

When adding a TUI feature:

- **State-machine logic** → direct `Update` tests in `tui_test.go`. Build
  a `Model` via `newModel(file, fakeSops, fakeGit, fakeClipboard.Copy)`,
  send synthesized `tea.KeyMsg` values, assert on returned model state.
- **Rendered-output behavior** → add a `teatest` case. The seed test
  `TestTeatest_OpenExpandCopy` is the template. Use `teatest.WaitFor`
  with substring matches; avoid pinning exact byte sequences (ANSI
  escapes will churn).
- **New shell-out** → add to `sopsx` or `gitx`, write a unit test using
  the fake-binary pattern (a shell script in `t.TempDir()` that records
  argv and stdin), and add an integration test in
  `internal/sopsx/integration_test.go` that exercises the real sops
  binary if applicable. Note that the integration test auto-skips when
  sops or age aren't on PATH.

If a change forces a new dependency in the TUI, prefer adding a method
to `sopsBackend` or `gitBackend` (and the production adapter) over
threading a concrete type through.

## Common gotchas

- **Bubbletea Cmds run on goroutines.** Don't share `Model` state between
  the model and a goroutine — return data via `tea.Msg`. The `fakeSops`
  in tests uses a `sync.Mutex` for the same reason.
- **`teatest` uses `bubbletea.WithoutSignalHandler`.** The model receives
  `tea.WindowSizeMsg` from `WithInitialTermSize`; don't assume sizing
  comes from a real TTY.
- **The integration test auto-skips silently.** Don't claim "tests pass"
  without confirming `sops` and `age-keygen` were on `PATH` for the run.
- **Don't lower the gosec G204 suppression to wider scope.** `internal/sopsx`
  and `internal/gitx` shell out by design. New shell-outs elsewhere
  deserve scrutiny per-call, not a blanket exception.
- **gocyclo threshold is 15.** Refactor a tangled function rather than
  raising the threshold. Split key-dispatch functions into nav vs.
  action helpers (see `handleListNav`/`handleListAction`).
- **`sops` 3.8+ has `set --value-stdin` and `unset`.** Don't fall back to
  the `$EDITOR` flow for delete — `unset` exists.
- **Don't enable goreleaser brew/nix/apt blocks** without first creating
  the corresponding tap repo. They're commented out for a reason; flipping
  them on without infra will break the release pipeline.

## Project layout

```
cmd/yls/                   entrypoint; flag parsing, version stamp, IsSopsFile gate
internal/tui/              Bubbletea Model, views, keybindings, fakes for tests
  ├─ model.go              Model struct + interfaces + Cmd factories + Update dispatch
  ├─ list.go               main list/tree view, key dispatch
  ├─ edit.go               edit + new flows (masked two-step input)
  ├─ history.go            git-history view
  ├─ confirm.go            delete confirmation modal
  ├─ search.go             incremental fuzzy search over key paths
  ├─ input.go              thin wrapper over bubbles/textinput
  ├─ styles.go             lipgloss styles
  └─ tui_test.go           fakes + Update unit tests + teatest seed
internal/sopsx/            thin wrapper around `sops` binary
  ├─ sopsx.go              public API: Client, Path, Decrypt/Extract/Set/Unset
  ├─ exec.go               unexported helpers (formatExtract, exec.Command wrappers)
  ├─ sopsx_test.go         unit tests using fake sops binary
  └─ integration_test.go   real-sops + real-age round-trip; auto-skips
internal/gitx/             `git log` / `git show` for per-field history
internal/secure/           *Buffer with crypto/rand wipe; clipboard with auto-clear
internal/tree/             parses *decrypted* bytes into a navigable Node tree
                           with sha256 fingerprint prefixes on leaves
```

## Style notes

- Comments explain WHY, not what. Code says what. If a comment restates
  the function name, delete it.
- No backwards-compatibility shims. This is pre-1.0; rename, refactor,
  remove freely. The README is the contract.
- No premature abstraction. Three similar lines is better than the wrong
  interface. The TUI testing seam earns its keep because the test fakes
  use it; don't extract more interfaces "in case."
- Memory hygiene is best-effort and we say so. Don't claim guarantees Go's
  runtime can't make. The real win is keeping plaintext out of files,
  scrollback, and argv — that part is enforceable.
- Errors that surface to the UI go through `m.setError`, which routes to
  the status line. They never include plaintext (only sops/git stderr or
  meta-information like file paths).

## When in doubt

Ask. The screen-safety invariant has subtle edges and most users running
this tool will be doing so on a screen-share. A wrong call here is worse
than a slow PR.
