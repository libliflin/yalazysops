# Contributing to yalazysops

Thanks for your interest. yalazysops is a small tool with a tight scope —
contributions that match the spec in [README.md](README.md) are very welcome.
Contributions that expand the scope ("add a web UI", "add cloud sync") will
be politely declined; see the **Anti-features** section of the README.

## Prerequisites

- **Go 1.22+** — `go version`
- **sops 3.8+** — yalazysops shells out to `sops`; integration tests need it on `PATH`
- **git** — required at runtime for the per-field history feature
- **age** — needed to build local fixtures without a cloud KMS
- **golangci-lint** — `brew install golangci-lint` or see [installation docs](https://golangci-lint.run/welcome/install/)

## Building and running

```sh
make build          # → ./yls
make run FILE=path/to/your.enc.yaml
```

There is no fixture in `testdata/` — the repo deliberately ships with no
encrypted file, since any committed fixture is decryptable by anyone with
the matching key. Build your own throwaway fixture with the recipe below.

## Testing

```sh
make test           # go test -race ./...
make lint           # golangci-lint run
```

Tests are split into three layers:

1. **Unit tests** in every package — pure logic, no I/O. Run in milliseconds.
2. **TUI tests** in [`internal/tui/tui_test.go`](internal/tui/tui_test.go).
   The `Model` depends on narrow `sopsBackend` / `gitBackend` interfaces and
   an injected `clipboardFunc`, so tests use hand-rolled fakes — no real
   sops, git, or system clipboard required. Direct `Update` calls cover the
   state machine; one [`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest)
   integration test drives the rendered output through a fake terminal.
3. **End-to-end integration test** in
   [`internal/sopsx/integration_test.go`](internal/sopsx/integration_test.go).
   Generates a real age key, encrypts a fixture with the real `sops`
   binary, then decrypts/extracts/sets/unsets through our wrapper.
   **Auto-skips silently** if `sops` or `age-keygen` aren't on `PATH` — so
   `make test` may report success while skipping the most important test.
   Confirm both binaries are installed before relying on a green run.

CI runs all three on Linux + macOS with sops installed.

## Creating a local fixture

`testdata/` should not contain any real secrets — and ideally no encrypted
file at all, since the matching key would have to live somewhere too. Build
fixtures locally and ignore them via `.gitignore` patterns (already covers
`*.dec.yaml`, `*.dec.json`).

```sh
# 1. Generate a throwaway age key. Treat this like any other secret —
#    do NOT commit /tmp/yls-test.age.
age-keygen -o /tmp/yls-test.age
PUB="$(grep -o 'age1[a-z0-9]*' /tmp/yls-test.age | head -1)"

# 2. Author a plaintext fixture.
cat > /tmp/example.yaml <<'EOF'
anthropic_api_key: sk-ant-FAKE000000000000000000000000
postgres_password: hunter2
auth_secret: not-a-real-secret
db:
  prod:
    host: db.prod.example.com
    password: prodpass-rotate-me
EOF

# 3. Encrypt to a path outside the repo (or into testdata/ if you've added
#    a .gitignore line for it).
sops -e --age "$PUB" /tmp/example.yaml > /tmp/example.enc.yaml

# 4. Run yls against it.
SOPS_AGE_KEY_FILE=/tmp/yls-test.age make run FILE=/tmp/example.enc.yaml
```

## Project layout

```
cmd/yls/            entrypoint — flag parsing, exit codes, version stamp
internal/tui/       Bubbletea models, views, keybindings, test fakes
internal/sopsx/     thin wrapper around the `sops` binary (decrypt, extract, set, unset)
internal/secure/    *Buffer holding plaintext bytes; Wipe() overwrites with crypto/rand;
                    clipboard helper with conditional auto-clear
internal/gitx/      `git log` / `git show` invocations for per-field history
internal/tree/      parses a *decrypted* YAML/JSON document into a navigable
                    Node tree with sha256 fingerprint prefixes on leaves
```

The boundaries are deliberate. `internal/secure.Buffer` is the only type
that holds plaintext bytes; every other package treats it as opaque and
must `defer buf.Wipe()` the moment one is acquired. `internal/tui` never
touches plaintext directly — it dispatches sops/git work as `tea.Cmd`s
that return secure buffers.

## Pull request guidelines

- **Include a test.** Even a minimal table-driven test against `internal/...`
  is more valuable than no test. If a change is genuinely untestable, say
  so in the PR description.
- **Run `make lint test` before pushing.** CI runs the same commands.
- **Keep PRs focused.** Smaller diffs land faster.
- **Don't introduce a new path where plaintext could reach stdout, stderr,
  scrollback, a temp file, or a log.** This is the whole product. The PR
  template asks you to confirm this explicitly.
- **Sign-off is optional** but appreciated (`git commit -s`).

## AI-assisted contributions

This repo includes [`CLAUDE.md`](CLAUDE.md) (also accessible as
`AGENTS.md`) with operational notes for AI coding assistants — build/test
commands, the screen-safety invariants, the testing seam, and gotchas
specific to this codebase. If you're using Claude Code, Cursor, Codex, or
similar, point your assistant at it. Human contributors are welcome to
read it too; it's largely a condensed contributor guide.

## Security

If you find a vulnerability — *especially* a path where plaintext leaks
somewhere we said it wouldn't — please report it privately rather than
opening a public issue. Two channels:

1. **GitHub Security Advisories** — preferred. Use the "Report a vulnerability"
   button on the repository's Security tab.
2. **Email** — `william.laffin@gmail.com`.

See [SECURITY.md](SECURITY.md) for the threat model and what counts as a
vulnerability.
