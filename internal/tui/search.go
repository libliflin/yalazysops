package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.searchInput.Reset()
		m.searchQuery = ""
		m.view = viewList
		m.rebuildFlat()
		return m, nil
	case "enter":
		m.searchQuery = m.searchInput.Value()
		m.view = viewList
		m.rebuildFlat()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	// Live filter as the user types — never indexes values, only key paths.
	m.searchQuery = m.searchInput.Value()
	m.rebuildFlat()
	return m, cmd
}

func (m Model) viewSearchRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" search "))
	b.WriteString("\n\n")
	b.WriteString(borderStyle.Render("/" + m.searchInput.View()))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("[enter] apply   [esc] cancel"))
	b.WriteString("\n\n")
	if m.root != nil {
		// Show a live preview of matching keys underneath.
		shown := 0
		for _, row := range m.flat {
			if !row.node.IsLeaf() {
				continue
			}
			b.WriteString("  ")
			b.WriteString(row.node.Path.Display())
			b.WriteString("\n")
			shown++
			if shown >= 10 {
				b.WriteString(dimStyle.Render("  …"))
				break
			}
		}
		if shown == 0 {
			b.WriteString(dimStyle.Render("  no matches"))
		}
	}
	return b.String()
}
