package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libliflin/yalazysops/internal/tree"
)

// startDelete opens the y/N confirm modal for the selected leaf.
func (m Model) startDelete(n *tree.Node) (tea.Model, tea.Cmd) {
	if n == nil || !n.IsLeaf() {
		m.setError("Can only delete leaf values")
		return m, clearStatusAfter(3e9)
	}
	m.editPath = n.Path
	m.view = viewConfirmDelete
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		path := m.editPath
		m.view = viewList
		m.setStatus(fmt.Sprintf("Deleting %s…", path.Display()))
		return m, unsetCmd(m.sops, m.file, path)
	case "n", "N", "esc", "q":
		m.view = viewList
		return m, nil
	}
	return m, nil
}

func (m Model) viewConfirmRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Confirm delete "))
	b.WriteString("\n\n")
	body := fmt.Sprintf("Permanently delete %s ?\n\n", m.editPath.Display())
	body += "This cannot be undone from inside yalazysops — but git has the\n"
	body += "previous encrypted blob, so a `git revert` would restore it.\n\n"
	body += helpStyle.Render("[y] yes   [n/esc] cancel")
	b.WriteString(borderStyle.Render(body))
	return b.String()
}
