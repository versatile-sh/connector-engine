package console

import (
	"context"
	"net/http"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
)

// CredentialMode is whose credential a connector's calls use: the organisation's one shared credential, or each person's own sign-in.
type CredentialMode string

// The credential modes a connector can have.
const (
	CredentialShared    CredentialMode = "shared"
	CredentialPerPerson CredentialMode = "per_person"
)

// Valid reports whether m is shared or per_person.
func (m CredentialMode) Valid() bool { return m == CredentialShared || m == CredentialPerPerson }

func credentialModes(m port.Manifest) []CredentialMode {
	allowed := func(k port.OwnerKind) bool {
		return !slices.ContainsFunc(m.Auth, func(a port.AuthSpec) bool {
			return len(a.OwnerKindsAllowed) > 0 && !slices.Contains(a.OwnerKindsAllowed, k)
		})
	}
	out := []CredentialMode{}
	if allowed(port.OwnerWorkspace) {
		out = append(out, CredentialShared)
	}
	if allowed(port.OwnerUser) {
		out = append(out, CredentialPerPerson)
	}
	return out
}

type bindRule struct {
	policy    port.CredentialPolicy
	approvers []string
}

func (c *Console) ruleFor(conn string) bindRule {
	if c.State.CredentialMode(conn) == CredentialPerPerson {
		return bindRule{policy: port.CredentialPerUser}
	}
	return bindRule{policy: port.CredentialShared, approvers: []string{string(c.managers())}}
}

func (c *Console) startMode(conn string, m port.Manifest) error {
	if modes := credentialModes(m); len(modes) > 0 && modes[0] != CredentialShared {
		return c.State.SetCredentialMode(conn, modes[0])
	}
	return nil
}

func (c *Console) credentialOwner(ctx context.Context, conn string) (port.OwnerKind, string) {
	if c.State.CredentialMode(conn) == CredentialPerPerson {
		return port.OwnerUser, actorOf(ctx)
	}
	return port.OwnerWorkspace, c.Engine.Workspace
}

// SetCredentialMode sets whose credential a connector's calls use, among the modes its listing allows, then rebinds every key bound to it under its lock with that mode's policy and approvers; calls already held keep their approvers.
func (c *Console) SetCredentialMode(ctx context.Context, conn string, mode CredentialMode) error {
	if !mode.Valid() {
		return invalid("the credential mode is shared or per_person")
	}
	if err := c.plainConnector(ctx, conn); err != nil {
		return err
	}
	v, err := c.Engine.connector(ctx, conn)
	if err != nil {
		return err
	}
	m, err := c.Engine.manifest(ctx, v)
	if err != nil {
		return err
	}
	if !slices.Contains(credentialModes(m), mode) {
		return invalid("this connector's listing does not allow the credential mode %s", mode)
	}
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.State.SetCredentialMode(conn, mode); err != nil {
		return err
	}
	binds, err := c.Engine.bindings(ctx, conn)
	if err != nil {
		return err
	}
	rule := c.ruleFor(conn)
	for _, b := range binds {
		owner, ok := c.State.KeyOwner(string(b.Agent))
		if !ok {
			owner = string(b.AgentOwner)
		}
		if err := c.Engine.bind(ctx, conn, string(b.Agent), owner, rule); err != nil {
			return err
		}
	}
	return nil
}

func (c *Console) atLeast(min Role, next http.HandlerFunc) http.HandlerFunc {
	return c.authed(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := signedInUser(r.Context()); !ok || !u.Role.AtLeast(min) {
			writeError(w, http.StatusForbidden, "your role does not allow this")
			return
		}
		next(w, r)
	})
}

func (c *Console) setCredentialMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode CredentialMode `json:"mode"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.SetCredentialMode(r.Context(), r.PathValue("id"), in.Mode); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
