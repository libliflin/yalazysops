package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
	"github.com/libliflin/yalazysops/internal/tree"
)

// startEdit opens the masked-input flow to replace an existing leaf value.
func (m Model) startEdit(n *tree.Node) (tea.Model, tea.Cmd) {
	if n == nil || !n.IsLeaf() {
		m.setError("Can only edit leaf values")
		return m, clearStatusAfter(3e9)
	}
	m.view = viewEdit
	m.editPath = n.Path
	m.editStep = 0
	m.inputView = newInput(true, "new value")
	m.stage1Val = nil
	return m, m.inputView.Focus()
}

// startNew opens the two-step flow: key name (visible), then masked value
// with confirm. The new key is created under the current cursor's branch
// (or under a leaf's parent branch).
func (m Model) startNew() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	var parent sopsx.Path
	if n != nil {
		if n.IsLeaf() && n.Parent != nil {
			parent = n.Parent.Path
		} else if !n.IsLeaf() {
			parent = n.Path
		}
	}
	m.view = viewNew
	m.newPath = parent
	m.editKey = ""
	m.editStep = 0
	m.inputView = newInput(false, "key name")
	m.stage1Val = nil
	return m, m.inputView.Focus()
}

// handleInputKey routes keys for the edit and new flows.
func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.cancelEdit()
		return m, nil
	case "enter":
		return m.advanceEdit()
	}
	var cmd tea.Cmd
	m.inputView, cmd = m.inputView.Update(msg)
	return m, cmd
}

func (m *Model) cancelEdit() {
	if m.stage1Val != nil {
		m.stage1Val.Wipe()
		m.stage1Val = nil
	}
	m.inputView.Reset()
	m.view = viewList
}

// advanceEdit handles Enter — moves to the next step or commits the write.
func (m Model) advanceEdit() (tea.Model, tea.Cmd) {
	val := m.inputView.Value()
	switch m.view {
	case viewEdit:
		// Step 0: first value. Step 1: confirm.
		if m.editStep == 0 {
			m.stage1Val = secure.NewBuffer([]byte(val))
			m.inputView.Reset()
			m.editStep = 1
			return m, nil
		}
		// Confirm step.
		if string(m.stage1Val.Bytes()) != val {
			m.stage1Val.Wipe()
			m.stage1Val = nil
			m.inputView.Reset()
			m.editStep = 0
			m.setError("Values didn't match. Try again.")
			return m, clearStatusAfter(4e9)
		}
		buf := m.stage1Val
		m.stage1Val = nil
		m.inputView.Reset()
		m.view = viewList
		path := m.editPath
		m.setStatus(fmt.Sprintf("Saving %s…", path.Display()))
		return m, setCmd(m.sops, m.file, path, buf)

	case viewNew:
		// Step 0: key name. Step 1: value. Step 2: confirm value.
		switch m.editStep {
		case 0:
			name := strings.TrimSpace(val)
			if name == "" {
				m.setError("Key name cannot be empty")
				return m, clearStatusAfter(3e9)
			}
			m.editKey = name
			m.editStep = 1
			m.inputView = newInput(true, "value")
			return m, m.inputView.Focus()
		case 1:
			m.stage1Val = secure.NewBuffer([]byte(val))
			m.inputView.Reset()
			m.editStep = 2
			return m, nil
		case 2:
			if string(m.stage1Val.Bytes()) != val {
				m.stage1Val.Wipe()
				m.stage1Val = nil
				m.inputView.Reset()
				m.editStep = 1
				m.setError("Values didn't match. Try again.")
				return m, clearStatusAfter(4e9)
			}
			buf := m.stage1Val
			m.stage1Val = nil
			path := append(sopsx.Path{}, m.newPath...)
			path = append(path, m.editKey)
			m.inputView.Reset()
			m.view = viewList
			m.setStatus(fmt.Sprintf("Adding %s…", path.Display()))
			return m, setCmd(m.sops, m.file, path, buf)
		}
	}
	return m, nil
}

func (m Model) viewEditRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Edit " + m.editPath.Display() + " "))
	b.WriteString("\n\n")
	body := "Current: (masked)\n\n"
	if m.editStep == 0 {
		body += "Enter new value: " + m.inputView.View()
	} else {
		body += "         Confirm: " + m.inputView.View()
	}
	body += "\n\n" + helpStyle.Render("[enter] save   [esc] cancel")
	b.WriteString(borderStyle.Render(body))
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

func (m Model) viewNewRender() string {
	var b strings.Builder
	parent := "(root)"
	if len(m.newPath) > 0 {
		parent = m.newPath.Display()
	}
	b.WriteString(titleStyle.Render(" New key under " + parent + " "))
	b.WriteString("\n\n")
	var body string
	switch m.editStep {
	case 0:
		body = "Key name: " + m.inputView.View()
	case 1:
		body = "Key:      " + m.editKey + "\n"
		body += "Value:    " + m.inputView.View()
	case 2:
		body = "Key:      " + m.editKey + "\n"
		body += "Confirm:  " + m.inputView.View()
	}
	body += "\n\n" + helpStyle.Render("[enter] next   [esc] cancel")
	b.WriteString(borderStyle.Render(body))
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
