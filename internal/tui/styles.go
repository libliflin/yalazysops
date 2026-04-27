package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#3a3a4a"))

	keyNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	branchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	fingerprintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666")).
				Italic(true)

	// helpStyle / statusStyle / errorStyle deliberately have no MarginTop —
	// the per-view renderers compose their own layout (see frame() in
	// model.go) so the View output is constant-height regardless of whether
	// a status line is currently showing. Margin-from-styles would re-flow
	// the bottom bar each time status appears or clears.
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00CC66"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))
)
