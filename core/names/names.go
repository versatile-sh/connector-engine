// Package names builds the exposed names <alias>__<function> and the lookup table that resolves them without splitting (§10.1).
package names

import (
	"regexp"
	"slices"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

// Defaults of gateway.separator and gateway.toolNameMax (§14).
const (
	Separator = "__"
	MaxLength = 64
)

var alias = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidAlias refuses an alias outside [a-z0-9-], so it never contains the separator.
func ValidAlias(a string) error {
	if !alias.MatchString(a) {
		return port.Errorf(port.CodeInvalidAlias, "alias %q is not [a-z0-9-]+", a)
	}
	return nil
}

// Exposed returns <alias><sep><function>; an install that passes max asks for a shorter alias (name_too_long).
func Exposed(a, function, sep string, max int) (string, error) {
	if err := ValidAlias(a); err != nil {
		return "", err
	}
	n := a + sep + function
	if len([]rune(n)) > max {
		return "", port.Errorf(port.CodeNameTooLong, "%s is longer than %d characters", n, max)
	}
	return n, nil
}

// Entry is what an exposed name resolves to.
type Entry struct {
	Connector port.ConnectorID
	Function  string
}

// Table resolves exposed names by lookup, never by splitting.
type Table struct {
	byName map[string]Entry
	order  []string
}

// Build makes the table of an agent's functions; a name that two entries would share is refused.
func Build(sep string, max int, aliases map[port.ConnectorID]string, entries []Entry) (Table, error) {
	t := Table{byName: map[string]Entry{}}
	for _, e := range entries {
		n, err := Exposed(aliases[e.Connector], e.Function, sep, max)
		if err != nil {
			return Table{}, err
		}
		if old, ok := t.byName[n]; ok && old != e {
			return Table{}, port.Errorf(port.CodeConflict, "%s names two functions", n)
		}
		t.byName[n] = e
	}
	for n := range t.byName {
		t.order = append(t.order, n)
	}
	slices.Sort(t.order)
	return t, nil
}

// Resolve returns the entry of an exposed name.
func (t Table) Resolve(name string) (Entry, bool) {
	e, ok := t.byName[name]
	return e, ok
}

// Names lists the exposed names in a deterministic order (§10.2 L4).
func (t Table) Names() []string { return slices.Clone(t.order) }

// ResourceURI returns <alias>+<uri>, the URI an agent sees for a connector's resource; an alias must start with a letter to lead a URI scheme (RFC 3986).
func ResourceURI(a, uri string) (string, bool) {
	if a == "" || a[0] < 'a' || a[0] > 'z' {
		return "", false
	}
	return a + "+" + uri, true
}

// SplitResourceURI returns the alias and the upstream URI of an agent-facing resource URI.
func SplitResourceURI(s string) (a, uri string, ok bool) {
	a, uri, ok = strings.Cut(s, "+")
	return a, uri, ok && ValidAlias(a) == nil && uri != ""
}
