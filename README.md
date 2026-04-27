# yalazysops

[![CI](https://img.shields.io/github/actions/workflow/status/libliflin/yalazysops/ci.yml?branch=main&label=CI)](https://github.com/libliflin/yalazysops/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/libliflin/yalazysops?include_prereleases&sort=semver)](https://github.com/libliflin/yalazysops/releases/latest)
[![License](https://img.shields.io/github/license/libliflin/yalazysops)](LICENSE)

**Yet Another Lazy SOPS** — pronounced *yallah-zee-sops*.

Screen-safe TUI for sops-encrypted secret files. Git history per field. Plaintext
never hits your screen, never hits a temp file. Never leaves home row.

---

## Product Announcement (working backwards)

**FOR IMMEDIATE RELEASE — 2026**

### yalazysops 1.0: secret management for engineers who pair-program over Zoom

A new open-source TUI for sops-encrypted secret files lets engineers view, edit,
rotate, and audit secrets without ever displaying plaintext on screen — solving
the long-standing footgun where running `sops <file>` to rotate a single value
reveals every other secret in the file to whoever's watching the screen-share.

> "Every six months we'd have a near-miss," said an SRE at a fictional startup,
> "where someone rotating a Stripe key on Zoom would have ten other secrets
> glance across our shared screen — Anthropic key, Postgres password, OAuth
> secrets. We knew it was bad. We just had no alternative until yalazysops."

yalazysops opens any sops-encrypted YAML or JSON file as a navigable
tree of keys, with branches that expand and collapse like lazygit's
file panel. Keys are cleartext (sops's design — and a feature, not a
bug). Values are masked. The user navigates with vim keys, copies one
value to the system clipboard with `y`, edits one value with `e` —
which opens a `getpass`-style prompt that masks input. Decrypted bytes
live only in process memory and are overwritten with random bytes the
moment the operation completes.

Three features distinguish yalazysops from existing sops wrappers:

1. **Git history per field.** Cursor on `anthropic_api_key`, press `?`, see every
   commit that ever modified that one value — with the historical values
   decrypted on demand from each commit's blob. No browser tab, no `git log`
   invocation, no leaking other keys.

2. **Content fingerprints.** Each value shows a short SHA-256 prefix in the list
   view. Verify "yes, this is the prod key from 1Password" by matching hashes,
   never by reading the value.

3. **No `$EDITOR` boundary.** sops's native flow writes plaintext to a tempfile
   and trusts your editor to clean up. yalazysops never invokes an external
   editor. All editing happens inside the TUI's own input fields.

Open-source under MIT, works with every sops-supported backend (age, AWS KMS,
GCP KMS, Azure Key Vault, PGP), runs on macOS, Linux, Windows terminals.

---

## Customer FAQ

**Q: Why not just use `sops <file>`?**
Because it opens the *entire* decrypted file in `$EDITOR`. The moment you scroll,
every secret in the file is on your screen. yalazysops reveals one value at a
time, only to your clipboard, never to your screen.

**Q: Why not 1Password / Vault / Doppler?**
yalazysops doesn't replace them. If you're already on a managed secret platform
with proper audit logs and rotation workflows, stay there. yalazysops is for
teams that have chosen sops-in-git (small team, IaC purity, no managed-secret
budget) but still want first-class UX at the terminal.

**Q: How does git-history-per-field work without leaking?**
For a chosen historical commit, yalazysops runs
`git show <sha>:<file> | sops -d --extract '["key"]'`. Decryption requires your
current sops keys to have been a recipient at that historical commit. No key,
no history. The decrypted value goes to the same in-memory buffer and is
overwritten after display.

**Q: Is it really screen-safe? Like really?**
Plaintext flows through:
- Your sops backend's decryption (network or local, depending on backend)
- Process memory (overwritten with `crypto/rand` bytes after use)
- The system clipboard (auto-clears 30s after copy; conditional clear, so a later user copy isn't clobbered)

Never to: stdout, stderr, scrollback, `$EDITOR` temp files, swap files,
shell history, terminal multiplexer logs.

**Q: Does it support search?**
Yes — `/` opens an incremental substring filter over key paths
(`db.prod.password` matches `prod`). Values are never indexed (they're
encrypted at rest and we don't decrypt for indexing).

**Q: How do I add a new secret?**
Move the cursor onto the branch you want to add under (or onto a leaf
inside it), press `n`, type the key name, then enter and confirm the
value at masked prompts. The file is re-encrypted via sops's normal
flow; only the new value's `ENC[...]` block changes. Existing values
keep their original IVs, so git diffs stay clean.

**Q: Multi-recipient files?**
yalazysops respects whatever recipients are listed in your `.sops.yaml`.
Adding/removing recipients is still done via `sops updatekeys`. yalazysops
doesn't try to re-implement key management.

**Q: Why not a desktop GUI?**
Because security-minded engineers live in the terminal and resent
context-switching. The audience that loves lazygit will love this. A desktop
GUI also has to be code-signed per OS, distributed per OS, maintained per OS —
too much work for the value-add.

---

## Customer Walkthrough

The list view is a navigable tree. Branches expand and collapse with
`l`/`h` (or arrow keys); leaves show a sha256 prefix as their value
fingerprint. The bottom bar is anchored to the last row of the terminal —
status messages appear above the help line and auto-clear:

```
$ yls secrets/production.enc.yaml

 secrets/production.enc.yaml

   anthropic_api_key  sha256:7a3f9c2b…
   auth_secret        sha256:9b210e84…
   postgres_password  sha256:2c19ff3a…
   ▾ db  (2)
     ▾ prod  (4)
       host          sha256:c5f2aa01…
▸      password      sha256:88d4ee99…
       port          sha256:e1a55d7c…
       ssl_mode      sha256:6f08bb22…
     ▸ dev  (4)
   ▸ trusted_origins  (3)




 ✓ Copied db.prod.password. Auto-clear in 30s.
 y copy   e edit   n new   d delete   ? history   / search   q quit
```

`?` on a leaf opens git history for that single value. Pressing `y` on a
historical commit copies the value as it existed at that commit (sops
re-decrypts the historical blob; your current keys must have been a
recipient at that commit):

```
 History: db.prod.password

   ed25322  2026-04-22  William L.  rotated after exposure
▸  c73aabd  2026-04-13  William L.  initial value

 y/enter copy historical value   q/esc back
```

`e` opens a masked two-step confirm. Plaintext only exists inside the
input field — never echoed, never written to a temp file:

```
 Edit db.prod.password
╭───────────────────────────────────────────────╮
│ Current: (masked)                             │
│                                               │
│ Enter new value: ●●●●●●●●●●●●●●●●●●●          │
│                                               │
│ [enter] save   [esc] cancel                   │
╰───────────────────────────────────────────────╯
```

After save, the file is re-encrypted in place via `sops set --value-stdin`
(the new value never enters argv) and the status line confirms:

```
 ✓ Updated db.prod.password in production.enc.yaml
```

`git diff` after the edit shows one `ENC[...]` blob change plus the
`lastmodified` and `mac` metadata — every other value keeps its original IV.

---

## Anti-features (deliberately not built)

- **Browser/Electron GUI** — defeats the point.
- **Built-in key management** — `sops updatekeys` exists.
- **Multi-file orchestration** — one file at a time. Use `make` for cross-file workflows.
- **Custom crypto** — sops handles every byte. yalazysops never touches AES/age/KMS directly.
- **Cloud sync** — your secrets live in git. Period.

---

## Architecture

| Component        | Implementation                                                                                |
|------------------|-----------------------------------------------------------------------------------------------|
| TUI              | [Bubbletea](https://github.com/charmbracelet/bubbletea) (Go) — same framework as lazygit, glow, k9s |
| Sops integration | Shell out to the `sops` binary (3.8+). Writes use `sops set --value-stdin` so plaintext never enters argv. Never re-implement crypto. |
| Memory hygiene   | Decrypted bytes in a `secure.Buffer`; overwritten with `crypto/rand` after each operation. Best-effort under Go's GC; see [SECURITY.md](SECURITY.md). |
| Git history      | `git log` for commits touching the file, then `git show <sha>:<file>` piped to `sops decrypt --extract '["key"]'` for the historical value. |
| Clipboard        | `pbcopy` / `wl-copy` / `xclip` — 30s auto-clear by default. Conditional clear: only wipes the clipboard if it still matches what we wrote, so a later user copy survives. |
| Fingerprint      | First 8 bytes (16 hex chars) of SHA-256 of the decrypted value, in-memory.                    |
| Masked input     | Bubbletea `textinput` with `EchoMode: EchoPassword`.                                          |

Cross-platform: macOS, Linux, and Windows binaries built per release;
all three OSes covered in CI.

---

## Naming

`ya*` — 1970s unix tradition. yacc (Yet Another Compiler-Compiler), YAML
(originally Yet Another Markup Language), yagni. Signals "I know there are
several of these already, here's mine, honestly."

`lazy*` — 2018 sub-genre of TUI tools kicked off by
[lazygit](https://github.com/jesseduffield/lazygit). Fast keyboard-driven UIs
over CLI tools. Followed by lazydocker, lazynpm, lazysql, lazyk8s.

`yalazysops` is both. The audience under 30 recognizes `lazy*`; the audience
over 35 recognizes `ya*`. The compound name catches both eras of unix culture
by accident, and the binary is `yls` so your fingers don't type 11 chars
every time.

Pronounced *yallah-zee-sops*. (*Yallah* — Arabic/Hebrew for "let's go" — is a
serendipitously appropriate imperative for a CLI.)

---

## Status

- [x] v0.1 — design doc + README
- [x] v0.2 — read-only TUI: list, copy, fingerprint
- [x] v0.3 — edit / new / delete with masked prompts
- [x] v0.4 — git history per field
- [~] v0.5 — search done; undo deferred
- [ ] v1.0 — `brew` / `apt` / `nix` packaging (binaries shipped per release; tap setup pending)

The current shipped release is on the [Releases page](https://github.com/libliflin/yalazysops/releases/latest).
Pre-built binaries for darwin/linux/windows × amd64/arm64 are attached
to each release.

## License

MIT — see [LICENSE](LICENSE).
