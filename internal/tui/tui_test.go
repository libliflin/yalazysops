package tui

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/libliflin/yalazysops/internal/gitx"
	"github.com/libliflin/yalazysops/internal/secure"
	"github.com/libliflin/yalazysops/internal/sopsx"
)

// --- fakes ------------------------------------------------------------------

// fakeSops is a hand-rolled fake for the sopsBackend interface. The decrypt
// payload is a plain YAML doc; extract returns canned per-path values; set
// and unset are recorded as call entries so tests can assert they ran.
type fakeSops struct {
	mu         sync.Mutex
	doc        []byte
	values     map[string]string // keyed by path.Display()
	setCalls   []sopsCall
	unsCalls   []sopsCall
	extractErr error
	setErr     error
}

type sopsCall struct {
	Path  string
	Value string // plaintext snapshot for assertions; never logged
}

func (f *fakeSops) Decrypt(file string) (*secure.Buffer, error) {
	out := make([]byte, len(f.doc))
	copy(out, f.doc)
	return secure.NewBuffer(out), nil
}

func (f *fakeSops) Extract(file string, path sopsx.Path) (*secure.Buffer, error) {
	if f.extractErr != nil {
		return nil, f.extractErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[path.Display()]
	if !ok {
		return nil, errors.New("not found: " + path.Display())
	}
	return secure.NewBuffer([]byte(v)), nil
}

func (f *fakeSops) Set(file string, path sopsx.Path, value *secure.Buffer) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls = append(f.setCalls, sopsCall{Path: path.Display(), Value: string(value.Bytes())})
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[path.Display()] = string(value.Bytes())
	return nil
}

func (f *fakeSops) Unset(file string, path sopsx.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unsCalls = append(f.unsCalls, sopsCall{Path: path.Display()})
	delete(f.values, path.Display())
	return nil
}

// fakeGit returns canned commit lists and historical values.
type fakeGit struct {
	commits []gitx.Commit
	values  map[string]string // keyed by sha + "|" + path.Display()
}

func (f *fakeGit) LogForKey(file string, path sopsx.Path, limit int) ([]gitx.Commit, error) {
	return f.commits, nil
}

func (f *fakeGit) ShowAt(sha, file string, path sopsx.Path) (*secure.Buffer, error) {
	v := f.values[sha+"|"+path.Display()]
	return secure.NewBuffer([]byte(v)), nil
}

// fakeClipboard records every write in order. Concurrent-safe because the
// tea runtime may dispatch the message on a goroutine.
type fakeClipboard struct {
	mu     sync.Mutex
	writes []string
}

func (c *fakeClipboard) Copy(b *secure.Buffer, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, string(b.Bytes()))
	return nil
}

func (c *fakeClipboard) Last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.writes) == 0 {
		return ""
	}
	return c.writes[len(c.writes)-1]
}

// fixtureModel returns a Model wired to fakes, with a small synthetic tree.
func fixtureModel(t *testing.T) (Model, *fakeSops, *fakeGit, *fakeClipboard) {
	t.Helper()
	sops := &fakeSops{
		doc: []byte(`anthropic_api_key: sk-ant-test
auth_secret: very-secret
db:
  prod:
    password: hunter2
    host: db.prod.example.com
`),
		values: map[string]string{
			"anthropic_api_key": "sk-ant-test",
			"auth_secret":       "very-secret",
			"db.prod.password":  "hunter2",
			"db.prod.host":      "db.prod.example.com",
		},
	}
	git := &fakeGit{}
	cb := &fakeClipboard{}
	m := newModel("secrets.enc.yaml", sops, git, cb.Copy)
	return m, sops, git, cb
}

// --- direct Update unit tests ----------------------------------------------

// loadInitial drives the Init cmd and the resulting treeLoadedMsg through
// Update once, returning a Model whose tree is populated. Avoids repeating
// the boilerplate in every test.
func loadInitial(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	out, _ := m.Update(msg)
	return out.(Model)
}

func keyMsg(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestUpdate_InitialLoadPopulatesTree(t *testing.T) {
	m, _, _, _ := fixtureModel(t)
	m = loadInitial(t, m)
	if m.root == nil {
		t.Fatal("tree root nil after initial load")
	}
	// Three top-level entries: anthropic_api_key, auth_secret, db.
	if len(m.flat) != 3 {
		t.Errorf("flat rows = %d, want 3", len(m.flat))
	}
}

func TestUpdate_JMovesCursorDown(t *testing.T) {
	m, _, _, _ := fixtureModel(t)
	m = loadInitial(t, m)
	if m.cursor != 0 {
		t.Fatalf("starting cursor = %d", m.cursor)
	}
	out, _ := m.Update(keyMsg("j"))
	m = out.(Model)
	if m.cursor != 1 {
		t.Errorf("after j, cursor = %d, want 1", m.cursor)
	}
}

func TestUpdate_LExpandsBranch(t *testing.T) {
	m, _, _, _ := fixtureModel(t)
	m = loadInitial(t, m)
	// Move cursor to "db" (index 2).
	for i := 0; i < 2; i++ {
		out, _ := m.Update(keyMsg("j"))
		m = out.(Model)
	}
	// l should expand the branch.
	out, _ := m.Update(keyMsg("l"))
	m = out.(Model)
	if !m.expanded["db"] {
		t.Error("db branch not marked expanded after l")
	}
	// db has one child: db.prod. So flat should now have 4 rows.
	if len(m.flat) != 4 {
		t.Errorf("flat rows after expand = %d, want 4", len(m.flat))
	}
}

func TestUpdate_YOnLeafCopies(t *testing.T) {
	m, _, _, cb := fixtureModel(t)
	m = loadInitial(t, m)
	// Cursor is on anthropic_api_key (leaf, index 0).
	out, cmd := m.Update(keyMsg("y"))
	m = out.(Model)
	if cmd == nil {
		t.Fatal("y on leaf returned nil cmd")
	}
	// Drive the cmd: it should produce a valueExtractedMsg, which when fed
	// back into Update writes to the (fake) clipboard.
	msg := cmd()
	out, _ = m.Update(msg)
	m = out.(Model)
	if cb.Last() != "sk-ant-test" {
		t.Errorf("clipboard = %q, want sk-ant-test", cb.Last())
	}
	if !strings.Contains(m.status, "Copied") {
		t.Errorf("status after copy = %q", m.status)
	}
}

func TestUpdate_DConfirmThenDeleteCallsUnset(t *testing.T) {
	m, sops, _, _ := fixtureModel(t)
	m = loadInitial(t, m)
	// Cursor on anthropic_api_key.
	out, _ := m.Update(keyMsg("d"))
	m = out.(Model)
	if m.view != viewConfirmDelete {
		t.Fatalf("view = %v, want confirm", m.view)
	}
	out, cmd := m.Update(keyMsg("y"))
	m = out.(Model)
	if cmd == nil {
		t.Fatal("y on confirm returned nil cmd")
	}
	cmd() // drives unset on the fake
	if len(sops.unsCalls) != 1 {
		t.Fatalf("unset call count = %d, want 1", len(sops.unsCalls))
	}
	if sops.unsCalls[0].Path != "anthropic_api_key" {
		t.Errorf("unset path = %q", sops.unsCalls[0].Path)
	}
}

// TestView_HelpLineSurvivesStatusClear pins the bug from
// https://github.com/libliflin/yalazysops issue (originally surfaced as
// "save message timeout clear clears the help"): when a transient status
// line auto-clears, the help line at the bottom must not vanish with it.
//
// Mechanism: frame() pads the View output to a constant height anchored on
// m.height. Without that, bubbletea's frame differ can leave a stale row
// when the View becomes one line shorter, taking the help line with it.
func TestView_HelpLineSurvivesStatusClear(t *testing.T) {
	m, _, _, _ := fixtureModel(t)
	m = loadInitial(t, m)
	m.height = 24
	m.width = 80

	withoutStatus := m.View()
	wsLines := strings.Split(withoutStatus, "\n")
	if !strings.Contains(wsLines[len(wsLines)-1], "q quit") {
		t.Errorf("with no status, last line missing help: %q", wsLines[len(wsLines)-1])
	}
	if got := len(wsLines); got != m.height {
		t.Errorf("View height with no status = %d, want %d", got, m.height)
	}

	m.setStatus("✓ Updated foo")
	withStatus := m.View()
	sLines := strings.Split(withStatus, "\n")
	if !strings.Contains(sLines[len(sLines)-1], "q quit") {
		t.Errorf("with status, last line missing help: %q", sLines[len(sLines)-1])
	}
	if got := len(sLines); got != m.height {
		t.Errorf("View height with status = %d, want %d (must equal no-status height to prevent frame-shrinkage glitch)", got, m.height)
	}
}

// --- teatest integration test ----------------------------------------------

// TestTeatest_OpenExpandCopy is the seed integration test. It boots the
// real Bubbletea program with our fake backends, sends keypresses through
// the fake terminal, and asserts the rendered output and side effects.
func TestTeatest_OpenExpandCopy(t *testing.T) {
	m, _, _, cb := fixtureModel(t)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Wait for the initial decrypt + tree-build to land.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("anthropic_api_key"))
	}, teatest.WithDuration(3*time.Second))

	// Move cursor to db (index 2), expand, move into the prod branch,
	// expand it, then move to password and copy.
	tm.Send(keyMsg("j"))
	tm.Send(keyMsg("j"))
	tm.Send(keyMsg("l")) // expand db
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("prod"))
	}, teatest.WithDuration(time.Second))

	tm.Send(keyMsg("j"))
	tm.Send(keyMsg("l")) // expand db.prod
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("password"))
	}, teatest.WithDuration(time.Second))

	tm.Send(keyMsg("j")) // password
	tm.Send(keyMsg("y")) // copy

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Copied"))
	}, teatest.WithDuration(2*time.Second))

	tm.Send(keyMsg("q"))
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	if cb.Last() != "hunter2" {
		t.Errorf("clipboard = %q, want hunter2", cb.Last())
	}
}
