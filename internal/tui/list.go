package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/williamlaffin/yalazysops/internal/tree"
)

// rebuildFlat walks the tree and produces the visible (post-expansion +
// post-search) row list, then clamps the cursor.
func (m *Model) rebuildFlat() {
	m.flat = m.flat[:0]
	if m.root == nil {
		return
	}
	q := strings.ToLower(m.searchQuery)
	for _, c := range m.root.Children {
		m.flatten(c, 0, q)
	}
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) flatten(n *tree.Node, depth int, query string) {
	if query == "" {
		m.flat = append(m.flat, flatRow{node: n, depth: depth})
		if !n.IsLeaf() && m.expanded[n.Path.Display()] {
			for _, c := range n.Children {
				m.flatten(c, depth+1, query)
			}
		}
		return
	}
	matches := strings.Contains(strings.ToLower(n.Path.Display()), query)
	if n.IsLeaf() {
		if matches {
			m.flat = append(m.flat, flatRow{node: n, depth: depth})
		}
		return
	}
	saved := len(m.flat)
	m.flat = append(m.flat, flatRow{node: n, depth: depth})
	for _, c := range n.Children {
		m.flatten(c, depth+1, query)
	}
	// If neither this branch nor any descendant matched, drop the
	// placeholder we just appended.
	if !matches && len(m.flat) == saved+1 {
		m.flat = m.flat[:saved]
	}
}

// currentNode returns the node under the cursor, or nil if the list is empty.
func (m *Model) currentNode() *tree.Node {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return nil
	}
	return m.flat[m.cursor].node
}

// handleKey routes a keypress based on the current view.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewList:
		return m.handleListKey(msg)
	case viewEdit, viewNew:
		return m.handleInputKey(msg)
	case viewHistory:
		return m.handleHistoryKey(msg)
	case viewConfirmDelete:
		return m.handleConfirmKey(msg)
	case viewSearch:
		return m.handleSearchKey(msg)
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if model, cmd, ok := m.handleListNav(key); ok {
		return model, cmd
	}
	return m.handleListAction(key)
}

// handleListNav covers cursor movement and tree expand/collapse. Returns
// ok=false when the key isn't a nav key, so the caller can try actions next.
func (m Model) handleListNav(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit, true
	case "j", "down":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
		return m, nil, true
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil, true
	case "g", "home":
		m.cursor = 0
		return m, nil, true
	case "G", "end":
		m.cursor = len(m.flat) - 1
		return m, nil, true
	case "l", "right", "enter":
		model, cmd := m.expandOrCopy(key == "enter")
		return model, cmd, true
	case "h", "left":
		return m.collapseOrAscend(), nil, true
	}
	return m, nil, false
}

// handleListAction covers the verb keys (y/e/n/d/?, /) — anything that
// triggers an action on the current row.
func (m Model) handleListAction(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		return m.startCopy(m.currentNode())
	case "e":
		return m.startEdit(m.currentNode())
	case "n":
		return m.startNew()
	case "d":
		return m.startDelete(m.currentNode())
	case "?":
		return m.startHistory(m.currentNode())
	case "/":
		m.view = viewSearch
		m.searchInput = newInput(false, "search keys...")
		m.searchInput.SetValue(m.searchQuery)
		return m, nil
	}
	return m, nil
}

func (m Model) expandOrCopy(isEnter bool) (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	if !n.IsLeaf() {
		m.expanded[n.Path.Display()] = !m.expanded[n.Path.Display()]
		m.rebuildFlat()
		return m, nil
	}
	if isEnter {
		return m.startCopy(n)
	}
	return m, nil
}

func (m Model) collapseOrAscend() tea.Model {
	n := m.currentNode()
	if n == nil {
		return m
	}
	if !n.IsLeaf() && m.expanded[n.Path.Display()] {
		m.expanded[n.Path.Display()] = false
		m.rebuildFlat()
		return m
	}
	if n.Parent != nil && len(n.Parent.Path) > 0 {
		parentDisp := n.Parent.Path.Display()
		for i, row := range m.flat {
			if row.node.Path.Display() == parentDisp {
				m.cursor = i
				break
			}
		}
	}
	return m
}

func (m Model) startCopy(n *tree.Node) (tea.Model, tea.Cmd) {
	if n == nil || !n.IsLeaf() {
		m.setError("Can only copy leaf values")
		return m, clearStatusAfter(3 * 1e9)
	}
	m.setStatus(fmt.Sprintf("Copying %s…", n.Path.Display()))
	return m, extractCmd(m.sops, m.file, n.Path, "copy")
}

func (m Model) viewListRender() string {
	if m.root == nil {
		return titleStyle.Render(" yalazysops ") + "\n\n  decrypting…"
	}
	var b strings.Builder

	b.WriteString(titleStyle.Render(" " + m.file + " "))
	b.WriteString("\n\n")

	if len(m.flat) == 0 {
		if m.searchQuery != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  no keys match %q", m.searchQuery)))
		} else {
			b.WriteString(dimStyle.Render("  (empty)"))
		}
	} else {
		for i, row := range m.flat {
			b.WriteString(m.renderRow(i, row))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(
		"y copy   e edit   n new   d delete   ? history   / search   q quit",
	))
	if m.searchQuery != "" {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("filter: /%s", m.searchQuery)))
	}
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

func (m *Model) renderRow(i int, row flatRow) string {
	var prefix string
	if i == m.cursor {
		prefix = cursorStyle.Render("▸ ")
	} else {
		prefix = "  "
	}
	indent := strings.Repeat("  ", row.depth)
	n := row.node

	var name, suffix string
	if n.IsLeaf() {
		name = keyNameStyle.Render(n.Name)
		suffix = "  " + fingerprintStyle.Render("sha256:"+n.Fingerprint+"…")
	} else {
		marker := "▾"
		if !m.expanded[n.Path.Display()] {
			marker = "▸"
		}
		name = branchStyle.Render(marker + " " + n.Name)
		suffix = "  " + dimStyle.Render(fmt.Sprintf("(%d)", len(n.Children)))
	}

	line := prefix + indent + name + suffix
	if i == m.cursor {
		return selectedRowStyle.Render(line)
	}
	return line
}
