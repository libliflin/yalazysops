package sopsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/williamlaffin/yalazysops/internal/secure"
)

func TestFormatExtract(t *testing.T) {
	cases := []struct {
		name string
		in   Path
		want string
	}{
		{"empty", Path{}, ""},
		{"single string", Path{"db"}, `["db"]`},
		{"nested strings", Path{"db", "prod", "password"}, `["db"]["prod"]["password"]`},
		{"list index", Path{"hosts", 0}, `["hosts"][0]`},
		{"key with quotes", Path{`weird key with "quotes"`}, `["weird key with \"quotes\""]`},
		{"key with space", Path{"a key", "sub"}, `["a key"]["sub"]`},
		{"multiple indexes", Path{"matrix", 1, 2}, `["matrix"][1][2]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatExtract(tc.in)
			if got != tc.want {
				t.Errorf("formatExtract(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatDisplay(t *testing.T) {
	cases := []struct {
		name string
		in   Path
		want string
	}{
		{"empty", Path{}, "(root)"},
		{"single string", Path{"db"}, "db"},
		{"nested strings", Path{"db", "prod", "password"}, "db.prod.password"},
		{"list index", Path{"hosts", 0}, "hosts[0]"},
		{"trailing key after index", Path{"matrix", 1, "name"}, "matrix[1].name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatDisplay(tc.in)
			if got != tc.want {
				t.Errorf("formatDisplay(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPathsEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b Path
		want bool
	}{
		{"both empty", Path{}, Path{}, true},
		{"identical", Path{"a", 0, "b"}, Path{"a", 0, "b"}, true},
		{"differ length", Path{"a"}, Path{"a", "b"}, false},
		{"differ string", Path{"a"}, Path{"b"}, false},
		{"differ int", Path{"a", 0}, Path{"a", 1}, false},
		{"mixed types same position", Path{"a", 0}, Path{"a", "0"}, false},
		{"int vs string different", Path{0}, Path{"0"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathsEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("pathsEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// fakeSops writes a shell script to dir that:
//   - records its argv (one per line) to <dir>/args.log
//   - records its stdin to <dir>/stdin.log
//   - prints stdoutBody to stdout
//   - exits with exitCode
//
// It returns the path to the script. The caller sets Client.Bin to this path.
func fakeSops(t *testing.T, dir, stdoutBody string, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary tests use a POSIX shell script")
	}
	script := filepath.Join(dir, "fake-sops.sh")
	argsLog := filepath.Join(dir, "args.log")
	stdinLog := filepath.Join(dir, "stdin.log")
	// Record argv (one arg per line) and stdin verbatim, then emit canned
	// stdout. printf %s avoids a trailing newline that 'echo' would add.
	body := `#!/bin/sh
: > "` + argsLog + `"
for a in "$@"; do
  printf '%s\n' "$a" >> "` + argsLog + `"
done
cat > "` + stdinLog + `"
printf '%s' '` + strings.ReplaceAll(stdoutBody, `'`, `'\''`) + `'
exit ` + strconv.Itoa(exitCode) + `
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}
	return script
}

func readArgs(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "args.log"))
	if err != nil {
		t.Fatalf("read args.log: %v", err)
	}
	s := strings.TrimRight(string(data), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func readStdin(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "stdin.log"))
	if err != nil {
		t.Fatalf("read stdin.log: %v", err)
	}
	return string(data)
}

func TestDecrypt(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSops(t, dir, "plaintext-doc-body", 0)
	c := &Client{Bin: bin}

	buf, err := c.Decrypt("secrets.yaml")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	defer buf.Wipe()

	if got := string(buf.Bytes()); got != "plaintext-doc-body" {
		t.Errorf("buffer = %q, want %q", got, "plaintext-doc-body")
	}
	args := readArgs(t, dir)
	want := []string{"decrypt", "secrets.yaml"}
	if !equalSlices(args, want) {
		t.Errorf("argv = %v, want %v", args, want)
	}
}

func TestDecryptError(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSops(t, dir, "", 1)
	c := &Client{Bin: bin}

	if _, err := c.Decrypt("secrets.yaml"); err == nil {
		t.Fatal("expected error on non-zero exit, got nil")
	} else if !strings.Contains(err.Error(), "secrets.yaml") {
		t.Errorf("error should mention file path: %v", err)
	}
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSops(t, dir, "secret-value", 0)
	c := &Client{Bin: bin}

	buf, err := c.Extract("secrets.yaml", Path{"db", "prod", "password"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	defer buf.Wipe()

	if got := string(buf.Bytes()); got != "secret-value" {
		t.Errorf("buffer = %q, want %q", got, "secret-value")
	}
	args := readArgs(t, dir)
	want := []string{
		"decrypt",
		"--extract",
		`["db"]["prod"]["password"]`,
		"secrets.yaml",
	}
	if !equalSlices(args, want) {
		t.Errorf("argv = %v, want %v", args, want)
	}
}

func TestSetValueStdin(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSops(t, dir, "", 0)
	c := &Client{Bin: bin}

	value := secure.NewBuffer([]byte("new-secret"))
	defer value.Wipe()

	if err := c.Set("secrets.yaml", Path{"db", "prod", "password"}, value); err != nil {
		t.Fatalf("Set: %v", err)
	}

	args := readArgs(t, dir)
	want := []string{
		"set",
		"--value-stdin",
		"secrets.yaml",
		`["db"]["prod"]["password"]`,
	}
	if !equalSlices(args, want) {
		t.Errorf("argv = %v, want %v", args, want)
	}
	// The plaintext must arrive via stdin, JSON-encoded.
	stdin := readStdin(t, dir)
	if stdin != `"new-secret"` {
		t.Errorf("stdin = %q, want %q", stdin, `"new-secret"`)
	}
	// Sanity: argv must NOT contain the plaintext.
	for _, a := range args {
		if strings.Contains(a, "new-secret") {
			t.Errorf("plaintext leaked into argv: %v", args)
		}
	}
}

func TestSetValueJSONEscapes(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSops(t, dir, "", 0)
	c := &Client{Bin: bin}

	value := secure.NewBuffer([]byte(`a "tricky" value` + "\n"))
	defer value.Wipe()

	if err := c.Set("secrets.yaml", Path{"k"}, value); err != nil {
		t.Fatalf("Set: %v", err)
	}
	stdin := readStdin(t, dir)
	want := `"a \"tricky\" value\n"`
	if stdin != want {
		t.Errorf("stdin = %q, want %q", stdin, want)
	}
}

func TestUnset(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSops(t, dir, "", 0)
	c := &Client{Bin: bin}

	if err := c.Unset("secrets.yaml", Path{"hosts", 0}); err != nil {
		t.Fatalf("Unset: %v", err)
	}
	args := readArgs(t, dir)
	want := []string{"unset", "secrets.yaml", `["hosts"][0]`}
	if !equalSlices(args, want) {
		t.Errorf("argv = %v, want %v", args, want)
	}
}

func TestIsSopsFile(t *testing.T) {
	dir := t.TempDir()

	yamlSops := filepath.Join(dir, "enc.yaml")
	if err := os.WriteFile(yamlSops, []byte(`db:
  password: ENC[AES256_GCM,data:abc,iv:xxx,tag:yyy,type:str]
sops:
  kms: []
  age:
    - recipient: age1xxx
      enc: ENC...
  lastmodified: "2026-01-01T00:00:00Z"
  mac: ENC[AES256_GCM,...]
  version: 3.8.0
`), 0o644); err != nil {
		t.Fatal(err)
	}

	yamlPlain := filepath.Join(dir, "plain.yaml")
	if err := os.WriteFile(yamlPlain, []byte(`db:
  password: hunter2
note: "the literal text 'sops:' should not trip detection"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	jsonSops := filepath.Join(dir, "enc.json")
	if err := os.WriteFile(jsonSops, []byte(`{
  "db": {"password": "ENC[AES256_GCM,...]"},
  "sops": {"version": "3.8.0"}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	jsonPlain := filepath.Join(dir, "plain.json")
	if err := os.WriteFile(jsonPlain, []byte(`{"db": {"password": "hunter2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// "sops:" appears, but only as a string value — must not match.
	stringValTrap := filepath.Join(dir, "trap.yaml")
	if err := os.WriteFile(stringValTrap, []byte(`note: "sops: not really"
other: 1
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		file string
		want bool
	}{
		{yamlSops, true},
		{yamlPlain, false},
		{jsonSops, true},
		{jsonPlain, false},
		{stringValTrap, false},
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.file), func(t *testing.T) {
			got, err := IsSopsFile(tc.file)
			if err != nil {
				t.Fatalf("IsSopsFile: %v", err)
			}
			if got != tc.want {
				t.Errorf("IsSopsFile(%s) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}

func TestIsSopsFileMissing(t *testing.T) {
	if _, err := IsSopsFile("/no/such/path/here.yaml"); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
