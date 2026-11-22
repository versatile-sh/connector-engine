package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

type write struct {
	key    string
	names  []string
	access console.Access
	mode   console.ProfileMode
}

type fakeAPI struct {
	mu     sync.Mutex
	conns  []console.ConnectorSummary
	writes []write
	fail   error
}

func (f *fakeAPI) Connectors(context.Context) ([]console.ConnectorSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]console.ConnectorSummary, len(f.conns))
	for i, k := range f.conns {
		out[i] = clone(k)
	}
	return out, nil
}

func (f *fakeAPI) SetAccess(_ context.Context, k console.ConnectorSummary, key string, names []string, a console.Access) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, write{key: key, names: slices.Clone(names), access: a})
	if f.fail != nil {
		return f.fail
	}
	for i := range f.conns {
		if f.conns[i].ID != k.ID {
			continue
		}
		if key == "" {
			for j := range f.conns[i].Functions {
				if slices.Contains(names, f.conns[i].Functions[j].Name) {
					f.conns[i].Functions[j].Access = a
				}
			}
		}
	}
	return nil
}

func (f *fakeAPI) SetMode(_ context.Context, _ console.ConnectorSummary, key string, mode console.ProfileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, write{key: key, mode: mode})
	return f.fail
}

func (f *fakeAPI) Keys(context.Context) ([]console.AgentView, error) { return nil, nil }

func (f *fakeAPI) CreateKey(_ context.Context, name string, _ time.Duration) (console.AgentToken, error) {
	return console.AgentToken{Agent: name, Token: "tok-" + name}, nil
}

func (f *fakeAPI) RevokeKey(context.Context, string) error { return nil }

func (f *fakeAPI) Approvals(context.Context) ([]port.Approval, error) { return nil, nil }

func (f *fakeAPI) Decide(context.Context, string, bool) error { return nil }

func (f *fakeAPI) Activity(context.Context, time.Time, string, string) ([]console.ActivityCall, error) {
	return nil, nil
}

func fixture() *fakeAPI {
	return &fakeAPI{conns: []console.ConnectorSummary{{
		ID: "c1", Alias: "notes", Title: "Notes",
		Functions: []console.FunctionView{
			{Name: "search", Kind: port.ClassRead, Access: console.AccessOn},
			{Name: "get", Kind: port.ClassRead, Access: console.AccessOff},
			{Name: "create", Kind: port.ClassWrite, Access: console.AccessOn},
			{Name: "purge", Kind: port.ClassDestructive, Access: console.AccessAsk},
		},
		Profiles: []console.Profile{{Agent: "claude", Mode: console.ProfileFollow, Overrides: map[string]console.Access{}}},
	}}}
}

func drive(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return m
		}
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				m = drive(t, m, c)
			}
			return m
		}
		if _, ok := msg.(tickMsg); ok {
			return m
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	special := map[string]rune{"enter": tea.KeyEnter, "esc": tea.KeyEscape, "right": tea.KeyRight, "left": tea.KeyLeft, "down": tea.KeyDown, "tab": tea.KeyTab, " ": tea.KeySpace}
	for _, k := range keys {
		msg := tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
		if c, ok := special[k]; ok {
			msg = tea.KeyPressMsg{Code: c}
		}
		next, cmd := m.Update(msg)
		m = drive(t, next.(Model), cmd)
	}
	return m
}

func started(t *testing.T, api *fakeAPI) Model {
	t.Helper()
	m := New(context.Background(), api)
	m.refresh = 0
	return drive(t, m, m.Init())
}

func fn(m Model, name string) console.Access {
	i, _ := m.connector()
	k := m.conns[i]
	p, ok := m.key(k)
	if !ok {
		return Access(k, nil, name)
	}
	return Access(k, &p, name)
}

func TestTogglingAFunctionWritesItAndShowsIt(t *testing.T) {
	api := fixture()
	m := press(t, started(t, api), "enter", "right")
	if got := fn(m, "search"); got != console.AccessAsk {
		t.Fatalf("search shows %s after →, want ask", got)
	}
	m = press(t, m, "down", " ")
	if got := fn(m, "get"); got != console.AccessOn {
		t.Fatalf("get shows %s after space, want on", got)
	}
	m = press(t, m, "left")
	want := []write{{names: []string{"search"}, access: "ask"}, {names: []string{"get"}, access: "on"}, {names: []string{"get"}, access: "off"}}
	if !equalWrites(api.writes, want) {
		t.Fatalf("writes %+v", api.writes)
	}
	if api.conns[0].Functions[1].Access != console.AccessOff || fn(m, "get") != console.AccessOff {
		t.Fatalf("after ← the console has %s and the screen %s", api.conns[0].Functions[1].Access, fn(m, "get"))
	}
}

func TestBulkActionsSendOneRequest(t *testing.T) {
	api := fixture()
	m := press(t, started(t, api), "enter", "r", "w", "x")
	want := []write{{names: []string{"search", "get"}, access: "on"}, {names: []string{"create"}, access: "ask"}, {names: []string{"search", "get", "create", "purge"}, access: "off"}}
	if !equalWrites(api.writes, want) {
		t.Fatalf("writes %+v", api.writes)
	}
	for _, name := range []string{"search", "get", "create", "purge"} {
		if fn(m, name) != console.AccessOff {
			t.Fatalf("%s is %s after everything off", name, fn(m, name))
		}
	}
}

func TestARefusedWriteRollsBack(t *testing.T) {
	api := fixture()
	api.fail = errors.New("you may not change this connector")
	m := press(t, started(t, api), "enter", "x")
	if got := fn(m, "search"); got != console.AccessOn {
		t.Fatalf("search is %s after a refused write, want the old on", got)
	}
	if !m.failed || !strings.Contains(m.render(), "you may not change this connector") {
		t.Fatalf("the refusal is not shown: %q", m.status)
	}
}

func TestTheKeyTabsWriteTheKeysProfile(t *testing.T) {
	api := fixture()
	m := press(t, started(t, api), "enter", "]")
	if p, ok := m.key(m.conns[0]); !ok || p.Agent != "claude" {
		t.Fatalf("] did not select the key: tab %d", m.tab)
	}
	m = press(t, m, "right", "f", "[")
	want := []write{{key: "claude", names: []string{"search"}, access: "ask"}, {key: "claude", mode: console.ProfileCustom}}
	if !equalWrites(api.writes, want) || m.tab != 0 {
		t.Fatalf("writes %+v, tab %d", api.writes, m.tab)
	}
}

func TestFilterQuitAndKeys(t *testing.T) {
	api := fixture()
	m := press(t, started(t, api), "/", "z", "z", "enter")
	if len(m.visible()) != 0 {
		t.Fatal("the filter kept a connector that does not match")
	}
	m = press(t, m, "/", "esc")
	if len(m.visible()) != 1 {
		t.Fatal("esc did not clear the filter")
	}
	m = press(t, m, "2", "n", "b", "o", "t", "enter")
	if m.token != "tok-bot" || !strings.Contains(m.render(), "tok-bot") {
		t.Fatalf("the new key is not shown once: %q", m.token)
	}
	m = press(t, m, "q")
	if m.token != "" {
		t.Fatal("a key press did not close the new key")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q does not quit")
	}
}

func TestTheScreenFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {140, 40}} {
		api := fixture()
		api.conns[0].Functions[2].Estimated = true
		m := started(t, api)
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for _, keys := range [][]string{nil, {"enter"}, {"tab"}, {"tab", "tab"}, {"tab", "tab", "tab"}} {
			v := press(t, m, keys...).render()
			lines := strings.Split(v, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%v at %v: %d lines", keys, size, len(lines))
			}
			for _, l := range lines {
				if lipgloss.Width(l) > size[0] {
					t.Fatalf("%v at %v: a line is %d wide: %q", keys, size, lipgloss.Width(l), l)
				}
			}
		}
	}
}

func sized(t *testing.T, api *fakeAPI, w, h int) Model {
	t.Helper()
	next, _ := started(t, api).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func TestEveryHintShowsAt80x24(t *testing.T) {
	m := press(t, sized(t, fixture(), 80, 24), "enter")
	v := ansi.Strip(m.render())
	for _, hint := range m.hints() {
		if !strings.Contains(v, hint) {
			t.Fatalf("hint %q is not on the screen:\n%s", hint, v)
		}
	}
}

func TestTheSelectedFunctionStaysOnScreen(t *testing.T) {
	api := fixture()
	for i := range 40 {
		kind := []port.Class{port.ClassRead, port.ClassWrite, port.ClassDestructive}[i%3]
		api.conns[0].Functions = append(api.conns[0].Functions, console.FunctionView{Name: fmt.Sprintf("fn%02d", i), Kind: kind, Access: console.AccessOff})
	}
	m := press(t, sized(t, api, 80, 24), "enter")
	fns := rows(m.conns[0])
	for i := 1; i < len(fns); i++ {
		m = press(t, m, "down")
		v := ansi.Strip(m.render())
		if !strings.Contains(v, "● "+fns[i].Name) || !strings.Contains(v, "Notes") || len(strings.Split(v, "\n")) != 24 {
			t.Fatalf("after %d downs %s is not on screen:\n%s", i, fns[i].Name, v)
		}
	}
}

func TestAnEstimatedKindIsMarked(t *testing.T) {
	api := fixture()
	api.conns[0].Functions[2].Estimated = true
	v := ansi.Strip(press(t, sized(t, api, 80, 24), "enter").render())
	for _, l := range strings.Split(v, "\n") {
		marked := strings.Contains(l, "· estimated")
		if strings.Contains(l, "create") != marked || marked && strings.Contains(l, "search") {
			t.Fatalf("only create is estimated:\n%s", v)
		}
	}
	if !strings.Contains(v, "· estimated") {
		t.Fatalf("create is not marked estimated:\n%s", v)
	}
}

func TestALongKeyIsShownWhole(t *testing.T) {
	m := sized(t, fixture(), 80, 24)
	m.screen, m.token = screenKeys, strings.Repeat("abcdefghij", 47)+"wxyz"
	v := m.render()
	var b strings.Builder
	for _, l := range strings.Split(ansi.Strip(v), "\n") {
		b.WriteString(strings.TrimSpace(l))
	}
	if !strings.Contains(b.String(), m.token) || len(strings.Split(v, "\n")) != 24 {
		t.Fatalf("the %d-character key is cut:\n%s", len(m.token), v)
	}
}

func equalWrites(got, want []write) bool {
	return slices.EqualFunc(got, want, func(a, b write) bool {
		return a.key == b.key && a.access == b.access && a.mode == b.mode && slices.Equal(a.names, b.names)
	})
}
