package pinning

import (
	"cmp"
	"slices"
	"time"

	"github.com/agents-control/connector-engine/port"
)

// Effects is what a function transition asks the use case to do in the same transaction.
type Effects struct {
	DropSessionRows bool
	Cancel          port.Code
}

// ChangeKind names what a re-list or an update found for one function (§9).
type ChangeKind string

// Kinds of change of §9.
const (
	ChangeChanged  ChangeKind = "changed"
	ChangeNew      ChangeKind = "new"
	ChangeRemoved  ChangeKind = "removed"
	ChangeReturned ChangeKind = "returned"
)

// Change is one difference between the pinned functions and a capture.
type Change struct {
	Function string
	Kind     ChangeKind
	OldHash  port.Hash
	NewHash  port.Hash
	Effects  Effects
}

// Captured sets the state of a function captured at install: active for a reviewed or first_party listing, else quarantined (§5.3).
func Captured(f port.Function, trust port.TrustLevel) port.Function {
	f.State = port.FunctionQuarantined
	if trust == port.TrustReviewed || trust == port.TrustFirstParty {
		f.State = port.FunctionActive
	}
	return f
}

// New quarantines a function new to the connector, whatever a review says (§5.3, OD-6).
func New(f port.Function) port.Function {
	f.State = port.FunctionQuarantined
	return f
}

// Changed pins a new definition: quarantined, session rows dropped, pending approvals cancelled (§9).
func Changed(f port.Function, definition []byte, hash port.Hash) (port.Function, Effects) {
	f.Definition, f.DefinitionHash = definition, hash
	return quarantine(f)
}

func quarantine(f port.Function) (port.Function, Effects) {
	f.State = port.FunctionQuarantined
	return f, Effects{DropSessionRows: true, Cancel: port.ReasonQuarantined}
}

// Reconcile compares the pinned functions with a capture and returns every function after the change and the changes (§5.3, §9).
func Reconcile(pinned, captured []port.Function) ([]port.Function, []Change) {
	byName := map[string]port.Function{}
	for _, f := range pinned {
		byName[f.Name] = f
	}
	seen := map[string]bool{}
	var out []port.Function
	var changes []Change
	for _, c := range captured {
		seen[c.Name] = true
		old, ok := byName[c.Name]
		switch {
		case !ok:
			out = append(out, New(c))
			changes = append(changes, Change{Function: c.Name, Kind: ChangeNew, NewHash: c.DefinitionHash})
		case old.State == port.FunctionRemoved && old.DefinitionHash == c.DefinitionHash:
			old.State, old.PreviousState = old.PreviousState, ""
			out = append(out, old)
			changes = append(changes, Change{Function: c.Name, Kind: ChangeReturned, OldHash: old.DefinitionHash, NewHash: c.DefinitionHash})
		case old.DefinitionHash != c.DefinitionHash:
			f, eff := Changed(old, c.Definition, c.DefinitionHash)
			f.PreviousState = ""
			out = append(out, f)
			changes = append(changes, Change{Function: c.Name, Kind: ChangeChanged, OldHash: old.DefinitionHash, NewHash: c.DefinitionHash, Effects: eff})
		default:
			out = append(out, old)
		}
	}
	for _, f := range pinned {
		if seen[f.Name] {
			continue
		}
		if f.State != port.FunctionRemoved {
			f.PreviousState, f.State = f.State, port.FunctionRemoved
			changes = append(changes, Change{Function: f.Name, Kind: ChangeRemoved, OldHash: f.DefinitionHash, Effects: Effects{Cancel: port.ReasonRemoved}})
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b port.Function) int { return cmp.Compare(a.Name, b.Name) })
	return out, changes
}

// Accept clears a quarantine when the admin names the hash they saw (§9 "Clearing").
func Accept(f port.Function, seen port.Hash, by string, at time.Time) (port.Function, error) {
	switch {
	case f.State != port.FunctionQuarantined:
		return f, port.Errorf(port.CodeInvalidTransition, "function %s is %s, not quarantined", f.Name, f.State)
	case seen != f.DefinitionHash:
		return f, port.Errorf(port.CodeDefinitionChanged, "function %s is at another hash than the one seen", f.Name)
	}
	f.State, f.AcceptedBy, f.AcceptedAt = port.FunctionActive, by, at
	return f, nil
}

// Reclassify sets a class; a raise drops the session rows and cancels pending approvals on the function (§6.4 G7).
func Reclassify(f port.Function, class port.Class, source port.ClassSource) (port.Function, Effects) {
	raised := f.Class.Raises(class)
	f.Class, f.ClassSource = class, source
	if raised {
		return f, Effects{DropSessionRows: true, Cancel: port.CodeClassChanged}
	}
	return f, Effects{}
}

// AutoAccept reports whether an update activates a new hash without an admin: only with pinning.autoAcceptReviewed = read,
// a hash the new review covers and a reviewed class read (§9).
func AutoAccept(f port.Function, review *port.Review, setting string) bool {
	return setting == "read" && review != nil && slices.Contains(review.DefinitionHashes, f.DefinitionHash) &&
		f.ClassSource == port.ClassSourceReviewed && f.Class == port.ClassRead
}
