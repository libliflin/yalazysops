// Package sopsx is a thin wrapper over the `sops` binary. It never
// re-implements crypto. All operations shell out to sops 3.8+; plaintext
// values are passed via stdin (sops set --value-stdin) so they never appear
// in argv or process listings.
//
// Path syntax follows sops's --extract format: a sequence of bracketed
// JSON-encoded keys/indexes, e.g. ["db"]["prod"]["password"] or ["hosts"][0].
package sopsx

import (
	"github.com/libliflin/yalazysops/internal/secure"
)

// Path is a sequence of map keys (string) and list indexes (int) identifying
// a single value inside a sops document.
type Path []any

// Extract renders the path in sops's bracket syntax: ["a"]["b"][0]. Strings
// are JSON-encoded so quotes/escapes are handled. Empty path returns "".
func (p Path) Extract() string { return formatExtract(p) }

// Display renders the path in human-friendly dotted form: a.b[0].
func (p Path) Display() string { return formatDisplay(p) }

// Equal reports whether two paths are identical.
func (p Path) Equal(other Path) bool { return pathsEqual(p, other) }

// Client is a sops shell-out client. Bin is the path to the sops binary
// (defaults to "sops" — looked up via $PATH).
type Client struct {
	Bin string
}

// New returns a Client using the sops binary on $PATH.
func New() *Client { return &Client{Bin: "sops"} }

// Decrypt runs `sops decrypt file` and returns the full plaintext document
// in a secure buffer. The caller MUST defer buf.Wipe().
func (c *Client) Decrypt(file string) (*secure.Buffer, error) {
	return decrypt(c, file)
}

// Extract runs `sops decrypt --extract '<path>' file` and returns the single
// value's plaintext bytes in a secure buffer. Caller defers Wipe().
func (c *Client) Extract(file string, path Path) (*secure.Buffer, error) {
	return extract(c, file, path)
}

// Set runs `sops set --value-stdin file '<path>'` with value piped through
// stdin. The plaintext never enters argv. Returns nil on success.
//
// sops set creates intermediate map nodes as needed. The value bytes are
// JSON-encoded by this function (so a string "foo" is sent as "\"foo\"") —
// callers pass raw plaintext, the JSON wrapping happens inside.
func (c *Client) Set(file string, path Path, value *secure.Buffer) error {
	return setValue(c, file, path, value)
}

// Unset runs `sops unset file '<path>'` to delete a single key. Other values
// retain their original IVs.
func (c *Client) Unset(file string, path Path) error {
	return unsetValue(c, file, path)
}

// IsSopsFile returns true if the file appears to be a sops-encrypted YAML or
// JSON document (presence of a `sops:` metadata block at the top level). We
// do this by reading the file as plain text — no decryption, no network.
func IsSopsFile(file string) (bool, error) { return isSopsFile(file) }
