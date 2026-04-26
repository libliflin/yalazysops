// Package gitx provides git-history-per-field via `git log` and `git show`.
// It never invokes `git diff` or anything that could expose decrypted
// plaintext: history is just the list of commits that touched the file,
// with historical values fetched on demand by piping `git show <sha>:<file>`
// through `sops decrypt --extract`.
package gitx

import (
	"time"

	"github.com/williamlaffin/yalazysops/internal/secure"
	"github.com/williamlaffin/yalazysops/internal/sopsx"
)

// Commit is a minimal commit record returned by Log.
type Commit struct {
	SHA     string    // 40-char SHA
	Short   string    // 7-char abbrev
	Date    time.Time // author date
	Author  string    // author name
	Subject string    // commit subject (first line)
}

// Log returns the commits that touched file, newest first. Limit caps the
// result count (0 = no limit). The repo root is inferred from the file path.
func Log(file string, limit int) ([]Commit, error) {
	return logFile(file, limit)
}

// LogForKey is like Log but only returns commits where the value at path
// actually changed. We diff the encrypted ENC[...] blocks between commits —
// no decryption needed for the filter — so this is fast and screen-safe.
func LogForKey(file string, path sopsx.Path, limit int) ([]Commit, error) {
	return logForKey(file, path, limit)
}

// ShowAt fetches the value at path from the file as it existed at the given
// commit SHA. Internally: `git show <sha>:<file>` piped to
// `sops decrypt --extract '<path>'`. The plaintext lands in a secure buffer
// that the caller must Wipe().
//
// Decryption requires the caller's current sops keys to have been a
// recipient at that historical commit. No key, no history.
func ShowAt(sopsClient *sopsx.Client, sha, file string, path sopsx.Path) (*secure.Buffer, error) {
	return showAt(sopsClient, sha, file, path)
}
