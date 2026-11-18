// Package wire is the JWT claim set of a per-run or person token and its mapping to port.Claims (§4.3).
package wire

import (
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Claims is the JSON payload of a token.
type Claims struct {
	jwt.Claims
	Tenant     port.TenantID    `json:"tenant,omitempty"`
	Workspace  port.WorkspaceID `json:"workspace,omitempty"`
	Agent      port.AgentID     `json:"agent,omitempty"`
	AgentOwner port.PersonID    `json:"agent_owner,omitempty"`
	Session    port.SessionID   `json:"session,omitempty"`
	OnBehalfOf port.PersonID    `json:"on_behalf_of,omitempty"`
	Caller     port.Caller      `json:"caller,omitempty"`
}

// From maps verified claims to a payload.
func From(c port.Claims) Claims {
	w := Claims{Claims: jwt.Claims{Issuer: c.Issuer, Subject: c.Subject, Audience: c.Audience}, Tenant: c.Tenant, Workspace: c.Workspace,
		Agent: c.Agent, AgentOwner: c.AgentOwner, Session: c.Session, OnBehalfOf: c.OnBehalfOf, Caller: c.Caller}
	if !c.IssuedAt.IsZero() {
		w.IssuedAt = jwt.NewNumericDate(c.IssuedAt)
	}
	if !c.ExpiresAt.IsZero() {
		w.Expiry = jwt.NewNumericDate(c.ExpiresAt)
	}
	return w
}

// Port maps the payload to claims; absent times stay zero.
func (w Claims) Port() port.Claims {
	at := func(d *jwt.NumericDate) time.Time {
		if d == nil {
			return time.Time{}
		}
		return d.Time()
	}
	return port.Claims{Issuer: w.Issuer, Subject: w.Subject, Audience: w.Audience, IssuedAt: at(w.IssuedAt), ExpiresAt: at(w.Expiry), Tenant: w.Tenant,
		Workspace: w.Workspace, Agent: w.Agent, AgentOwner: w.AgentOwner, Session: w.Session, OnBehalfOf: w.OnBehalfOf, Caller: w.Caller}
}
