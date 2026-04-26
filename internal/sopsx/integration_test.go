package sopsx_test

// End-to-end integration test against a real sops binary and a real age key.
// Skipped if sops or age aren't on $PATH, so unit-test runs stay hermetic.
// CI runs this on Linux + macOS with sops/age installed.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
	"github.com/libliflin/yalazysops/internal/tree"
)

func TestIntegration_EncryptDecryptRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("sops"); err != nil {
		t.Skip("sops not installed")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not installed")
	}

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.txt")
	if out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput(); err != nil {
		t.Fatalf("age-keygen: %v\n%s", err, out)
	}
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	var recipient string
	for _, line := range strings.Split(string(keyBytes), "\n") {
		if strings.HasPrefix(line, "# public key:") {
			recipient = strings.TrimSpace(strings.TrimPrefix(line, "# public key:"))
			break
		}
	}
	if recipient == "" {
		t.Fatalf("no public key in:\n%s", keyBytes)
	}

	plain := filepath.Join(dir, "secrets.yaml")
	if err := os.WriteFile(plain, []byte(`api_key: sk-test-12345
db:
  prod:
    password: hunter2
    host: db.prod.example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}

	enc := filepath.Join(dir, "secrets.enc.yaml")
	cmd := exec.Command("sops", "encrypt", "--age", recipient, plain)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sops encrypt: %v", err)
	}
	if err := os.WriteFile(enc, out, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)
	c := sopsx.New()

	// --- IsSopsFile ---
	ok, err := sopsx.IsSopsFile(enc)
	if err != nil || !ok {
		t.Errorf("IsSopsFile(%s) = %v, %v", enc, ok, err)
	}
	ok, err = sopsx.IsSopsFile(plain)
	if err != nil || ok {
		t.Errorf("IsSopsFile(%s) = %v, %v; want false", plain, ok, err)
	}

	// --- Decrypt + tree.Build ---
	buf, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	root, err := tree.Build(buf.Bytes())
	buf.Wipe()
	if err != nil {
		t.Fatalf("tree.Build: %v", err)
	}
	wantLeaves := map[string]bool{
		"api_key":          false,
		"db.prod.password": false,
		"db.prod.host":     false,
	}
	root.Walk(func(n *tree.Node) {
		if n.IsLeaf() {
			disp := n.Path.Display()
			if _, ok := wantLeaves[disp]; ok {
				wantLeaves[disp] = true
			}
		}
	})
	for k, found := range wantLeaves {
		if !found {
			t.Errorf("missing leaf %q in tree", k)
		}
	}

	// --- Extract a single leaf ---
	apiPath := sopsx.Path{"api_key"}
	leafBuf, err := c.Extract(enc, apiPath)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got := strings.TrimSpace(string(leafBuf.Bytes()))
	leafBuf.Wipe()
	if got != "sk-test-12345" {
		t.Errorf("extracted api_key = %q, want sk-test-12345", got)
	}

	// --- Set a new nested value ---
	newPath := sopsx.Path{"db", "prod", "password"}
	newVal := secure.NewBuffer([]byte("new-rotated-pass"))
	if err := c.Set(enc, newPath, newVal); err != nil {
		t.Fatalf("Set: %v", err)
	}
	leafBuf2, err := c.Extract(enc, newPath)
	if err != nil {
		t.Fatalf("Extract after Set: %v", err)
	}
	got2 := strings.TrimSpace(string(leafBuf2.Bytes()))
	leafBuf2.Wipe()
	if got2 != "new-rotated-pass" {
		t.Errorf("after Set, password = %q, want new-rotated-pass", got2)
	}

	// --- Unset ---
	if err := c.Unset(enc, newPath); err != nil {
		t.Fatalf("Unset: %v", err)
	}
	// After unset, extract should fail.
	if _, err := c.Extract(enc, newPath); err == nil {
		t.Errorf("Extract after Unset succeeded; want error")
	}
}
