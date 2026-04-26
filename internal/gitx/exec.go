package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
)

// ErrNotInRepo is returned when the file is not inside a git working tree.
// Callers can use this to hide history features rather than surfacing a
// scary error to the user.
var ErrNotInRepo = errors.New("file is not inside a git repository")

func logFile(file string, limit int) ([]Commit, error) {
	dir := filepath.Dir(file)
	args := []string{
		"log",
		"--format=%H%x09%h%x09%aI%x09%an%x09%s",
	}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	args = append(args, "--", filepath.Base(file))

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "not a git repository") {
			return nil, fmt.Errorf("%w: %s", ErrNotInRepo, dir)
		}
		return nil, fmt.Errorf("git log failed: %v: %s", err, strings.TrimSpace(errMsg))
	}

	return parseLog(stdout.Bytes()), nil
}

func parseLog(out []byte) []Commit {
	var commits []Commit
	lines := bytes.Split(out, []byte{'\n'})
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		fields := strings.SplitN(string(line), "\t", 5)
		if len(fields) < 5 {
			continue
		}
		t, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			continue
		}
		commits = append(commits, Commit{
			SHA:     fields[0],
			Short:   fields[1],
			Date:    t,
			Author:  fields[3],
			Subject: fields[4],
		})
	}
	return commits
}

// TODO(v0.5): real per-key filtering. Diff the encrypted ENC[...] block at
// the path between adjacent commits and only emit commits where it changed.
// For now we conservatively return every commit that touched the file —
// callers see a superset of the true per-key history.
func logForKey(file string, _ sopsx.Path, limit int) ([]Commit, error) {
	return logFile(file, limit)
}

func showAt(c *sopsx.Client, sha, file string, path sopsx.Path) (*secure.Buffer, error) {
	relPath, repoRoot, err := repoRelPath(file)
	if err != nil {
		return nil, err
	}

	gitCmd := exec.Command("git", "show", sha+":"+relPath)
	gitCmd.Dir = repoRoot
	var gitErr bytes.Buffer
	gitCmd.Stderr = &gitErr
	gitOut, err := gitCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git show pipe: %w", err)
	}

	bin := c.Bin
	if bin == "" {
		bin = "sops"
	}
	sopsCmd := exec.Command(bin, "decrypt", "--input-type", "yaml", "--extract", path.Extract(), "/dev/stdin")
	sopsCmd.Stdin = gitOut
	var sopsOut bytes.Buffer
	var sopsErr bytes.Buffer
	sopsCmd.Stdout = &sopsOut
	sopsCmd.Stderr = &sopsErr

	if err := gitCmd.Start(); err != nil {
		return nil, fmt.Errorf("git show start: %w", err)
	}
	if err := sopsCmd.Start(); err != nil {
		_ = gitCmd.Process.Kill()
		_, _ = io.Copy(io.Discard, gitOut)
		_ = gitCmd.Wait()
		return nil, fmt.Errorf("sops decrypt start: %w", err)
	}

	gitWaitErr := gitCmd.Wait()
	sopsWaitErr := sopsCmd.Wait()

	if gitWaitErr != nil {
		// Don't include sops output in the error — we already kept it out of
		// stdout/stderr by capturing it. But sopsErr is fine to surface.
		return nil, fmt.Errorf("git show %s:%s: %v: %s", sha, relPath, gitWaitErr, strings.TrimSpace(gitErr.String()))
	}
	if sopsWaitErr != nil {
		return nil, fmt.Errorf("sops decrypt: %v: %s", sopsWaitErr, strings.TrimSpace(sopsErr.String()))
	}

	plaintext := bytes.TrimRight(sopsOut.Bytes(), "\n")
	// Copy out of the bytes.Buffer's internal storage so wiping the secure
	// buffer doesn't leave a copy in the now-discarded buffer.
	owned := make([]byte, len(plaintext))
	copy(owned, plaintext)
	return secure.NewBuffer(owned), nil
}

// repoRelPath returns (relativePath, repoRoot, error). The relative path is
// suitable for `git show <sha>:<relpath>`.
func repoRelPath(file string) (string, string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", "", fmt.Errorf("absolute path: %w", err)
	}
	dir := filepath.Dir(abs)

	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "not a git repository") {
			return "", "", fmt.Errorf("%w: %s", ErrNotInRepo, dir)
		}
		return "", "", fmt.Errorf("git rev-parse: %v: %s", err, strings.TrimSpace(errMsg))
	}
	repoRoot := strings.TrimSpace(stdout.String())
	if repoRoot == "" {
		return "", "", fmt.Errorf("%w: empty toplevel for %s", ErrNotInRepo, dir)
	}

	// Resolve symlinks on the toplevel for matching, since macOS /var vs
	// /private/var commonly differs between filepath.Abs and `git rev-parse`.
	rootResolved, err := filepath.EvalSymlinks(repoRoot)
	if err == nil {
		repoRoot = rootResolved
	}
	absResolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = absResolved
	}

	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return "", "", fmt.Errorf("relativize %s under %s: %w", abs, repoRoot, err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("%w: %s is outside %s", ErrNotInRepo, abs, repoRoot)
	}
	// git wants forward slashes regardless of OS.
	rel = filepath.ToSlash(rel)
	return rel, repoRoot, nil
}
