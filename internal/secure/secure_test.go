package secure

import (
	"bytes"
	"testing"
	"time"

	"github.com/atotto/clipboard"
)

func TestBufferWipeOverwrites(t *testing.T) {
	original := []byte("super-secret-api-key-do-not-leak")
	snapshot := make([]byte, len(original))
	copy(snapshot, original)

	b := NewBuffer(original)
	b.Wipe()

	// The slice header inside the Buffer is now nil, but the backing array
	// we handed in still exists at `snapshot`'s old address. What we can
	// verify is that the bytes we passed in were overwritten in place
	// before nil-ing — peek at `original` (same underlying array).
	if bytes.Equal(original[:len(snapshot)], snapshot) {
		t.Fatalf("wipe did not overwrite buffer contents")
	}
}

func TestBufferWipeIdempotent(t *testing.T) {
	b := NewBuffer([]byte("hunter2"))
	b.Wipe()
	// Second call must not panic on the already-nil data slice.
	b.Wipe()
}

func TestBufferWipeNil(t *testing.T) {
	var b *Buffer
	// Calling Wipe on a nil receiver routes through the package-level wipe
	// helper, which must tolerate a nil buffer.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wipe on nil buffer panicked: %v", r)
		}
	}()
	b.Wipe()
}

func TestBufferLenAfterWipe(t *testing.T) {
	b := NewBuffer([]byte("0123456789"))
	if got := b.Len(); got != 10 {
		t.Fatalf("Len before wipe = %d, want 10", got)
	}
	b.Wipe()
	if got := b.Len(); got != 0 {
		t.Fatalf("Len after wipe = %d, want 0", got)
	}
}

func TestBufferLenNil(t *testing.T) {
	var b *Buffer
	if got := b.Len(); got != 0 {
		t.Fatalf("Len on nil buffer = %d, want 0", got)
	}
}

// probeClipboard returns true if the system clipboard appears usable in the
// current test environment. Headless CI (no DISPLAY, no pbcopy/wl-copy/xclip)
// makes atotto/clipboard fail; we skip clipboard-dependent tests there.
func probeClipboard(t *testing.T) {
	t.Helper()
	if err := clipboard.WriteAll("yalazysops-clipboard-probe"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
}

func TestCopyToClipboardWritesValue(t *testing.T) {
	probeClipboard(t)
	defer clipboard.WriteAll("")

	b := NewBuffer([]byte("alpha-secret-value"))
	defer b.Wipe()

	if err := CopyToClipboard(b, 50*time.Millisecond); err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}

	got, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("clipboard read failed: %v", err)
	}
	if got != "alpha-secret-value" {
		t.Fatalf("clipboard contents = %q, want %q", got, "alpha-secret-value")
	}
}

func TestCopyToClipboardAutoClears(t *testing.T) {
	probeClipboard(t)
	defer clipboard.WriteAll("")

	b := NewBuffer([]byte("ephemeral-secret"))
	defer b.Wipe()

	if err := CopyToClipboard(b, 25*time.Millisecond); err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}

	// Wait long enough for the scheduled clear to fire.
	time.Sleep(150 * time.Millisecond)

	got, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("clipboard read failed: %v", err)
	}
	if got != "" {
		t.Fatalf("clipboard not cleared: got %q", got)
	}
}

func TestCopyToClipboardConditionalClear(t *testing.T) {
	probeClipboard(t)
	defer clipboard.WriteAll("")

	a := NewBuffer([]byte("value-A-from-yalazysops"))
	defer a.Wipe()

	if err := CopyToClipboard(a, 50*time.Millisecond); err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}

	// Simulate the user copying something else (a URL, a code snippet,
	// whatever) before A's clear timer fires. A's timer must NOT clobber B.
	const userCopied = "value-B-user-copied-this-themselves"
	if err := clipboard.WriteAll(userCopied); err != nil {
		t.Fatalf("setup: writing B failed: %v", err)
	}

	// Wait past A's timer.
	time.Sleep(200 * time.Millisecond)

	got, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("clipboard read failed: %v", err)
	}
	if got != userCopied {
		t.Fatalf("conditional clear clobbered user's copy: got %q, want %q", got, userCopied)
	}
}
