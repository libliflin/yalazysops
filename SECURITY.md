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
