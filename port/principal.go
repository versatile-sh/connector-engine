package port

import "time"

// PrincipalKind is who a request comes from.
type PrincipalKind string

// Principal kinds.
const (
	PrincipalAgent  PrincipalKind = "agent"
	PrincipalView   PrincipalKind = "view"
	PrincipalPerson PrincipalKind = "person"
	PrincipalSystem PrincipalKind = "system"
	PrincipalWorker PrincipalKind = "worker"
)

// Claims are the verified claims of a bearer token; a token with an agent claim is an agent principal.
type Claims struct {
	Issuer     string      `json:"iss"`
	Subject    string      `json:"sub,omitempty"`
	Audience   []string    `json:"aud"`
	IssuedAt   time.Time   `json:"iat"`
	ExpiresAt  time.Time   `json:"exp"`
	Tenant     TenantID    `json:"tenant"`
	Workspace  WorkspaceID `json:"workspace,omitempty"`
	Agent      AgentID     `json:"agent,omitempty"`
	AgentOwner PersonID    `json:"agent_owner,omitempty"`
	Session    SessionID   `json:"session,omitempty"`
	OnBehalfOf PersonID    `json:"on_behalf_of,omitempty"`
	Caller     Caller      `json:"caller,omitempty"`
}

// Principal is the verified caller of one request.
type Principal struct {
	Kind   PrincipalKind `json:"kind"`
	Claims Claims        `json:"claims"`
	Person PersonID      `json:"person,omitempty"`
	Token  string        `json:"-"`
}

// KindOf classifies claims: agent claim first, then caller view, then person.
func KindOf(c Claims) PrincipalKind {
	switch {
	case c.Agent != "" && c.Caller == CallerView:
		return PrincipalView
	case c.Agent != "":
		return PrincipalAgent
	}
	return PrincipalPerson
}
