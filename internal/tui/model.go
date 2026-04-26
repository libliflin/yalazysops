// Package tui implements the Bubbletea model for yalazysops.
//
// The model is a state machine over a small set of views (list, edit, new,
// history, confirm-delete, search). Decryption work is dispatched as
// tea.Cmds so the UI never blocks on a sops shell-out, and plaintext lives
// only inside secure.Buffers that are wiped before the message lands back
// in Update.
package tui

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libliflin/yalazysops/internal/gitx"
	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
	"github.com/libliflin/yalazysops/internal/tree"
)

// view enumerates the screens the TUI cycles through.
type view int

const (
	viewList view = iota
	viewEdit
	viewNew
	viewHistory
	viewConfirmDelete
	viewSearch
)

// Model is the root Bubbletea model.
type Model struct {
	// Static
	file string
	sops *sopsx.Client

	// Tree state
	root     *tree.Node
	flat     []flatRow // visible rows (post-expansion + filter)
	expanded map[string]bool
	cursor   int
	width    int
	height   int

	// Mode
	view view

	// Status (transient)
	status    string
	statusErr bool

	// Edit / New state
	editPath  sopsx.Path // set during edit
	editKey   string     // set during new (the key being added under newPath)
	newPath   sopsx.Path // parent path during new
	editStep  int        // 0=value, 1=confirm
	inputView inputModel
	stage1Val *secure.Buffer // first masked entry, awaiting confirm

	// History state
	historyCommits []gitx.Commit
	historyCursor  int
	historyPath    sopsx.Path

	// Search state
	searchQuery string
	searchInput inputModel

	// Config
	clipboardTTL time.Duration
}

// flatRow is one visible row in the list view.
type flatRow struct {
	node  *tree.Node
	depth int
}

// New returns an initial Model. Loading the document is dispatched on Init.
func New(file string, c *sopsx.Client) Model {
	return Model{
		file:         file,
		sops:         c,
		expanded:     map[string]bool{},
		view:         viewList,
		clipboardTTL: 30 * time.Second,
	}
}

// Init is the Bubbletea entry point — kicks off the initial decrypt+tree.
func (m Model) Init() tea.Cmd {
	return loadTreeCmd(m.sops, m.file)
}

// --- messages ---------------------------------------------------------------

type treeLoadedMsg struct {
	root *tree.Node
	err  error
}

type valueExtractedMsg struct {
	purpose string // "copy" or "history-copy"
	path    sopsx.Path
	buf     *secure.Buffer
	err     error
}

type writeDoneMsg struct {
	op   string // "set", "unset"
	path sopsx.Path
	err  error
}

type historyLoadedMsg struct {
	path    sopsx.Path
	commits []gitx.Commit
	err     error
}

type clearStatusMsg struct{}

// --- commands ---------------------------------------------------------------

func loadTreeCmd(c *sopsx.Client, file string) tea.Cmd {
	return func() tea.Msg {
		buf, err := c.Decrypt(file)
		if err != nil {
			return treeLoadedMsg{err: err}
		}
		defer buf.Wipe()
		root, err := tree.Build(buf.Bytes())
		return treeLoadedMsg{root: root, err: err}
	}
}

func extractCmd(c *sopsx.Client, file string, path sopsx.Path, purpose string) tea.Cmd {
	return func() tea.Msg {
		buf, err := c.Extract(file, path)
		return valueExtractedMsg{purpose: purpose, path: path, buf: buf, err: err}
	}
}

func setCmd(c *sopsx.Client, file string, path sopsx.Path, value *secure.Buffer) tea.Cmd {
	return func() tea.Msg {
		err := c.Set(file, path, value)
		value.Wipe()
		return writeDoneMsg{op: "set", path: path, err: err}
	}
}

func unsetCmd(c *sopsx.Client, file string, path sopsx.Path) tea.Cmd {
	return func() tea.Msg {
		err := c.Unset(file, path)
		return writeDoneMsg{op: "unset", path: path, err: err}
	}
}

func historyCmd(file string, path sopsx.Path) tea.Cmd {
	return func() tea.Msg {
		commits, err := gitx.LogForKey(file, path, 50)
		return historyLoadedMsg{path: path, commits: commits, err: err}
	}
}

func historyCopyCmd(c *sopsx.Client, sha, file string, path sopsx.Path) tea.Cmd {
	return func() tea.Msg {
		buf, err := gitx.ShowAt(c, sha, file, path)
		return valueExtractedMsg{purpose: "history-copy", path: path, buf: buf, err: err}
	}
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearStatusMsg{} })
}

// --- update -----------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case treeLoadedMsg:
		if msg.err != nil {
			m.setError(fmt.Sprintf("decrypt failed: %v", msg.err))
			return m, clearStatusAfter(8 * time.Second)
		}
		m.root = msg.root
		m.rebuildFlat()
		return m, nil

	case valueExtractedMsg:
		if msg.err != nil {
			m.setError(fmt.Sprintf("extract failed: %v", msg.err))
			return m, clearStatusAfter(6 * time.Second)
		}
		defer msg.buf.Wipe()
		if err := secure.CopyToClipboard(msg.buf, m.clipboardTTL); err != nil {
			m.setError(fmt.Sprintf("clipboard: %v", err))
			return m, clearStatusAfter(6 * time.Second)
		}
		m.setStatus(fmt.Sprintf("✓ Copied %s. Auto-clear in %s.",
			msg.path.Display(), m.clipboardTTL))
		return m, clearStatusAfter(m.clipboardTTL)

	case writeDoneMsg:
		if msg.err != nil {
			m.setError(fmt.Sprintf("%s failed: %v", msg.op, msg.err))
			return m, clearStatusAfter(8 * time.Second)
		}
		switch msg.op {
		case "set":
			m.setStatus(fmt.Sprintf("✓ Updated %s in %s",
				msg.path.Display(), filepath.Base(m.file)))
		case "unset":
			m.setStatus(fmt.Sprintf("✓ Deleted %s from %s",
				msg.path.Display(), filepath.Base(m.file)))
		}
		// Reload tree to show the new state.
		return m, tea.Batch(
			loadTreeCmd(m.sops, m.file),
			clearStatusAfter(5*time.Second),
		)

	case historyLoadedMsg:
		if msg.err != nil {
			m.setError(fmt.Sprintf("history failed: %v", msg.err))
			m.view = viewList
			return m, clearStatusAfter(6 * time.Second)
		}
		m.historyCommits = msg.commits
		m.historyPath = msg.path
		m.historyCursor = 0
		m.view = viewHistory
		return m, nil

	case clearStatusMsg:
		m.status = ""
		m.statusErr = false
		return m, nil
	}
	return m, nil
}

func (m *Model) setStatus(s string) {
	m.status = s
	m.statusErr = false
}

func (m *Model) setError(s string) {
	m.status = s
	m.statusErr = true
}

// View dispatches to the per-view renderer.
func (m Model) View() string {
	switch m.view {
	case viewList:
		return m.viewListRender()
	case viewEdit:
		return m.viewEditRender()
	case viewNew:
		return m.viewNewRender()
	case viewHistory:
		return m.viewHistoryRender()
	case viewConfirmDelete:
		return m.viewConfirmRender()
	case viewSearch:
		return m.viewSearchRender()
	}
	return ""
}
