// Command yls is the yalazysops TUI: a screen-safe browser for
// sops-encrypted secret files.
//
// Usage:
//
//	yls path/to/file.enc.yaml
//
// See README.md for the full design doc.
package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libliflin/yalazysops/internal/sopsx"
	"github.com/libliflin/yalazysops/internal/tui"
)

// Set by goreleaser via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("yls %s (%s, built %s)\n", version, commit, date)
			return
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}

	if len(os.Args) != 2 {
		printUsage()
		os.Exit(2)
	}
	file := os.Args[1]

	if _, err := os.Stat(file); err != nil {
		fmt.Fprintf(os.Stderr, "yls: %v\n", err)
		os.Exit(1)
	}
	ok, err := sopsx.IsSopsFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "yls: %v\n", err)
		os.Exit(1)
	}
	if !ok {
		fmt.Fprintf(os.Stderr,
			"yls: %s does not look like a sops-encrypted file (no top-level `sops:` block)\n",
			file)
		os.Exit(1)
	}

	client := sopsx.New()
	model := tui.New(file, client)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		// Be careful: tea wraps panics; never include plaintext in messages.
		// The TUI's status line is the right place for sops/git errors —
		// only fall through here for terminal-level issues.
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, exitErr.Error())
			os.Exit(exitErr.code)
		}
		fmt.Fprintf(os.Stderr, "yls: %v\n", err)
		os.Exit(1)
	}
}

type exitError struct {
	msg  string
	code int
}

func (e *exitError) Error() string { return e.msg }

func printUsage() {
	fmt.Fprintln(os.Stderr, `yls — yalazysops, screen-safe TUI for sops-encrypted secrets

Usage:
  yls FILE              open a sops-encrypted YAML or JSON file
  yls --version         print version
  yls --help            print this help

Inside the TUI:
  j/k         move cursor (or arrow keys)
  l/enter     expand branch / copy leaf value
  h           collapse branch / move to parent
  y           copy value to clipboard (auto-clears in 30s)
  e           edit value (masked input, two-step confirm)
  n           add new key under current branch
  d           delete value
  ?           git history for current value
  /           fuzzy search by key path
  q           quit

See README.md for the design doc and security model.`)
}
