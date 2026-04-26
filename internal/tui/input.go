package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// inputModel wraps bubbles/textinput with a thin convenience layer so
// password inputs and plain inputs share the same usage pattern.
type inputModel struct {
	ti     textinput.Model
	masked bool
}

// newInput builds a fresh input. If masked is true, characters are hidden
// with bullets — used for value entry. Otherwise plain — used for key names
// and search queries.
func newInput(masked bool, placeholder string) inputModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 60
	if masked {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '●'
	}
	return inputModel{ti: ti, masked: masked}
}

func (m inputModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m inputModel) View() string       { return m.ti.View() }
func (m inputModel) Value() string      { return m.ti.Value() }
func (m *inputModel) SetValue(s string) { m.ti.SetValue(s) }
func (m *inputModel) Reset()            { m.ti.Reset() }
func (m *inputModel) Focus() tea.Cmd    { return m.ti.Focus() }
func (m *inputModel) Blur()             { m.ti.Blur() }
