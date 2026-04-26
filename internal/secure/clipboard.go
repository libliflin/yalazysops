package secure

import (
	"time"

	"github.com/atotto/clipboard"
)

// copyToClipboard writes the buffer to the system clipboard and schedules a
// conditional clear. The conditional check matters: if the user copied
// something else (a URL, a snippet of code, anything) between our write and
// the timer firing, blindly clearing would destroy their unrelated copy. We
// only clear when the clipboard still holds the value we put there.
func copyToClipboard(b *Buffer, autoClearAfter time.Duration) error {
	// Snapshot as a string here so the goroutine below has something stable
	// to compare against even after the caller wipes their Buffer.
	value := string(b.Bytes())

	if err := clipboard.WriteAll(value); err != nil {
		return err
	}

	go func() {
		time.Sleep(autoClearAfter)

		current, err := clipboard.ReadAll()
		if err != nil {
			// Can't verify what's there — better to leave it alone than to
			// risk clobbering the user's other copy.
			return
		}
		if current != value {
			return
		}
		_ = clipboard.WriteAll("")
	}()

	return nil
}
