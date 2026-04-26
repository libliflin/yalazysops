// Package secure provides best-effort memory hygiene primitives and a
// clipboard helper that auto-clears after a configurable duration.
//
// "Best-effort" because Go's runtime gives us no real guarantees: the GC may
// copy bytes during slice growth, goroutine stacks may persist transient
// values, and the OS may swap pages to disk. We do what we can — hold
// plaintext in []byte (never string), overwrite with crypto/rand the moment
// we're done, and avoid logging or stringifying — and document the rest as a
// known limitation. The bigger win is keeping plaintext out of stdout,
// stderr, scrollback, $EDITOR temp files, and shell history. That part we
// can guarantee.
package secure

import "time"

// Buffer holds plaintext bytes that should be wiped after use. Always defer
// b.Wipe() the moment you create one. Bytes() returns the underlying slice;
// do not retain references after Wipe().
type Buffer struct {
	data []byte
}

// NewBuffer takes ownership of b. The caller must not retain b.
func NewBuffer(b []byte) *Buffer { return &Buffer{data: b} }

// Bytes returns the underlying slice. Valid until Wipe().
func (b *Buffer) Bytes() []byte { return b.data }

// Len returns the buffer length, or 0 if wiped.
func (b *Buffer) Len() int {
	if b == nil {
		return 0
	}
	return len(b.data)
}

// Wipe overwrites the buffer with crypto/rand bytes and zeros the length.
// Safe to call multiple times. Implementation in buffer.go.
func (b *Buffer) Wipe() { wipe(b) }

// CopyToClipboard copies value to the system clipboard and schedules an
// auto-clear after autoClearAfter. The clear is conditional: it only wipes
// the clipboard if it still matches what we set (so we don't clobber
// something the user copied in the meantime). Returns immediately; the
// scheduled clear runs in a goroutine.
//
// The provided buffer is NOT wiped by this function — callers wipe their own
// buffers in the same defer that creates them.
func CopyToClipboard(b *Buffer, autoClearAfter time.Duration) error {
	return copyToClipboard(b, autoClearAfter)
}
