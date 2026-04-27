# Security

## Threat model

yalazysops exists to defend against one specific, common, and embarrassing
failure mode: **plaintext secrets appearing on screen during demos, pair
programming, or screen-shares** when an engineer opens a sops-encrypted file
to rotate a single value. Within that scope, we work to ensure decrypted
bytes never reach stdout, stderr, terminal scrollback, `$EDITOR` temp files,
swap, shell history, or terminal multiplexer logs — they live only in
process memory and the (auto-clearing) system clipboard.

yalazysops does **not** defend against: local malware running as your user,
kernel-level attackers, OS swap-to-disk of process memory before our zeroize
runs, hardware keyloggers, compromised sops backends, compromised clipboard
managers, or anyone with shell access to your machine. It is not a
replacement for a managed secret platform with proper audit logs and
rotation workflows; if you have one, use it.

Memory hygiene is **best-effort**. Go is a garbage-collected language with a
moving runtime; we overwrite our own buffers with `crypto/rand` bytes after
each operation, but we cannot guarantee the runtime never copies plaintext
to a region we don't control. If your threat model requires guaranteed
memory wiping, use a tool written against `mlock` / `mprotect` primitives in
a non-GC language.

## Things to know that aren't bugs

A few user-side patterns can defeat the screen-safety guarantee even though
yalazysops is doing its job correctly. Worth knowing:

- **Pasting after copy.** `y` puts a value on your system clipboard with a
  30-second auto-clear. If you paste into a window that's also being
  screen-shared (or whose contents are being observed) before the
  auto-clear fires, the value lands on the screen. yalazysops can't see
  what you do with the clipboard. Treat copy as "the secret is now in
  flight" until either the auto-clear runs or you've pasted into a
  trusted destination.
- **Inline environment variables in shell history.** Running
  `SOPS_AGE_KEY_FILE=/path/to/key yls file.enc.yaml` puts the **path** to
  your age key in your shell history. The key contents stay safe, but the
  path is a useful reconnaissance signal for someone with later shell
  access. Prefer placing your key at sops's default location
  (`~/.config/sops/age/keys.txt`) so you can run `yls file.enc.yaml`
  without inline env vars.
- **Terminal scrollback for sops/git stderr.** When sops or git fails,
  yalazysops surfaces the **error message** (not the input value) in the
  status line. Error messages shouldn't contain plaintext, but if you find
  a case where they do, that **is** an in-scope report.

## Reporting a vulnerability

Please do **not** open a public issue for security reports.

- **Preferred:** GitHub Security Advisories — use the "Report a
  vulnerability" button on the repository's Security tab.
- **Email:** `william.laffin@gmail.com`.

Please include enough detail to reproduce, and redact any real secret
values from the report. We will acknowledge receipt within a few days and
work with you on a fix and disclosure timeline.

Examples of in-scope reports:

- A code path that writes plaintext to stdout, stderr, a log, a temp file,
  or any persistent location.
- Plaintext leaking via crash dumps, panics, or error wrapping.
- Clipboard content not being cleared on the configured timeout.
- Git-history-per-field decrypting a value the current keyring should not
  have access to.

Examples of out-of-scope reports:

- "An attacker with shell access can read process memory" — yes, that is
  outside our threat model.
- "sops itself has a bug" — please report to upstream sops.
- Generic dependency CVEs without a concrete exploit path through
  yalazysops.
