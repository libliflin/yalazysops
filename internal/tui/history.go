package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libliflin/yalazysops/internal/tree"
)

func (m Model) startHistory(n *tree.Node) (tea.Model, tea.Cmd) {
	if n == nil || !n.IsLeaf() {
		m.setError("History is only available for leaf values")
		return m, clearStatusAfter(3e9)
	}
	m.setStatus(fmt.Sprintf("Loading history for %s…", n.Path.Display()))
	return m, historyCmd(m.git, m.file, n.Path)
}

func (m Model) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "h", "left":
		m.view = viewList
		return m, nil
	case "j", "down":
		if m.historyCursor < len(m.historyCommits)-1 {
			m.historyCursor++
		}
		return m, nil
	case "k", "up":
		if m.historyCursor > 0 {
			m.historyCursor--
		}
		return m, nil
	case "y", "enter":
		if m.historyCursor < 0 || m.historyCursor >= len(m.historyCommits) {
			return m, nil
		}
		c := m.historyCommits[m.historyCursor]
		m.setStatus(fmt.Sprintf("Copying %s @ %s…", m.historyPath.Display(), c.Short))
		return m, historyCopyCmd(m.git, c.SHA, m.file, m.historyPath)
	}
	return m, nil
}

func (m Model) viewHistoryRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" History: " + m.historyPath.Display() + " "))
	b.WriteString("\n\n")
	if len(m.historyCommits) == 0 {
		b.WriteString(dimStyle.Render("  (no commits found for this file)"))
	} else {
		for i, c := range m.historyCommits {
			line := fmt.Sprintf("  %s  %s  %-20s  %s",
				c.Short, c.Date.Format("2006-01-02"), truncate(c.Author, 20), c.Subject)
			if i == m.historyCursor {
				line = cursorStyle.Render("▸") + line[1:]
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("y/enter copy historical value   q/esc back"))
	if m.status != "" {
		b.WriteString("\n")
		if m.statusErr {
			b.WriteString(errorStyle.Render(m.status))
		} else {
			b.WriteString(statusStyle.Render(m.status))
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return s[:n-1] + "…"
}
