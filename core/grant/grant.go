// Package grant holds the rules of the grant table: row legality, keys, levels and matching inside one layer (§6, §7.1 M1 to M5).
package grant

import (
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
)

// Specificity ranks a selector: 2 for fn:, 1 for class:, 0 for * (§7.1 M2).
func Specificity(selector string) int {
	switch {
	case strings.HasPrefix(selector, port.SelectorFunction):
		return 2
	case strings.HasPrefix(selector, port.SelectorClass):
		return 1
	}
	return 0
}

// Covers reports whether a selector covers the function by name, by effective class or as * (§7.1 M1).
func Covers(selector string, f port.Function) bool {
	return selector == port.SelectorAll || selector == port.SelectorClass+string(f.Class.Effective()) || selector == port.SelectorFunction+f.Name
}

// Key is the uniqueness key of a row: writing a row with an existing key replaces it (§6.4 G1).
func Key(g port.Grant) string {
	target := string(g.Connector)
	if g.Listing != nil {
		target = "listing:" + g.Listing.String()
	}
	return strings.Join([]string{string(g.Tenant), string(g.ScopeKind), g.ScopeID, string(g.Agent), target, g.Selector}, "\x00")
}

// Policy bounds settings-made allow rows of the agent and user scopes; a zero AlwaysMax skips the bound (§6.1).
type Policy struct {
	Now       time.Time
	AlwaysMax time.Duration
}

// Validate refuses a row the rules of §6.1 and §6.4 G5, G8, G9 and G10 forbid, with code row_refused.
func Validate(g port.Grant, p Policy) error {
	refuse := func(why string) error { return port.Errorf(port.CodeRowRefused, "%s", why) }
	sel := g.Selector
	if sel != port.SelectorAll && !validClass(sel) && !(strings.HasPrefix(sel, port.SelectorFunction) && len(sel) > len(port.SelectorFunction)) {
		return refuse("unknown selector " + sel)
	}
	if g.Mode.Rank() < 0 && g.Mode != port.ModeInherit {
		return refuse("unknown mode " + string(g.Mode))
	}
	if g.MaxUses < 0 || (g.MaxUses > 0 && (g.Mode == port.ModeDeny || g.Mode == port.ModeAsk)) {
		return refuse("max_uses is refused on deny and ask rows (G9)")
	}
	if g.ScopeKind == port.ScopeTenant {
		switch {
		case g.Mode == port.ModeAllow:
			return refuse("a tenant row never says allow (G8)")
		case g.DefinitionHash != "" || !g.ExpiresAt.IsZero() || g.MaxUses != 0:
			return refuse("a tenant row carries no hash, expiry or max_uses (G8)")
		case g.Connector != "" || g.Listing == nil:
			return refuse("a tenant row names a listing, never a connector (G8)")
		case g.Mode == port.ModeInherit && Specificity(sel) != 2:
			return refuse("inherit is accepted on an fn: selector only (G10)")
		}
		return nil
	}
	switch {
	case g.Mode == port.ModeInherit:
		return refuse("inherit exists only on a tenant fn: row (G10)")
	case g.Listing != nil || g.Connector == "":
		return refuse("a row below the tenant names a connector")
	case Specificity(sel) == 2 && g.DefinitionHash == "":
		return refuse("an fn: row carries the definition_hash the grantor saw (§6.1)")
	}
	switch g.ScopeKind {
	case port.ScopeWorkspace, port.ScopeAgent, port.ScopeUser:
	case port.ScopeSession:
		if g.Agent == "" || g.ExpiresAt.IsZero() || g.Origin != port.OriginApproval || Specificity(sel) != 2 {
			return refuse("a session row is an fn: row written by an approval, with an agent and an expiry (G5)")
		}
		return nil
	default:
		return refuse("unknown scope " + string(g.ScopeKind))
	}
	if p.AlwaysMax > 0 && g.Mode == port.ModeAllow && g.Origin == port.OriginSettings && g.ScopeKind != port.ScopeWorkspace {
		if g.ExpiresAt.IsZero() || g.ExpiresAt.After(p.Now.Add(p.AlwaysMax)) {
			return refuse("an allow row of an agent or a person expires within permission.alwaysMaxDuration (§6.1)")
		}
	}
	return nil
}

func validClass(sel string) bool {
	c, ok := strings.CutPrefix(sel, port.SelectorClass)
	return ok && (c == string(port.ClassRead) || c == string(port.ClassWrite) || c == string(port.ClassDestructive))
}

// LevelModes is the expansion of level write, configured as permission.levelWrite.* (§6.2).
type LevelModes struct {
	Write       port.Mode
	Destructive port.Mode
}

// DefaultLevelModes is the recommended expansion of level write: write ask_once, destructive ask (OD-4).
var DefaultLevelModes = LevelModes{Write: port.ModeAskOnce, Destructive: port.ModeAsk}

// Expand returns the class rows a level writes on base's layer; custom writes none (§6.2).
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

// SetLevel returns the rows of one layer a level deletes and the rows it writes: level rows and same-key rows go (§6.4 G3).
func SetLevel(layer []port.Grant, base port.Grant, level port.Level, m LevelModes) (removed, added []port.Grant) {
	added = Expand(base, level, m)
	keys := map[string]bool{}
	for _, g := range added {
		keys[Key(g)] = true
	}
	for _, g := range layer {
		if g.Origin == port.OriginLevel || keys[Key(g)] {
			removed = append(removed, g)
		}
	}
	return removed, added
}

// Answer is what one layer yields for a function (§7.1 steps 6 to 9).
type Answer struct {
	Found  bool
	Mode   port.Mode
	Reason port.Code
	Row    *port.Grant
}

// Live reports whether a row is unexpired and, when counted, under max_uses (§7.1 M1).
func Live(g port.Grant, now time.Time) bool {
	return (g.ExpiresAt.IsZero() || now.Before(g.ExpiresAt)) && (g.MaxUses == 0 || g.Uses < g.MaxUses)
}

// Match applies M1 to M5 to the rows of one layer; an inherit answer is no answer (§6.4 G10).
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

// Stale reports whether an fn: allow or ask_once row was written at another hash than the function's (§7.1 M4, M5).
func Stale(g port.Grant, f port.Function) bool {
	return (g.Mode == port.ModeAllow || g.Mode == port.ModeAskOnce) && g.DefinitionHash != f.DefinitionHash
}

// SessionRow builds the session row of A4: fn:F allow at the current hash, ending at the earlier of exp and now plus max (§6.4 G5).
func SessionRow(a port.Approval, now, tokenExp time.Time, max time.Duration) port.Grant {
	exp := now.Add(max)
	if !tokenExp.IsZero() && tokenExp.Before(exp) {
		exp = tokenExp
	}
	return port.Grant{
		Tenant: a.Tenant, ScopeKind: port.ScopeSession, ScopeID: string(a.Session), Agent: a.Agent, Connector: a.Connector,
		Selector: port.SelectorFunction + a.Function, Mode: port.ModeAllow, DefinitionHash: a.DefinitionHash,
		ExpiresAt: exp, CreatedBy: a.DecidedBy, CreatedAt: now, Origin: port.OriginApproval,
	}
}

// Capped lists the functions a row with mode allow covers whose effective class is destructive (§6.2 warning).
func Capped(g port.Grant, fns []port.Function) []string {
	var out []string
	for _, f := range fns {
		if g.Mode == port.ModeAllow && Covers(g.Selector, f) && f.Class.Effective() == port.ClassDestructive {
			out = append(out, f.Name)
		}
	}
	return out
}

// Confirm moves an fn: row to the function's current hash; a tenant row is never stale and has nothing to confirm (§9).
func Confirm(g port.Grant, f port.Function) (port.Grant, error) {
	if g.ScopeKind == port.ScopeTenant || g.Selector != port.SelectorFunction+f.Name {
		return g, port.Errorf(port.CodeInvalidArgument, "only an fn: row below the tenant on %s is confirmed", f.Name)
	}
	g.DefinitionHash = f.DefinitionHash
	return g, nil
}
