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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/libliflin/yalazysops/internal/gitx"
	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
	"github.com/libliflin/yalazysops/internal/tree"
)

// sopsBackend is the subset of sops operations the TUI needs. Allows
// substitution in tests; production wiring uses *sopsx.Client directly,
// which already satisfies this interface.
type sopsBackend interface {
	Decrypt(file string) (*secure.Buffer, error)
	Extract(file string, path sopsx.Path) (*secure.Buffer, error)
	Set(file string, path sopsx.Path, value *secure.Buffer) error
	Unset(file string, path sopsx.Path) error
}

// gitBackend is the subset of git history operations the TUI needs. The
// gitx package exposes these as package-level functions; gitAdapter below
// bridges to the interface so production code keeps the simple call sites.
type gitBackend interface {
	LogForKey(file string, path sopsx.Path, limit int) ([]gitx.Commit, error)
	ShowAt(sha, file string, path sopsx.Path) (*secure.Buffer, error)
}

// clipboardFunc is the signature secure.CopyToClipboard implements. Injected
// so tests can capture clipboard writes without depending on a real X/Wayland
// or pbcopy session.
type clipboardFunc func(b *secure.Buffer, ttl time.Duration) error

// gitAdapter bridges the package-level gitx functions to gitBackend.
// gitx.ShowAt needs a *sopsx.Client because the historical decryption
// pipeline runs `git show … | sops --extract …`; we keep that wiring here
// rather than threading the sops client through the gitBackend interface.
type gitAdapter struct{ sops *sopsx.Client }

func (g *gitAdapter) LogForKey(file string, path sopsx.Path, limit int) ([]gitx.Commit, error) {
	return gitx.LogForKey(file, path, limit)
}

func (g *gitAdapter) ShowAt(sha, file string, path sopsx.Path) (*secure.Buffer, error) {
	return gitx.ShowAt(g.sops, sha, file, path)
}

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
	file      string
	sops      sopsBackend
	git       gitBackend
	clipboard clipboardFunc

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

// New returns the production Model: real sops shell-out, real gitx, real
// clipboard. cmd/yls calls this.
func New(file string, c *sopsx.Client) Model {
	return newModel(file, c, &gitAdapter{sops: c}, secure.CopyToClipboard)
}

// newModel is the test-friendly constructor — accepts arbitrary backends and
// a clipboard sink. Kept unexported; tests live in package tui.
func newModel(file string, sops sopsBackend, git gitBackend, cb clipboardFunc) Model {
	return Model{
		file:         file,
		sops:         sops,
		git:          git,
		clipboard:    cb,
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

func loadTreeCmd(c sopsBackend, file string) tea.Cmd {
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

func extractCmd(c sopsBackend, file string, path sopsx.Path, purpose string) tea.Cmd {
	return func() tea.Msg {
		buf, err := c.Extract(file, path)
		return valueExtractedMsg{purpose: purpose, path: path, buf: buf, err: err}
	}
}

func setCmd(c sopsBackend, file string, path sopsx.Path, value *secure.Buffer) tea.Cmd {
	return func() tea.Msg {
		err := c.Set(file, path, value)
		value.Wipe()
		return writeDoneMsg{op: "set", path: path, err: err}
	}
}

func unsetCmd(c sopsBackend, file string, path sopsx.Path) tea.Cmd {
	return func() tea.Msg {
		err := c.Unset(file, path)
		return writeDoneMsg{op: "unset", path: path, err: err}
	}
}

func historyCmd(g gitBackend, file string, path sopsx.Path) tea.Cmd {
	return func() tea.Msg {
		commits, err := g.LogForKey(file, path, 50)
		return historyLoadedMsg{path: path, commits: commits, err: err}
	}
}

func historyCopyCmd(g gitBackend, sha, file string, path sopsx.Path) tea.Cmd {
	return func() tea.Msg {
		buf, err := g.ShowAt(sha, file, path)
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
		if err := m.clipboard(msg.buf, m.clipboardTTL); err != nil {
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

// frame composes a header, body, and bottom bar into a constant-height
// view. The bottom bar is anchored to row m.height so it never moves when
// transient status lines come and go — that frame stability is what
// prevents the "help line disappears when status auto-clears" rendering
// glitch.
//
// Layout:
//
//	header       (1+ rows; usually a title + blank)
//	body         (variable; the list / commit log / etc.)
//	<spacer>     (blank rows; pads up to terminal height)
//	bottomBar    (status if any, then help — always last row)
//
// If m.height is 0 (no WindowSizeMsg yet) we fall back to a plain
// header / body / bar concatenation with one blank between segments.
func (m Model) frame(header, body, bottomBar string) string {
	header = strings.TrimRight(header, "\n")
	body = strings.TrimRight(body, "\n")
	bottomBar = strings.TrimRight(bottomBar, "\n")

	if m.height <= 0 {
		return header + "\n" + body + "\n\n" + bottomBar
	}

	used := lipgloss.Height(header) + lipgloss.Height(body) + lipgloss.Height(bottomBar)
	if used >= m.height {
		// Body is already taller than the terminal — let it overflow rather
		// than truncate. Better to scroll than to silently drop rows.
		return header + "\n" + body + "\n" + bottomBar
	}
	// Newline arithmetic: between two adjacent content blocks, we want K
	// blank rows separated by row terminators — that's K+1 newlines (one
	// to end the previous row, K-1 to terminate K-1 of the blanks, one to
	// position the next block). Total rows: used + K = m.height.
	blanks := m.height - used
	return header + "\n" + body + strings.Repeat("\n", blanks+1) + bottomBar
}
