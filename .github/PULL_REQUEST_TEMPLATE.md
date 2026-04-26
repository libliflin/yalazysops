<!--
Thanks for the PR. Please fill out the sections below — the screen-safety
check in particular is non-negotiable for this project.
-->

## Summary

<!-- One or two sentences describing what this change does and why. -->

## Screen-safety check

- [ ] This change does **not** introduce any new path where plaintext could
      reach stdout, stderr, terminal scrollback, a temp file, swap, shell
      history, a log file, or a multiplexer buffer.
- [ ] If this change touches `internal/secure`, `internal/sopsx`, or
      `internal/tui` value rendering, I have specifically reviewed those
      paths for accidental leaks.

## Tests

- [ ] Added or updated tests under `internal/...`
- [ ] `make lint test` passes locally

## Notes for the reviewer

<!-- Anything the diff alone won't tell them: design choices, trade-offs,
follow-ups intentionally deferred, screenshots of TUI changes (with values
masked), etc. -->
