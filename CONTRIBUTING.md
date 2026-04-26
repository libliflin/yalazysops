# Contributing to yalazysops

Thanks for your interest. yalazysops is a small tool with a tight scope —
contributions that match the spec in [README.md](README.md) are very welcome.
Contributions that expand the scope ("add a web UI", "add cloud sync") will
be politely declined; see the **Anti-features** section of the README.

## Prerequisites

- **Go 1.22+** — `go version`
- **sops 3.8+** — yalazysops shells out to `sops`; tests need it on `PATH`
- **git** — required at runtime for the per-field history feature
- **age** — recommended for testing (lets you create local fixtures without a cloud KMS)
- **golangci-lint** — `brew install golangci-lint` or see [installation docs](https://golangci-lint.run/welcome/install/)

## Running

```sh
make build
./yls testdata/example.enc.yaml
```

Or, during iteration:

```sh
make run FILE=testdata/example.enc.yaml
```

## Creating a test fixture

`testdata/` should not contain any real secrets. Use a throwaway age key to
build local fixtures:

```sh
# 1. Generate a throwaway age key (DO NOT commit the private key).
age-keygen -o /tmp/yls-test.age
PUB="$(age-keygen -y /tmp/yls-test.age)"

# 2. Tell sops which recipient to use for testdata/.
cat > .sops.yaml <<EOF
creation_rules:
  - path_regex: ^testdata/.*\.enc\.(ya?ml|json)$
    age: $PUB
EOF

# 3. Author a plaintext fixture.
cat > /tmp/example.yaml <<'EOF'
anthropic_api_key: sk-ant-FAKE000000000000000000000000
postgres_password: hunter2
auth_secret: not-a-real-secret
EOF

# 4. Encrypt into the repo.
SOPS_AGE_KEY_FILE=/tmp/yls-test.age \
  sops -e /tmp/example.yaml > testdata/example.enc.yaml

# 5. Tests load the matching key via SOPS_AGE_KEY_FILE.
```

The private key (`/tmp/yls-test.age`) is for your local test runs. CI tests
generate their own ephemeral keys; never commit a private key.

## Pull request guidelines

- **Include a test.** Even a minimal table-driven test against `internal/...`
  is more valuable than no test. If a change is genuinely untestable, say so
  in the PR description.
- **Run `make lint test` before pushing.** CI runs the same commands.
- **Keep PRs focused.** Smaller diffs land faster.
- **Sign-off is optional** but appreciated (`git commit -s`).
- **Don't introduce a new path where plaintext could reach stdout, stderr,
  scrollback, a temp file, or a log.** This is the whole product. The PR
  template asks you to confirm this explicitly.

## Project layout

```
cmd/yls/            entrypoint — flag parsing, exit codes, version stamp
internal/tui/       Bubbletea models, views, keybindings
internal/sopsx/     thin wrapper around the `sops` binary (decrypt, encrypt-update)
internal/secure/    in-memory secret buffer with crypto/rand zeroize on drop
internal/gitx/      `git log` / `git show` invocations for per-field history
internal/tree/      parse a sops-encrypted YAML/JSON file into a key list
testdata/           encrypted fixtures + matching test keys (never real secrets)
```

The boundaries are deliberate: `internal/secure` is the only package that
holds plaintext bytes, and `internal/tui` never sees them except through
that package's read-and-overwrite API.

## Security

If you find a vulnerability — *especially* a path where plaintext leaks
somewhere we said it wouldn't — please report it privately rather than
opening a public issue. Two channels:

1. **GitHub Security Advisories** — preferred. Use the "Report a vulnerability"
   button on the repository's Security tab.
2. **Email** — `william.laffin@gmail.com`.

See [SECURITY.md](SECURITY.md) for the threat model and what counts as a
vulnerability.
