package port

import "slices"

// Match reports whether c passes the filter.
func (f ConnectorFilter) Match(c Connector) bool {
	return (f.Workspace == "" || c.Workspace == f.Workspace) && (f.Listing == nil || c.Listing.Listing == *f.Listing)
}

// Match reports whether b passes the filter.
func (f BindingFilter) Match(b Binding) bool {
	return (f.Connector == "" || b.Connector == f.Connector) && (f.Agent == "" || b.Agent == f.Agent) && (f.Owner == "" || b.AgentOwner == f.Owner)
}

// Match reports whether g passes the filter.
func (f GrantFilter) Match(g Grant) bool {
	return (f.ScopeKind == "" || g.ScopeKind == f.ScopeKind) && (f.ScopeID == "" || g.ScopeID == f.ScopeID) &&
		(f.Agent == "" || g.Agent == f.Agent) && (f.Connector == "" || g.Connector == f.Connector) &&
		(f.Listing == nil || (g.Listing != nil && *g.Listing == *f.Listing))
}

// Match reports whether a passes the filter.
func (f ApprovalFilter) Match(a Approval) bool {
	return (len(f.States) == 0 || slices.Contains(f.States, a.State)) && (f.Session == "" || a.Session == f.Session) &&
		(f.Agent == "" || a.Agent == f.Agent) && (f.Connector == "" || a.Connector == f.Connector) && (f.CallHash == "" || a.CallHash == f.CallHash)
}

// Match reports whether c passes the filter.
func (f CallFilter) Match(c Call) bool {
	return (f.Agent == "" || c.Agent == f.Agent) && (f.Session == "" || c.Session == f.Session) &&
		(f.Connector == "" || c.Connector == f.Connector) && (f.State == "" || c.State == f.State)
}

// Match reports whether c passes the filter.
func (f CredentialFilter) Match(c Credential) bool {
	return (f.OwnerKind == "" || c.OwnerKind == f.OwnerKind) && (f.OwnerID == "" || c.OwnerID == f.OwnerID) && (f.Connector == "" || c.Connector == f.Connector)
}

// Targets reports whether o is an overlay of function on the connector, or on the listing when listing is set.
func (o Overlay) Targets(connector ConnectorID, listing *ListingRef, function string) bool {
	if o.Function != function || o.Connector != connector {
		return false
	}
	if listing == nil || o.Listing == nil {
		return listing == o.Listing
	}
	return *o.Listing == *listing
}

// Match reports whether r passes the filter.
func (f RoleFilter) Match(r RoleAssignment) bool { return f.Person == "" || r.Person == f.Person }

// Match reports whether m passes the filter.
func (f MemberFilter) Match(m GroupMember) bool {
	return (f.Person == "" || m.Person == f.Person) && (f.Group == "" || m.Group == f.Group)
}
