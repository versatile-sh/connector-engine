package evaluate_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	corecatalog "github.com/agents-control/connector-engine/core/catalog"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/core/grant"
	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/port"
)

type (
	Mode          = port.Mode
	Class         = port.Class
	State         = port.FunctionState
	Scope         = port.ScopeKind
	Level         = port.Level
	Ceiling       = port.Ceiling
	ListingStatus = port.ListingStatus
	SubjectKind   = port.SubjectKind
	TargetKind    = port.TargetKind
)

const (
	Deny    = port.ModeDeny
	Ask     = port.ModeAsk
	AskOnce = port.ModeAskOnce
	Allow   = port.ModeAllow
	Inherit = port.ModeInherit

	Unclassified = port.ClassNone
	Read         = port.ClassRead
	Write        = port.ClassWrite
	Destructive  = port.ClassDestructive

	Active      = port.FunctionActive
	Quarantined = port.FunctionQuarantined
	Removed     = port.FunctionRemoved
	Disabled    = port.FunctionDisabled

	Workspace = port.ScopeWorkspace
	Tenant    = port.ScopeTenant
	Agent     = port.ScopeAgent
	User      = port.ScopeUser
	Session   = port.ScopeSession

	Off        = port.LevelOff
	ReadLevel  = port.LevelRead
	WriteLevel = port.LevelWrite
	Custom     = port.LevelCustom

	CeilRead  = port.CeilingRead
	CeilWrite = port.CeilingWrite
	CeilFull  = port.CeilingFull

	ListingActive      = port.ListingActive
	ListingDeprecated  = port.ListingDeprecated
	ListingQuarantined = port.ListingQuarantined
	ListingWithdrawn   = port.ListingDeleted

	TenantSubject    = port.SubjectTenant
	WorkspaceSubject = port.SubjectWorkspace
	GroupSubject     = port.SubjectGroup
	UserSubject      = port.SubjectUser

	SourceTarget    = port.TargetSource
	PublisherTarget = port.TargetPublisher
	ListingTarget   = port.TargetListing

	FromApproval = port.OriginApproval
)

type Caller int

const (
	Model Caller = iota
	View
)

var epoch = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

func at(s int64) time.Time {
	if s == 0 {
		return time.Time{}
	}
	return epoch.Add(time.Duration(s) * time.Second)
}

type Function struct {
	Name       string
	Hash       string
	Class      Class
	State      State
	Visibility []Caller
}

func (f *Function) port(tenant port.TenantID, c port.ConnectorID) port.Function {
	var vis []string
	for _, v := range f.Visibility {
		vis = append(vis, map[Caller]string{Model: port.VisibleToModel, View: port.VisibleToApp}[v])
	}
	state := f.State
	if state == "" {
		state = Active
	}
	return port.Function{Tenant: tenant, Connector: c, Name: f.Name, DefinitionHash: port.Hash(f.Hash), Class: f.Class, State: state, Visibility: vis}
}

func (f *Function) set(p port.Function) {
	f.Hash, f.Class, f.State = string(p.DefinitionHash), p.Class, p.State
}

type Connector struct {
	ID        string
	Listing   string
	Version   string
	Workspace string
	Enabled   bool
	Functions map[string]*Function
}

type Grant struct {
	Scope     Scope
	ScopeID   string
	Agent     string
	Connector string
	Listing   string
	Selector  string
	Mode      Mode
	Hash      string
	ExpiresAt int64
	MaxUses   int
	Uses      int
	Origin    port.Origin
	id        string
}

func ref(id string) port.ListingRef {
	owner, name, _ := strings.Cut(id, "/")
	return port.ListingRef{Owner: port.TenantID(owner), Name: name}
}

func (g *Grant) port(tenant port.TenantID) port.Grant {
	p := port.Grant{
		ID: port.GrantID(g.id), Tenant: tenant, ScopeKind: g.Scope, ScopeID: g.ScopeID, Agent: port.AgentID(g.Agent),
		Connector: port.ConnectorID(g.Connector), Selector: g.Selector, Mode: g.Mode, DefinitionHash: port.Hash(g.Hash),
		ExpiresAt: at(g.ExpiresAt), MaxUses: g.MaxUses, Uses: g.Uses, Origin: g.Origin,
	}
	if p.Origin == "" {
		p.Origin = port.OriginSettings
	}
	if g.Listing != "" {
		r := ref(g.Listing)
		p.Listing = &r
	}
	return p
}

type Query struct {
	Agent     string
	User      string
	Session   string
	Connector string
	Function  string
	HasArgs   bool
	Arguments string
}

type World struct {
	Tenant         string
	Inst           *Installation
	AgentOwner     map[string]string
	Groups         map[string][]string
	Connectors     map[string]*Connector
	Bindings       map[[2]string]bool
	Grants         []*Grant
	EndedSessions  map[string]bool
	Removed        map[string]bool
	Pauses         map[[3]string]bool
	ForceAsk       bool
	ExposedLegC    map[string]map[string]bool
	DestructiveMax Mode
	SessionMax     int64
	External       func(Query) Mode
	mu             sync.Mutex
	seq            int
}

func NewWorld() *World {
	return &World{
		AgentOwner: map[string]string{}, Groups: map[string][]string{}, Connectors: map[string]*Connector{},
		Bindings: map[[2]string]bool{}, EndedSessions: map[string]bool{}, Removed: map[string]bool{},
		Pauses: map[[3]string]bool{}, ExposedLegC: map[string]map[string]bool{}, DestructiveMax: AskOnce, SessionMax: 28800,
	}
}

type Request struct {
	Agent     string
	User      string
	Session   string
	Connector string
	Function  string
	Caller    Caller
	Now       int64
	TokenExp  int64
	Arguments string
}

type Decision struct {
	Mode    Mode
	Reason  string
	Step    int
	Layers  [4]string
	matched []port.GrantID
}

type Outcome int

const (
	UnknownTool Outcome = iota
	Held
	Forwarded
)

func (o Outcome) String() string { return [...]string{"unknown tool", "held", "forwarded"}[o] }

func (w *World) tenant() port.TenantID { return port.TenantID(w.Tenant) }

func (w *World) subject(r Request) port.Subject {
	caller := port.CallerModel
	if r.Caller == View {
		caller = port.CallerView
	}
	return port.Subject{
		Tenant: w.tenant(), Agent: port.AgentID(r.Agent), AgentOwner: port.PersonID(w.AgentOwner[r.Agent]),
		OnBehalfOf: port.PersonID(r.User), Session: port.SessionID(r.Session), Caller: caller,
	}
}

func (w *World) grants() []port.Grant {
	out := make([]port.Grant, 0, len(w.Grants))
	for _, g := range w.Grants {
		if g.id == "" {
			w.seq++
			g.id = fmt.Sprintf("g%d", w.seq)
		}
		out = append(out, g.port(w.tenant()))
	}
	return out
}

func (w *World) facts(r Request) evaluate.Facts {
	f := evaluate.Facts{
		Bound: w.Bindings[[2]string{r.Connector, r.Agent}], SessionEnded: w.EndedSessions[r.Session],
		PersonRemoved: w.Removed[r.User] || w.Removed[w.AgentOwner[r.Agent]],
		Catalog:       w.Inst.view(w.tenant()), Paused: w.Pauses[[3]string{r.Session, r.Agent, r.Connector}],
		Grants: w.grants(), ForceAsk: w.ForceAsk, DestructiveMax: w.DestructiveMax,
	}
	if c, ok := w.Connectors[r.Connector]; ok {
		f.Connector = &port.Connector{
			ID: port.ConnectorID(c.ID), Tenant: w.tenant(), Workspace: port.WorkspaceID(c.Workspace), Enabled: c.Enabled,
			Listing: port.VersionRef{Listing: ref(c.Listing), Version: c.Version},
		}
		f.Functions = map[string]port.Function{}
		f.LegC = map[string]bool{}
		for name, fn := range c.Functions {
			f.Functions[name] = fn.port(w.tenant(), f.Connector.ID)
			f.LegC[name] = w.ExposedLegC[r.Agent][r.Connector+"/"+name]
		}
	}
	if w.External != nil {
		f.External = func(q port.Query) Mode {
			return w.External(Query{Agent: string(q.Agent), User: string(q.OnBehalfOf), Session: string(q.Session), Connector: string(q.Connector),
				Function: q.Function, HasArgs: q.Arguments != nil, Arguments: string(q.Arguments)})
		}
	}
	return f
}

func (w *World) Evaluate(r Request, withArgs bool) Decision {
	d := evaluate.Evaluate(w.facts(r), evaluate.Request{Subject: w.subject(r), Function: r.Function, Now: at(r.Now), Arguments: []byte(r.Arguments), Call: withArgs})
	return Decision{Mode: d.Mode, Reason: string(d.Reason), Step: d.Step, Layers: d.Layers, matched: d.Matched}
}

func (w *World) Listed(agent, user, session string, caller Caller, now int64) []string {
	var out []string
	for cid := range w.Connectors {
		r := Request{Agent: agent, User: user, Session: session, Connector: cid, Caller: caller, Now: now}
		for _, fd := range evaluate.Search(w.facts(r), w.subject(r), at(now)) {
			if fd.Decision.Mode != Deny {
				out = append(out, cid+"__"+fd.Function)
			}
		}
	}
	slices.Sort(out)
	return out
}

func (w *World) reserve(ids []port.GrantID) bool {
	var rows []*Grant
	for _, g := range w.Grants {
		if slices.Contains(ids, port.GrantID(g.id)) && g.MaxUses > 0 {
			if g.Uses >= g.MaxUses {
				return false
			}
			rows = append(rows, g)
		}
	}
	for _, g := range rows {
		g.Uses++
	}
	return true
}

func (w *World) Call(r Request) (Outcome, Decision) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for {
		d := w.Evaluate(r, true)
		switch d.Mode {
		case Deny:
			return UnknownTool, d
		case Ask, AskOnce:
			return Held, d
		}
		if w.reserve(d.matched) {
			return Forwarded, d
		}
	}
}

func (w *World) ExecuteApproved(r Request) (Outcome, Decision) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for {
		d := w.Evaluate(r, true)
		if d.Mode == Deny {
			return UnknownTool, d
		}
		if w.reserve(d.matched) {
			return Forwarded, d
		}
	}
}

var ErrRowRefused = errors.New("row_refused")

func (w *World) PutGrant(g *Grant) error {
	p := g.port(w.tenant())
	if err := grant.Validate(p, grant.Policy{}); err != nil {
		return ErrRowRefused
	}
	for i, old := range w.Grants {
		if grant.Key(old.port(w.tenant())) == grant.Key(p) {
			w.Grants[i] = g
			return nil
		}
	}
	w.Grants = append(w.Grants, g)
	return nil
}

func (w *World) drop(keep func(*Grant) bool) {
	kept := w.Grants[:0]
	for _, g := range w.Grants {
		if keep(g) {
			kept = append(kept, g)
		}
	}
	w.Grants = kept
}

func (w *World) SetLevel(scope Scope, scopeID, connector string, level Level) {
	base := Grant{Scope: scope, ScopeID: scopeID, Connector: connector}
	var layer []port.Grant
	for _, g := range w.Grants {
		if g.Scope == scope && g.ScopeID == scopeID && g.Connector == connector {
			layer = append(layer, g.port(w.tenant()))
		}
	}
	removed, added := grant.SetLevel(layer, base.port(w.tenant()), level, grant.DefaultLevelModes)
	gone := map[string]bool{}
	for _, g := range removed {
		gone[grant.Key(g)] = true
	}
	w.drop(func(g *Grant) bool { return !gone[grant.Key(g.port(w.tenant()))] })
	for _, p := range added {
		if err := w.PutGrant(&Grant{Scope: p.ScopeKind, ScopeID: p.ScopeID, Connector: string(p.Connector), Selector: p.Selector, Mode: p.Mode, Origin: p.Origin}); err != nil {
			panic(err)
		}
	}
}

func (w *World) apply(connector, function string, eff pinning.Effects) {
	if eff.DropSessionRows {
		w.drop(func(g *Grant) bool {
			return !(g.Scope == Session && g.Connector == connector && g.Selector == port.SelectorFunction+function)
		})
	}
}

func (w *World) function(connector, function string) *Function {
	return w.Connectors[connector].Functions[function]
}

func (w *World) Classify(connector, function string, class Class) {
	f := w.function(connector, function)
	p, eff := pinning.Reclassify(f.port(w.tenant(), port.ConnectorID(connector)), class, port.ClassSourceAdmin)
	f.set(p)
	w.apply(connector, function, eff)
}

func (w *World) UpstreamChanged(connector, function, hash string) {
	f := w.function(connector, function)
	p, eff := pinning.Changed(f.port(w.tenant(), port.ConnectorID(connector)), nil, port.Hash(hash))
	f.set(p)
	w.apply(connector, function, eff)
}

func (w *World) UpstreamAdded(connector string, f *Function) {
	f.set(pinning.New(f.port(w.tenant(), port.ConnectorID(connector))))
	w.Connectors[connector].Functions[f.Name] = f
}

func (w *World) Accept(connector, function string) {
	f := w.function(connector, function)
	p, err := pinning.Accept(f.port(w.tenant(), port.ConnectorID(connector)), port.Hash(f.Hash), "admin", epoch)
	if err != nil {
		panic(err)
	}
	f.set(p)
}

func (w *World) Update(connector, version string, fns []Function) {
	c := w.Connectors[connector]
	c.Version = version
	for _, f := range fns {
		old, ok := c.Functions[f.Name]
		switch {
		case !ok:
			w.UpstreamAdded(connector, &Function{Name: f.Name, Hash: f.Hash, Class: f.Class})
		case old.Hash != f.Hash:
			w.UpstreamChanged(connector, f.Name, f.Hash)
		}
	}
}

func (w *World) ConfirmGrant(scope Scope, scopeID, connector, function string) {
	f := w.function(connector, function).port(w.tenant(), port.ConnectorID(connector))
	for _, g := range w.Grants {
		if g.Scope != scope || g.ScopeID != scopeID || (g.Connector != connector && g.Scope != Tenant) {
			continue
		}
		if p, err := grant.Confirm(g.port(w.tenant()), f); err == nil {
			g.Hash = string(p.DefinitionHash)
		}
	}
}

func (w *World) RemovePerson(person string) {
	w.Removed[person] = true
	w.drop(func(g *Grant) bool { return !(g.Scope == User && g.ScopeID == person) })
}

func (w *World) ApproveForSession(r Request) bool {
	d := w.Evaluate(r, true)
	if d.Mode != AskOnce {
		return false
	}
	f := w.function(r.Connector, r.Function)
	a := port.Approval{Tenant: w.tenant(), Session: port.SessionID(r.Session), Agent: port.AgentID(r.Agent), Connector: port.ConnectorID(r.Connector), Function: f.Name, DefinitionHash: port.Hash(f.Hash)}
	p := grant.SessionRow(a, at(r.Now), at(r.TokenExp), time.Duration(w.SessionMax)*time.Second)
	if err := w.PutGrant(&Grant{Scope: Session, ScopeID: p.ScopeID, Agent: string(p.Agent), Connector: string(p.Connector), Selector: p.Selector,
		Mode: p.Mode, Hash: string(p.DefinitionHash), ExpiresAt: int64(p.ExpiresAt.Sub(epoch) / time.Second), Origin: p.Origin}); err != nil {
		panic(err)
	}
	return true
}

func (w *World) sessionRow(r Request, f *Function) bool {
	return evaluate.SessionGranted(w.grants(), w.subject(r), port.ConnectorID(r.Connector), f.port(w.tenant(), port.ConnectorID(r.Connector)), at(r.Now))
}

func (w *World) Bind(connector, agent string) error {
	c, ok := w.Connectors[connector]
	if !ok {
		return ErrNotFound
	}
	if c.Listing != "" {
		var people []port.PersonID
		if o := w.AgentOwner[agent]; o != "" {
			people = append(people, port.PersonID(o))
		}
		if !w.Inst.view(w.tenant()).Resolve(port.WorkspaceID(c.Workspace), people, ref(c.Listing)).Visible {
			return ErrNotFound
		}
	}
	w.Bindings[[2]string{connector, agent}] = true
	return nil
}

func (w *World) Find(agent, user, session string, caller Caller, now int64, q string) []string {
	var out []string
	for _, n := range w.Listed(agent, user, session, caller, now) {
		if strings.Contains(n, q) {
			out = append(out, n)
		}
	}
	return out
}

type CatalogRule struct {
	Tenant      string
	SubjectKind SubjectKind
	SubjectID   string
	TargetKind  TargetKind
	Target      string
	Show        bool
	Install     bool
	Ceiling     Ceiling
	Named       bool
}

type Listing struct {
	Owner     string
	Name      string
	Source    string
	Publisher string
	Starter   bool
	Status    ListingStatus
	Versions  map[string]ListingStatus
	Order     []string
	Functions []Function
}

func (l *Listing) ID() string { return l.Owner + "/" + l.Name }

type Installation struct {
	Listings        map[string]*Listing
	Withheld        map[[2]string]bool
	WithheldListing map[[2]string]bool
	Rules           []*CatalogRule
	TenantDefault   string
	WorkspaceHidden map[[2]string]bool
	Tenants         map[string]*World
}

func NewInstallation() *Installation {
	return &Installation{
		Listings: map[string]*Listing{}, Withheld: map[[2]string]bool{}, WithheldListing: map[[2]string]bool{},
		TenantDefault: "none", WorkspaceHidden: map[[2]string]bool{}, Tenants: map[string]*World{},
	}
}

func (i *Installation) port(id string) port.Listing {
	l := i.Listings[id]
	status := l.Status
	if status == "" {
		status = ListingActive
	}
	return port.Listing{Ref: ref(id), Source: l.Source, Publisher: l.Publisher, Status: status, Starter: l.Starter, Versions: l.Order}
}

func (i *Installation) view(tenant port.TenantID) corecatalog.View {
	v := corecatalog.View{
		Tenant: tenant, Listings: map[port.ListingRef]port.Listing{}, Versions: map[port.VersionRef]port.ListingStatus{},
		Hidden: map[port.WorkspaceID]bool{}, Groups: map[port.PersonID][]port.GroupID{},
	}
	for id, l := range i.Listings {
		v.Listings[ref(id)] = i.port(id)
		for ver, s := range l.Versions {
			v.Versions[port.VersionRef{Listing: ref(id), Version: ver}] = s
		}
	}
	for k, withheld := range i.Withheld {
		v.Offers = append(v.Offers, port.Offer{Tenant: port.TenantID(k[0]), TargetKind: port.TargetSource, Target: k[1], Withheld: withheld})
	}
	for k, withheld := range i.WithheldListing {
		v.Offers = append(v.Offers, port.Offer{Tenant: port.TenantID(k[0]), TargetKind: port.TargetListing, Target: k[1], Withheld: withheld})
	}
	for n, r := range i.Rules {
		audience := port.AudienceAll
		if r.Named {
			audience = port.AudienceNamed
		}
		v.Rules = append(v.Rules, port.CatalogRule{
			ID: port.RuleID(fmt.Sprint(n)), Tenant: port.TenantID(r.Tenant), SubjectKind: r.SubjectKind, SubjectID: r.SubjectID,
			TargetKind: r.TargetKind, Target: r.Target, Show: r.Show, Install: r.Install, Ceiling: ceilingOf(r.Ceiling), Audience: audience,
		})
	}
	for k, h := range i.WorkspaceHidden {
		if port.TenantID(k[0]) == tenant {
			v.Hidden[port.WorkspaceID(k[1])] = h
		}
	}
	if w, ok := i.Tenants[string(tenant)]; ok {
		for p, gs := range w.Groups {
			for _, g := range gs {
				v.Groups[port.PersonID(p)] = append(v.Groups[port.PersonID(p)], port.GroupID(g))
			}
		}
	}
	return v
}

func ceilingOf(c Ceiling) Ceiling {
	if c == "" {
		return CeilRead
	}
	return c
}

func (i *Installation) AddListing(l *Listing) {
	i.Listings[l.ID()] = l
	if l.Owner != "" {
		r := corecatalog.PrivateRule(ref(l.ID()), CeilWrite)
		i.Rules = append(i.Rules, &CatalogRule{Tenant: string(r.Tenant), SubjectKind: r.SubjectKind, SubjectID: r.SubjectID, TargetKind: r.TargetKind, Target: r.Target, Show: r.Show, Install: r.Install, Ceiling: r.Ceiling})
	}
}

func (i *Installation) Import(id, version string) {
	l := i.Listings[id]
	if l.Versions == nil {
		l.Versions = map[string]ListingStatus{}
	}
	if _, ok := l.Versions[version]; !ok {
		l.Order = append(l.Order, version)
	}
	l.Versions[version] = ListingActive
}

func (i *Installation) CreateTenant(id string) *World {
	if _, ok := i.Tenants[id]; !ok && i.TenantDefault == "starter" {
		var all []port.Listing
		for lid := range i.Listings {
			all = append(all, i.port(lid))
		}
		for _, r := range corecatalog.StarterRules(port.TenantID(id), all) {
			i.Rules = append(i.Rules, &CatalogRule{Tenant: id, SubjectKind: r.SubjectKind, SubjectID: r.SubjectID, TargetKind: r.TargetKind, Target: r.Target, Show: r.Show, Install: r.Install, Ceiling: r.Ceiling})
		}
	}
	return i.Tenant(id)
}

func (i *Installation) Tenant(id string) *World {
	if w, ok := i.Tenants[id]; ok {
		return w
	}
	w := NewWorld()
	w.Tenant, w.Inst = id, i
	i.Tenants[id] = w
	return w
}

func (i *Installation) people(people []string) []port.PersonID {
	var out []port.PersonID
	for _, p := range people {
		out = append(out, port.PersonID(p))
	}
	return out
}

func (i *Installation) Visible(tenant, workspace string, people []string, id string) (bool, Ceiling, bool) {
	a := i.view(port.TenantID(tenant)).Resolve(port.WorkspaceID(workspace), i.people(people), ref(id))
	return a.Visible, a.Ceiling, a.Installable
}

func (i *Installation) Store(tenant, workspace, person, q string) []string {
	var out []string
	for _, r := range i.view(port.TenantID(tenant)).Visible(port.WorkspaceID(workspace), i.people([]string{person})) {
		if strings.Contains(r.Name, q) {
			out = append(out, r.String())
		}
	}
	return out
}

var (
	ErrNotFound      = errors.New("not_found")
	ErrDeprecated    = errors.New("listing_deprecated")
	ErrPolicyRefused = errors.New("policy_subject_refused")
)

func (i *Installation) PutCatalogPolicy(tenant string, kind SubjectKind, subject string, hidden bool) error {
	if corecatalog.ValidatePolicy(kind) != nil {
		return ErrPolicyRefused
	}
	i.WorkspaceHidden[[2]string{tenant, subject}] = hidden
	return nil
}

func (i *Installation) GetListing(tenant, workspace, person, id string) (*Listing, error) {
	if show, _, _ := i.Visible(tenant, workspace, []string{person}, id); !show {
		return nil, ErrNotFound
	}
	return i.Listings[id], nil
}

func (i *Installation) Install(tenant, workspace, person, id, alias string) error {
	v := i.view(port.TenantID(tenant))
	a := v.Resolve(port.WorkspaceID(workspace), i.people([]string{person}), ref(id))
	switch {
	case !a.Visible:
		return ErrNotFound
	case v.Status(ref(id), a.Version) == ListingDeprecated:
		return ErrDeprecated
	case !a.Installable:
		return ErrNotFound
	}
	c := &Connector{ID: alias, Enabled: true, Listing: id, Version: a.Version, Workspace: workspace, Functions: map[string]*Function{}}
	for _, f := range i.Listings[id].Functions {
		f.State = Active
		c.Functions[f.Name] = &f
	}
	i.Tenant(tenant).Connectors[alias] = c
	return nil
}

func mode(c Ceiling, class Class) Mode { return corecatalog.CeilingMode(c, class) }
