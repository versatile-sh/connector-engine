// Package grant holds the rules of the grant table: row legality, keys, levels and matching inside one layer.
package grant

import (
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

// Specificity ranks a selector: 2 for fn: and an exact item, 1 for class: and an item kind's wildcard, 0 for *.
func Specificity(selector string) int {
	kind := port.ItemKind(selector)
	switch {
	case strings.HasPrefix(selector, port.SelectorFunction), kind != "" && selector != kind+port.SelectorAll:
		return 2
	case strings.HasPrefix(selector, port.SelectorClass), kind != "":
		return 1
	}
	return 0
}

// Covers reports whether a selector covers the function by name, by effective class or as *; an item is covered only by its name or its kind's wildcard.
func Covers(selector string, f port.Function) bool {
	if kind := port.ItemKind(f.Name); kind != "" {
		return selector == f.Name || selector == kind+port.SelectorAll
	}
	return selector == port.SelectorAll || selector == port.SelectorClass+string(f.Class.Effective()) || selector == port.SelectorFunction+f.Name
}

// Policy bounds settings-made allow rows of the agent and user scopes; a zero AlwaysMax skips the bound.
type Policy struct {
	Now       time.Time
	AlwaysMax time.Duration
}

// Validate refuses, with code row_refused, a row whose selector, mode, scope, agent, hash, expiry or max_uses the grant rules forbid.
func Validate(g port.Grant, p Policy) error {
	refuse := func(why string) error { return port.Errorf(port.CodeRowRefused, "%s", why) }
	if g.Agent != "" && g.ScopeKind != port.ScopeSession && (g.ScopeKind != port.ScopeAgent || string(g.Agent) != g.ScopeID) {
		return refuse("only a session row names an agent; an agent row may repeat its own")
	}
	sel := g.Selector
	if kind := port.ItemKind(sel); sel != port.SelectorAll && !validClass(sel) && !(strings.HasPrefix(sel, port.SelectorFunction) && len(sel) > len(port.SelectorFunction)) && !(kind != "" && len(sel) > len(kind)) {
		return refuse("unknown selector " + sel)
	}
	if g.Mode.Rank() < 0 && g.Mode != port.ModeInherit {
		return refuse("unknown mode " + string(g.Mode))
	}
	if g.MaxUses < 0 || (g.MaxUses > 0 && (g.Mode == port.ModeDeny || g.Mode == port.ModeAsk)) {
		return refuse("max_uses is refused on deny and ask rows")
	}
	if g.ScopeKind == port.ScopeTenant {
		switch {
		case g.Mode == port.ModeAllow:
			return refuse("a tenant row never says allow")
		case g.DefinitionHash != "" || !g.ExpiresAt.IsZero() || g.MaxUses != 0:
			return refuse("a tenant row carries no hash, expiry or max_uses")
		case g.Connector != "" || g.Listing == nil:
			return refuse("a tenant row names a listing, never a connector")
		case g.Mode == port.ModeInherit && Specificity(sel) != 2:
			return refuse("inherit is accepted on an fn: selector only")
		}
		return nil
	}
	switch {
	case g.Mode == port.ModeInherit:
		return refuse("inherit exists only on a tenant fn: row")
	case g.Listing != nil || g.Connector == "":
		return refuse("a row below the tenant names a connector")
	case Specificity(sel) == 2 && g.DefinitionHash == "":
		return refuse("an fn: row carries the definition_hash the grantor saw")
	}
	switch g.ScopeKind {
	case port.ScopeWorkspace, port.ScopeAgent, port.ScopeUser:
	case port.ScopeSession:
		if g.Agent == "" || g.ExpiresAt.IsZero() || g.Origin != port.OriginApproval || Specificity(sel) != 2 {
			return refuse("a session row is an fn: row written by an approval, with an agent and an expiry")
		}
		return nil
	default:
		return refuse("unknown scope " + string(g.ScopeKind))
	}
	if p.AlwaysMax > 0 && g.Mode == port.ModeAllow && g.Origin == port.OriginSettings && g.ScopeKind != port.ScopeWorkspace {
		if g.ExpiresAt.IsZero() || g.ExpiresAt.After(p.Now.Add(p.AlwaysMax)) {
			return refuse("an allow row of an agent or a person expires within permission.alwaysMaxDuration")
		}
	}
	return nil
}

func validClass(sel string) bool {
	c, ok := strings.CutPrefix(sel, port.SelectorClass)
	return ok && (c == string(port.ClassRead) || c == string(port.ClassWrite) || c == string(port.ClassDestructive))
}

// LevelModes is the expansion of level write, configured as permission.levelWrite.*.
type LevelModes struct {
	Write       port.Mode
	Destructive port.Mode
}

// DefaultLevelModes is the recommended expansion of level write: write ask_once, destructive ask.
var DefaultLevelModes = LevelModes{Write: port.ModeAskOnce, Destructive: port.ModeAsk}

// Expand returns the class rows a level writes on base's layer; custom writes none.
func Expand(base port.Grant, level port.Level, m LevelModes) []port.Grant {
	modes := map[port.Level][3]port.Mode{
		port.LevelOff:   {port.ModeDeny, port.ModeDeny, port.ModeDeny},
		port.LevelRead:  {port.ModeAllow, port.ModeDeny, port.ModeDeny},
		port.LevelWrite: {port.ModeAllow, m.Write, m.Destructive},
	}[level]
	if modes[0] == "" {
		return nil
	}
	var out []port.Grant
	for i, c := range []port.Class{port.ClassRead, port.ClassWrite, port.ClassDestructive} {
		g := base
		g.Selector, g.Mode, g.Origin = port.SelectorClass+string(c), modes[i], port.OriginLevel
		g.DefinitionHash, g.ExpiresAt, g.MaxUses, g.Uses = "", time.Time{}, 0, 0
		out = append(out, g)
	}
	return out
}

// SetLevel returns the rows of one layer a level deletes and the rows it writes: level rows and same-key rows go.
func SetLevel(layer []port.Grant, base port.Grant, level port.Level, m LevelModes) (removed, added []port.Grant) {
	added = Expand(base, level, m)
	keys := map[string]bool{}
	for _, g := range added {
		keys[g.Key()] = true
	}
	for _, g := range layer {
		if g.Origin == port.OriginLevel || keys[g.Key()] {
			removed = append(removed, g)
		}
	}
	return removed, added
}

// Answer is what one layer yields for a function.
type Answer struct {
	Found  bool
	Mode   port.Mode
	Reason port.Code
	Row    *port.Grant
}

// Live reports whether a row is unexpired and, when counted, under max_uses.
func Live(g port.Grant, now time.Time) bool {
	return (g.ExpiresAt.IsZero() || now.Before(g.ExpiresAt)) && (g.MaxUses == 0 || g.Uses < g.MaxUses)
}

// Match answers with the live row of one layer that covers the function, most specific first and strictest on a tie; an inherit answer is no answer.
func Match(rows []port.Grant, f port.Function, now time.Time) Answer {
	var best *port.Grant
	top := -1
	for i := range rows {
		g := &rows[i]
		if !Live(*g, now) || !Covers(g.Selector, f) {
			continue
		}
		s := Specificity(g.Selector)
		if s > top || (s == top && strictness(g.Mode) < strictness(best.Mode)) {
			top, best = s, g
		}
	}
	switch {
	case best == nil || best.Mode == port.ModeInherit:
		return Answer{}
	case top == 2 && best.ScopeKind != port.ScopeTenant && Stale(*best, f):
		return Answer{Found: true, Mode: port.ModeDeny, Reason: port.ReasonGrantStale, Row: best}
	}
	return Answer{Found: true, Mode: best.Mode, Row: best}
}

func strictness(m port.Mode) int {
	if m == port.ModeInherit {
		return 4
	}
	return m.Rank()
}

// Stale reports whether an fn: allow or ask_once row was written at another hash than the function's.
func Stale(g port.Grant, f port.Function) bool {
	return (g.Mode == port.ModeAllow || g.Mode == port.ModeAskOnce) && g.DefinitionHash != f.DefinitionHash
}

// SessionRow builds the row an approval for the session writes: fn:F allow at the current hash, ending at the earlier of exp and now plus max.
func SessionRow(a port.Approval, now, tokenExp time.Time, max time.Duration) port.Grant {
	exp := now.Add(max)
	if !tokenExp.IsZero() && tokenExp.Before(exp) {
		exp = tokenExp
	}
	return port.Grant{
		Tenant: a.Tenant, ScopeKind: port.ScopeSession, ScopeID: string(a.Session), Agent: a.Agent, Connector: a.Connector,
		Selector: port.SelectorOf(a.Function), Mode: port.ModeAllow, DefinitionHash: a.DefinitionHash,
		ExpiresAt: exp, CreatedBy: a.DecidedBy, CreatedAt: now, Origin: port.OriginApproval,
	}
}

// Capped lists the functions a row with mode allow covers whose effective class is destructive.
func Capped(g port.Grant, fns []port.Function) []string {
	var out []string
	for _, f := range fns {
		if g.Mode == port.ModeAllow && Covers(g.Selector, f) && f.Class.Effective() == port.ClassDestructive {
			out = append(out, f.Name)
		}
	}
	return out
}

// Confirm moves an fn: row to the function's current hash; a tenant row is never stale and has nothing to confirm.
func Confirm(g port.Grant, f port.Function) (port.Grant, error) {
	if g.ScopeKind == port.ScopeTenant || g.Selector != port.SelectorOf(f.Name) {
		return g, port.Errorf(port.CodeInvalidArgument, "only an fn: row below the tenant on %s is confirmed", f.Name)
	}
	g.DefinitionHash = f.DefinitionHash
	return g, nil
}

// Normalize clears agent on every row but a session row: evaluation reads it nowhere else, so the row answers as before.
func Normalize(g port.Grant) port.Grant {
	if g.ScopeKind != port.ScopeSession {
		g.Agent = ""
	}
	return g
}

// ConflictReason is the reason of the deny that replaces rows which came to share a key and none of which decides every instant.
const ConflictReason = "rows_conflicted"

// Canonical normalizes rows; of rows that then share a key, those dead at now go and the rest become the one that decides every later instant, or one permanent deny.
func Canonical(rows []port.Grant, now time.Time) []port.Grant {
	groups := map[string][]port.Grant{}
	var keys []string
	for _, g := range rows {
		g = Normalize(g)
		k := g.Key()
		if groups[k] == nil {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], g)
	}
	out := make([]port.Grant, 0, len(keys))
	for _, k := range keys {
		out = append(out, merged(groups[k], now))
	}
	return out
}

func merged(rows []port.Grant, now time.Time) port.Grant {
	slices.SortFunc(rows, func(a, b port.Grant) int { return strings.Compare(string(a.ID), string(b.ID)) })
	live := slices.DeleteFunc(slices.Clone(rows), func(g port.Grant) bool { return !Live(g, now) })
	switch len(live) {
	case 0:
		return rows[0]
	case 1:
		return live[0]
	}
	for _, r := range live {
		if decides(r, live) {
			return r
		}
	}
	g := live[0]
	g.Mode, g.ExpiresAt, g.MaxUses, g.Uses, g.Origin = port.ModeDeny, time.Time{}, 0, 0, port.OriginSettings
	g.CreatedBy, g.CreatedAt, g.Reason = "system", now, ConflictReason
	return g
}

func decides(r port.Grant, rows []port.Grant) bool {
	if r.MaxUses != 0 {
		return false
	}
	for _, s := range rows {
		switch {
		case strictness(s.Mode) < strictness(r.Mode),
			s.Mode == r.Mode && (r.Mode == port.ModeAllow || r.Mode == port.ModeAskOnce) && s.DefinitionHash != r.DefinitionHash,
			!r.ExpiresAt.IsZero() && (s.ExpiresAt.IsZero() || s.ExpiresAt.After(r.ExpiresAt)):
			return false
		}
	}
	return true
}

// Rewrite stores Canonical of the rows tx lists, deleting merged rows first; each merge is audited with its original rows, and cleared agents with a count.
func Rewrite(tx port.Tx, now time.Time) error {
	rows, err := tx.ListGrants(port.GrantFilter{})
	if err != nil || len(rows) == 0 {
		return err
	}
	out := Canonical(rows, now)
	kept, byKey := make(map[port.GrantID]port.Grant, len(out)), make(map[string]port.Grant, len(out))
	for _, g := range out {
		kept[g.ID], byKey[g.Key()] = g, g
	}
	group := map[port.GrantID][]port.Grant{}
	for _, g := range rows {
		id := byKey[Normalize(g).Key()].ID
		group[id] = append(group[id], g)
	}
	var changed []port.Grant
	cleared := 0
	for _, g := range rows {
		k, ok := kept[g.ID]
		switch {
		case !ok:
			if err := tx.DeleteGrant(g.ID); err != nil {
				return err
			}
		case !reflect.DeepEqual(k, g):
			changed = append(changed, k)
			if len(group[g.ID]) == 1 {
				cleared++
			}
		}
	}
	for _, g := range changed {
		if err := tx.PutGrant(g); err != nil {
			return err
		}
	}
	for _, g := range out {
		if len(group[g.ID]) > 1 {
			if err := tx.Audit(port.AuditRow{At: now, Tenant: g.Tenant, Actor: "system", Action: "grant.merged", Subject: string(g.ID),
				Detail: map[string]any{"rows": group[g.ID], "kept": g}}); err != nil {
				return err
			}
		}
	}
	if cleared == 0 {
		return nil
	}
	return tx.Audit(port.AuditRow{At: now, Tenant: rows[0].Tenant, Actor: "system", Action: "grant.agent_cleared", Subject: "grants", Detail: map[string]any{"count": cleared}})
}

// RewriteTenants runs Rewrite at now once per tenant, in the unit of work in returns for it.
func RewriteTenants(tenants []port.TenantID, in func(port.TenantID) (port.Tx, error), now time.Time) error {
	for _, tenant := range tenants {
		tx, err := in(tenant)
		if err != nil {
			return err
		}
		if err := Rewrite(tx, now); err != nil {
			return err
		}
	}
	return nil
}
