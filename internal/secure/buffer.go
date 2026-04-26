package secure

import (
	"crypto/rand"
)

// wipe overwrites b.data with random bytes before dropping the reference.
// Using crypto/rand instead of zeros makes the wiped state indistinguishable
// from active key material for any code that races with the wipe — there is
// no recognizable "this slot is empty" pattern to scan for.
func wipe(b *Buffer) {
	if b == nil || b.data == nil {
		return
	}
	// Best-effort: rand.Read on a non-empty slice in modern Go does not fail,
	// but if it ever did we still want the reference dropped, so we ignore
	// the error and proceed to nil out the slice.
	_, _ = rand.Read(b.data)
	b.data = nil
}
