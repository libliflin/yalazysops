package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/libliflin/yalazysops/internal/sopsx"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

// initRepo creates a fresh git repo in a temp dir and returns its path.
// It configures user.name/user.email locally so commits work without depending
// on the runner's global git config.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLogFile_NewestFirstAndFields(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)
	file := filepath.Join(repo, "secrets.yaml")

	writeFile(t, file, "v: 1\n")
	runGit(t, repo, "add", "secrets.yaml")
	runGit(t, repo, "commit", "-q", "-m", "first commit")

	writeFile(t, file, "v: 2\n")
	runGit(t, repo, "commit", "-q", "-am", "second commit")

	writeFile(t, file, "v: 3\n")
	runGit(t, repo, "commit", "-q", "-am", "third commit")

	commits, err := logFile(file, 0)
	if err != nil {
		t.Fatalf("logFile: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("want 3 commits, got %d: %#v", len(commits), commits)
	}
	if commits[0].Subject != "third commit" {
		t.Errorf("want newest-first, got top subject %q", commits[0].Subject)
	}
	if commits[2].Subject != "first commit" {
		t.Errorf("want oldest last, got bottom subject %q", commits[2].Subject)
	}
	for i, c := range commits {
		if len(c.SHA) != 40 {
			t.Errorf("commit %d: SHA %q not 40 chars", i, c.SHA)
		}
		if len(c.Short) < 4 || len(c.Short) > 40 {
			t.Errorf("commit %d: Short %q unexpected length", i, c.Short)
		}
		if c.Author != "Test User" {
			t.Errorf("commit %d: Author = %q, want Test User", i, c.Author)
		}
		if c.Date.IsZero() {
			t.Errorf("commit %d: Date is zero", i)
		}
		if c.Subject == "" {
			t.Errorf("commit %d: empty Subject", i)
		}
	}
}

func TestLogFile_LimitFlag(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)
	file := filepath.Join(repo, "secrets.yaml")

	for i := 1; i <= 4; i++ {
		writeFile(t, file, "v: "+string(rune('0'+i))+"\n")
		runGit(t, repo, "add", "secrets.yaml")
		runGit(t, repo, "commit", "-q", "-m", "commit "+string(rune('0'+i)))
	}

	commits, err := logFile(file, 2)
	if err != nil {
		t.Fatalf("logFile: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("want 2 commits with limit=2, got %d", len(commits))
	}
}

func TestLogFile_UntrackedFileReturnsEmpty(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)

	// Need at least one commit so HEAD exists; otherwise `git log` errors with
	// "does not have any commits yet" rather than just an empty result. That
	// matches real-world: an untracked file in an established repo.
	seed := filepath.Join(repo, "seed")
	writeFile(t, seed, "x")
	runGit(t, repo, "add", "seed")
	runGit(t, repo, "commit", "-q", "-m", "seed")

	fresh := filepath.Join(repo, "brand-new.yaml")
	writeFile(t, fresh, "v: 1\n")

	commits, err := logFile(fresh, 0)
	if err != nil {
		t.Fatalf("logFile on untracked file: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("want 0 commits for untracked file, got %d", len(commits))
	}
}

func TestLogFile_NotInRepo(t *testing.T) {
	requireGit(t)
	// t.TempDir is fine — TempDir is a fresh directory; we just don't init git.
	dir := t.TempDir()
	file := filepath.Join(dir, "loose.yaml")
	writeFile(t, file, "v: 1\n")

	_, err := logFile(file, 0)
	if err == nil {
		t.Fatalf("want error for file outside any repo, got nil")
	}
	if !errors.Is(err, ErrNotInRepo) {
		// Some systems may have a parent dir as a repo; in CI/tmp this should
		// not happen, but if it does the error will not be ErrNotInRepo. Skip
		// in that unlikely case to keep the test useful elsewhere.
		t.Skipf("tempdir appears to live inside a git repo: %v", err)
	}
}

func TestLogForKey_DelegatesToLogFile(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)
	file := filepath.Join(repo, "secrets.yaml")
	writeFile(t, file, "v: 1\n")
	runGit(t, repo, "add", "secrets.yaml")
	runGit(t, repo, "commit", "-q", "-m", "only commit")

	commits, err := logForKey(file, sopsx.Path{"v"}, 0)
	if err != nil {
		t.Fatalf("logForKey: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("want 1 commit, got %d", len(commits))
	}
}

func TestRepoRelPath(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(sub, "secrets.yaml")
	writeFile(t, file, "v: 1\n")

	rel, root, err := repoRelPath(file)
	if err != nil {
		t.Fatalf("repoRelPath: %v", err)
	}
	wantRel := "a/b/secrets.yaml"
	if rel != wantRel {
		t.Errorf("rel = %q, want %q", rel, wantRel)
	}
	// Root may be symlink-resolved; the resolved repo dir should match.
	resolvedRepo, _ := filepath.EvalSymlinks(repo)
	if root != resolvedRepo && root != repo {
		t.Errorf("root = %q, want %q (or %q)", root, resolvedRepo, repo)
	}
}

func TestRepoRelPath_NotInRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "loose.yaml")
	writeFile(t, file, "v: 1\n")
	_, _, err := repoRelPath(file)
	if err == nil {
		t.Fatalf("want error, got nil")
	}
	if !errors.Is(err, ErrNotInRepo) {
		t.Skipf("tempdir may live inside a git repo: %v", err)
	}
}

// fakeSops writes a small shell script that mimics `sops decrypt --input-type
// yaml --extract <path> /dev/stdin` enough for showAt to round-trip a value.
// It reads stdin (the encrypted file from `git show`), looks for a literal
// marker line, and prints "PLAINTEXT" on success.
func fakeSops(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake sops uses /bin/sh")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sops")
	script := `#!/bin/sh
# Drain stdin into a tmp file so we can verify the pipeline wired it up.
tmp=$(mktemp)
cat > "$tmp"
if grep -q "ENC_MARKER" "$tmp"; then
  printf "PLAINTEXT_VALUE\n"
  rm -f "$tmp"
  exit 0
fi
rm -f "$tmp"
echo "fake sops: marker not found" 1>&2
exit 1
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sops: %v", err)
	}
	return path
}

func TestShowAt_Pipeline(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)
	file := filepath.Join(repo, "secrets.yaml")
	// Content includes the marker our fake sops grep's for, simulating an
	// ENC[...] block.
	writeFile(t, file, "k: ENC_MARKER\n")
	runGit(t, repo, "add", "secrets.yaml")
	runGit(t, repo, "commit", "-q", "-m", "init")

	sopsBin := fakeSops(t)
	client := &sopsx.Client{Bin: sopsBin}

	buf, err := showAt(client, "HEAD", file, sopsx.Path{"k"})
	if err != nil {
		t.Fatalf("showAt: %v", err)
	}
	defer buf.Wipe()
	got := string(buf.Bytes())
	if got != "PLAINTEXT_VALUE" {
		t.Errorf("plaintext = %q, want %q", got, "PLAINTEXT_VALUE")
	}
}

func TestShowAt_FileMissingAtSha(t *testing.T) {
	requireGit(t)
	repo := initRepo(t)
	seed := filepath.Join(repo, "seed")
	writeFile(t, seed, "x")
	runGit(t, repo, "add", "seed")
	runGit(t, repo, "commit", "-q", "-m", "seed")

	// Reference a file that did not exist at HEAD.
	missing := filepath.Join(repo, "never-tracked.yaml")
	writeFile(t, missing, "k: ENC_MARKER\n")

	sopsBin := fakeSops(t)
	client := &sopsx.Client{Bin: sopsBin}

	_, err := showAt(client, "HEAD", missing, sopsx.Path{"k"})
	if err == nil {
		t.Fatalf("want error for file missing at SHA, got nil")
	}
	// The error should mention git show, not leak any plaintext (there is none
	// here, but we sanity-check the wording).
	if !strings.Contains(err.Error(), "git show") {
		t.Errorf("error = %v, want it to mention git show", err)
	}
}
