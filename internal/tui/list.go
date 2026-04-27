package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libliflin/yalazysops/internal/tree"
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
		return m.frame(
			titleStyle.Render(" yalazysops "),
			"  decrypting…",
			helpStyle.Render("q quit"),
		)
	}

	header := titleStyle.Render(" " + m.file + " ")

	var body strings.Builder
	if len(m.flat) == 0 {
		if m.searchQuery != "" {
			body.WriteString(dimStyle.Render(fmt.Sprintf("  no keys match %q", m.searchQuery)))
		} else {
			body.WriteString(dimStyle.Render("  (empty)"))
		}
	} else {
		for i, row := range m.flat {
			if i > 0 {
				body.WriteString("\n")
			}
			body.WriteString(m.renderRow(i, row))
		}
	}

	return m.frame(header, body.String(), m.bottomBar(
		"y copy   e edit   n new   d delete   ? history   / search   q quit",
	))
}

// bottomBar renders the status line (if any), the search filter (if any),
// and the help line, in that order from top to bottom. Help is ALWAYS the
// last line so the user's eye knows where to look.
func (m Model) bottomBar(help string) string {
	var lines []string
	if m.status != "" {
		if m.statusErr {
			lines = append(lines, errorStyle.Render(m.status))
		} else {
			lines = append(lines, statusStyle.Render(m.status))
		}
	}
	if m.searchQuery != "" {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("filter: /%s", m.searchQuery)))
	}
	lines = append(lines, helpStyle.Render(help))
	return strings.Join(lines, "\n")
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
