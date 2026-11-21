// Package tui is the full-screen vsh: connectors with their access per key, keys, approvals and a live activity tail, every write through the console API.
package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

// API is what the TUI reads and writes; *manage.Client implements it over the console's HTTP API.
type API interface {
	Connectors(ctx context.Context) ([]console.ConnectorSummary, error)
	SetAccess(ctx context.Context, k console.ConnectorSummary, key string, names []string, access console.Access) error
	SetMode(ctx context.Context, k console.ConnectorSummary, key string, mode console.ProfileMode) error
	Keys(ctx context.Context) ([]console.AgentView, error)
	CreateKey(ctx context.Context, name string, ttl time.Duration) (console.AgentToken, error)
	RevokeKey(ctx context.Context, name string) error
	Approvals(ctx context.Context) ([]port.Approval, error)
	Decide(ctx context.Context, id string, approve bool) error
	Activity(ctx context.Context, since time.Time, connector, key string) ([]console.ActivityCall, error)
}

type screen int

const (
	screenConnectors screen = iota
	screenKeys
	screenApprovals
	screenActivity
)

var screens = []string{"Connectors", "Keys", "Approvals", "Activity"}

var levels = []console.Access{console.AccessOff, console.AccessOn, console.AccessAsk}

const refreshEvery = 2 * time.Second

// Model is the TUI's state; New builds it and bubbletea drives it.
type Model struct {
	api     API
	ctx     context.Context
	refresh time.Duration
	w, h    int
	screen  screen
	status  string
	failed  bool

	conns     []console.ConnectorSummary
	filter    textinput.Model
	filtering bool
	cursor    int
	open      string
	tab       int
	row       int

	keys      []console.AgentView
	keyCursor int
	naming    bool
	name      textinput.Model
	token     string
	revoking  string

	approvals []port.Approval
	apCursor  int

	calls []console.ActivityCall
}

type (
	connsMsg struct {
		list  []console.ConnectorSummary
		err   error
		quiet bool
	}
	writeMsg struct {
		before console.ConnectorSummary
		what   string
		err    error
	}
	keysMsg struct {
		list []console.AgentView
		err  error
	}
	tokenMsg struct {
		tok console.AgentToken
		err error
	}
	doneMsg struct {
		what string
		err  error
	}
	approvalsMsg struct {
		list []port.Approval
		err  error
	}
	activityMsg struct {
		list []console.ActivityCall
		err  error
	}
	tickMsg struct{}
)

// New returns the model over api; ctx bounds every request.
func New(ctx context.Context, api API) Model {
	f := textinput.New()
	f.Prompt, f.Placeholder = "/ ", "filter"
	n := textinput.New()
	n.Prompt, n.Placeholder, n.CharLimit = "name: ", "the agent's name", 64
	return Model{api: api, ctx: ctx, refresh: refreshEvery, w: 80, h: 24, filter: f, name: n}
}

// Init loads every screen and starts the refresh tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadConns(), m.loadKeys(), m.loadApprovals(), m.loadActivity(), m.tick())
}

func (m Model) tick() tea.Cmd {
	if m.refresh <= 0 {
		return nil
	}
	return tea.Tick(m.refresh, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) loadConns() tea.Cmd {
	return func() tea.Msg { l, err := m.api.Connectors(m.ctx); return connsMsg{l, err, false} }
}

func (m Model) reloadKeepingError() tea.Cmd {
	return func() tea.Msg { l, err := m.api.Connectors(m.ctx); return connsMsg{l, err, true} }
}

func (m Model) loadKeys() tea.Cmd {
	return func() tea.Msg { l, err := m.api.Keys(m.ctx); return keysMsg{l, err} }
}

func (m Model) loadApprovals() tea.Cmd {
	return func() tea.Msg { l, err := m.api.Approvals(m.ctx); return approvalsMsg{l, err} }
}

func (m Model) loadActivity() tea.Cmd {
	return func() tea.Msg {
		l, err := m.api.Activity(m.ctx, time.Now().Add(-time.Hour), "", "")
		return activityMsg{l, err}
	}
}

func (m *Model) say(err error, ok string) {
	m.failed = err != nil
	if err != nil {
		m.status = err.Error()
		return
	}
	if ok != "" {
		m.status = ok
	}
}

func (m Model) visible() []console.ConnectorSummary {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		return m.conns
	}
	var out []console.ConnectorSummary
	for _, k := range m.conns {
		if strings.Contains(strings.ToLower(k.Alias+" "+k.Title), q) {
			out = append(out, k)
		}
	}
	return out
}

func (m Model) connector() (int, bool) {
	i := slices.IndexFunc(m.conns, func(k console.ConnectorSummary) bool { return k.ID == m.open })
	return i, i >= 0
}

func rows(k console.ConnectorSummary) []console.FunctionView {
	var out []console.FunctionView
	for _, kind := range []port.Class{port.ClassRead, port.ClassWrite, port.ClassDestructive} {
		for _, f := range k.Functions {
			if f.Kind == kind {
				out = append(out, f)
			}
		}
	}
	for _, f := range k.Functions {
		if !slices.Contains([]port.Class{port.ClassRead, port.ClassWrite, port.ClassDestructive}, f.Kind) {
			out = append(out, f)
		}
	}
	return out
}

func (m Model) key(k console.ConnectorSummary) (console.Profile, bool) {
	if m.tab == 0 || m.tab > len(k.Profiles) {
		return console.Profile{}, false
	}
	return k.Profiles[m.tab-1], true
}

// Access is what function name of k gets on the tab: the connector's own access, or for a key its override, nothing when off, else the connector's.
func Access(k console.ConnectorSummary, p *console.Profile, name string) console.Access {
	var base console.Access
	for _, f := range k.Functions {
		if f.Name == name {
			base = f.Access
		}
	}
	if p == nil {
		return base
	}
	switch p.Mode {
	case console.ProfileOff:
		return console.AccessOff
	case console.ProfileCustom:
		if a, ok := p.Overrides[name]; ok {
			return a
		}
	}
	return base
}

func clone(k console.ConnectorSummary) console.ConnectorSummary {
	k.Functions = slices.Clone(k.Functions)
	k.Profiles = slices.Clone(k.Profiles)
	for i, p := range k.Profiles {
		overrides := make(map[string]console.Access, len(p.Overrides))
		for n, a := range p.Overrides {
			overrides[n] = a
		}
		k.Profiles[i].Overrides = overrides
	}
	return k
}

func (m *Model) set(names []string, a console.Access, what string) tea.Cmd {
	i, ok := m.connector()
	if !ok || len(names) == 0 {
		return nil
	}
	before := clone(m.conns[i])
	k := clone(m.conns[i])
	key := ""
	if p, ok := m.key(k); ok {
		key = p.Agent
		pi := m.tab - 1
		k.Profiles[pi].Mode = console.ProfileCustom
		for _, n := range names {
			k.Profiles[pi].Overrides[n] = a
		}
	} else {
		for fi := range k.Functions {
			if slices.Contains(names, k.Functions[fi].Name) {
				k.Functions[fi].Access = a
			}
		}
	}
	m.conns[i] = k
	m.status, m.failed = what+"…", false
	return func() tea.Msg {
		return writeMsg{before: before, what: what, err: m.api.SetAccess(m.ctx, before, key, names, a)}
	}
}

func (m *Model) setMode(mode console.ProfileMode) tea.Cmd {
	i, ok := m.connector()
	if !ok {
		return nil
	}
	p, ok := m.key(m.conns[i])
	if !ok {
		return nil
	}
	before := clone(m.conns[i])
	k := clone(m.conns[i])
	k.Profiles[m.tab-1].Mode = mode
	m.conns[i] = k
	what := p.Agent + ": " + string(mode)
	m.status, m.failed = what+"…", false
	return func() tea.Msg {
		return writeMsg{before: before, what: what, err: m.api.SetMode(m.ctx, before, p.Agent, mode)}
	}
}

func step(a console.Access, by int) console.Access {
	i := slices.Index(levels, a)
	if i < 0 {
		i = 0
	}
	return levels[(i+by+len(levels))%len(levels)]
}

func ofKind(k console.ConnectorSummary, kind port.Class) []string {
	var out []string
	for _, f := range k.Functions {
		if kind == "" || f.Kind == kind {
			out = append(out, f.Name)
		}
	}
	return out
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case connsMsg:
		if msg.err == nil {
			m.conns = msg.list
		}
		if msg.err != nil || !msg.quiet {
			m.say(msg.err, "")
		}
		return m, nil
	case writeMsg:
		if msg.err != nil {
			if i := slices.IndexFunc(m.conns, func(k console.ConnectorSummary) bool { return k.ID == msg.before.ID }); i >= 0 {
				m.conns[i] = msg.before
			}
			m.say(msg.err, "")
			return m, m.reloadKeepingError()
		}
		m.say(nil, msg.what)
		return m, m.loadConns()
	case keysMsg:
		if msg.err == nil {
			m.keys = msg.list
			m.keyCursor = min(m.keyCursor, max(len(m.keys)-1, 0))
		}
		m.say(msg.err, "")
		return m, nil
	case tokenMsg:
		if msg.err == nil {
			m.token = msg.tok.Token
		}
		m.say(msg.err, "created "+msg.tok.Agent)
		return m, tea.Batch(m.loadKeys(), m.loadConns())
	case doneMsg:
		m.say(msg.err, msg.what)
		return m, tea.Batch(m.loadKeys(), m.loadApprovals(), m.loadConns())
	case approvalsMsg:
		if msg.err == nil {
			m.approvals = msg.list
			m.apCursor = min(m.apCursor, max(len(m.approvals)-1, 0))
		}
		return m, nil
	case activityMsg:
		if msg.err == nil {
			m.calls = msg.list
		}
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.loadApprovals(), m.loadActivity(), m.tick())
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	if s == "ctrl+c" {
		return m, tea.Quit
	}
	if m.token != "" {
		m.token = ""
		return m, nil
	}
	if m.filtering || m.naming {
		return m.typing(msg)
	}
	if m.revoking != "" {
		name := m.revoking
		m.revoking = ""
		if s == "y" {
			return m, func() tea.Msg { return doneMsg{"revoked " + name, m.api.RevokeKey(m.ctx, name)} }
		}
		m.status = "kept " + name
		return m, nil
	}
	switch s {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.screen == screenConnectors && m.open != "" {
			m.open = ""
			return m, nil
		}
		return m, tea.Quit
	case "tab":
		m.screen = (m.screen + 1) % screen(len(screens))
		return m, nil
	case "shift+tab":
		m.screen = (m.screen + screen(len(screens)) - 1) % screen(len(screens))
		return m, nil
	case "1", "2", "3", "4":
		m.screen = screen(s[0] - '1')
		return m, nil
	}
	switch m.screen {
	case screenConnectors:
		if m.open != "" {
			return m.detailKey(s)
		}
		return m.listKey(s)
	case screenKeys:
		return m.keysKey(s)
	case screenApprovals:
		return m.approvalsKey(s)
	}
	return m, nil
}

func (m Model) typing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.filtering {
			m.filter.SetValue("")
		}
		m.filtering, m.naming = false, false
		m.filter.Blur()
		m.name.Blur()
		return m, nil
	case "enter":
		if m.naming {
			name := strings.TrimSpace(m.name.Value())
			m.naming = false
			m.name.Blur()
			m.name.SetValue("")
			if name == "" {
				return m, nil
			}
			m.status, m.failed = "creating "+name+"…", false
			return m, func() tea.Msg { t, err := m.api.CreateKey(m.ctx, name, 0); return tokenMsg{t, err} }
		}
		m.filtering = false
		m.filter.Blur()
		m.cursor = 0
		return m, nil
	}
	var cmd tea.Cmd
	if m.naming {
		m.name, cmd = m.name.Update(msg)
	} else {
		m.filter, cmd = m.filter.Update(msg)
		m.cursor = 0
	}
	return m, cmd
}

func (m Model) listKey(s string) (tea.Model, tea.Cmd) {
	list := m.visible()
	switch s {
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, max(len(list)-1, 0))
	case "/":
		m.filtering = true
		return m, m.filter.Focus()
	case "enter", "right", "l":
		if m.cursor < len(list) {
			m.open, m.tab, m.row = list[m.cursor].ID, 0, 0
		}
	case "g":
		return m, m.loadConns()
	}
	return m, nil
}

func (m Model) detailKey(s string) (tea.Model, tea.Cmd) {
	i, ok := m.connector()
	if !ok {
		m.open = ""
		return m, nil
	}
	k := m.conns[i]
	fns := rows(k)
	var p *console.Profile
	if pr, ok := m.key(k); ok {
		p = &pr
	}
	switch s {
	case "up", "k":
		m.row = max(m.row-1, 0)
	case "down", "j":
		m.row = min(m.row+1, max(len(fns)-1, 0))
	case "]", "}":
		m.tab = (m.tab + 1) % (len(k.Profiles) + 1)
	case "[", "{":
		m.tab = (m.tab + len(k.Profiles)) % (len(k.Profiles) + 1)
	case "left", "h", "right", "l", "space":
		if m.row >= len(fns) {
			return m, nil
		}
		by := 1
		if s == "left" || s == "h" {
			by = -1
		}
		name := fns[m.row].Name
		a := step(Access(k, p, name), by)
		return m, m.set([]string{name}, a, name+": "+string(a))
	case "r":
		return m, m.set(ofKind(k, port.ClassRead), console.AccessOn, "all read functions on")
	case "w":
		return m, m.set(ofKind(k, port.ClassWrite), console.AccessAsk, "all write functions ask")
	case "x":
		return m, m.set(ofKind(k, ""), console.AccessOff, "everything off")
	case "f":
		if p != nil {
			mode := console.ProfileFollow
			if p.Mode == console.ProfileFollow {
				mode = console.ProfileCustom
			}
			return m, m.setMode(mode)
		}
	}
	return m, nil
}

func (m Model) keysKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		m.keyCursor = max(m.keyCursor-1, 0)
	case "down", "j":
		m.keyCursor = min(m.keyCursor+1, max(len(m.keys)-1, 0))
	case "n":
		m.naming = true
		return m, m.name.Focus()
	case "d":
		if m.keyCursor < len(m.keys) {
			m.revoking = m.keys[m.keyCursor].Name
		}
	}
	return m, nil
}

func (m Model) approvalsKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		m.apCursor = max(m.apCursor-1, 0)
	case "down", "j":
		m.apCursor = min(m.apCursor+1, max(len(m.approvals)-1, 0))
	case "a", "d":
		if m.apCursor >= len(m.approvals) {
			return m, nil
		}
		id, approve := string(m.approvals[m.apCursor].ID), s == "a"
		what := "denied " + id
		if approve {
			what = "approved " + id
		}
		return m, func() tea.Msg { return doneMsg{what, m.api.Decide(m.ctx, id, approve)} }
	}
	return m, nil
}
