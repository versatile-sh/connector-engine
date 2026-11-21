package console

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	linkFailsPerMin = 10
	auditPageSize   = 50
)

var errRemoveSelf = &statusError{status: http.StatusConflict, code: "remove_self", err: errors.New("ask another owner to remove your account")}

// PersonRow is a console user as the People list shows them, with whether the viewer may make them a reset link.
type PersonRow struct {
	Person    string    `json:"person"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	Keys      int       `json:"keys"`
	You       bool      `json:"you"`
	MayReset  bool      `json:"may_reset"`
}

// LinkRow is a link still waiting as the People list shows it, with whether the viewer may revoke it; its token is never part of it.
type LinkRow struct {
	ID        string    `json:"id"`
	Purpose   Purpose   `json:"purpose"`
	Role      Role      `json:"role,omitempty"`
	Person    string    `json:"person,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	MayRevoke bool      `json:"may_revoke"`
}

func (c *Console) publicURL() string {
	return strings.TrimRight(firstOf(c.PublicURL, strings.TrimSuffix(c.MCPURL, "/mcp")), "/")
}

func (c *Console) listPeople(w http.ResponseWriter, r *http.Request) {
	viewer, _ := signedInUser(r.Context())
	keys := map[string]int{}
	for _, a := range c.State.Agents() {
		keys[a.Owner]++
	}
	users := c.State.Users()
	slices.SortFunc(users, func(a, b User) int {
		return cmp.Or(roleRank[b.Role]-roleRank[a.Role], a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Username, b.Username))
	})
	people := make([]PersonRow, 0, len(users))
	for _, u := range users {
		people = append(people, PersonRow{Person: u.Person, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt, Keys: keys[u.Person], You: u.Person == viewer.Person, MayReset: mayResetFor(viewer, u)})
	}
	links := []LinkRow{}
	for _, inv := range c.State.Links() {
		links = append(links, LinkRow{ID: inv.ID, Purpose: inv.Purpose, Role: inv.Role, Person: inv.Person, CreatedBy: inv.CreatedBy, CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt, MayRevoke: mayRevokeFor(viewer, inv)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "invites": links, "audit": c.State.Audit(auditPageSize), "signup": c.State.Signup()})
}

func (c *Console) linkJSON(w http.ResponseWriter, l NewLink, err error) {
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": l.ID, "link": c.publicURL() + "/invite#" + url.PathEscape(l.Token), "expires_at": l.ExpiresAt})
}

func (c *Console) createInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	role, err := ParseRole(in.Role)
	if err != nil {
		c.fail(w, err)
		return
	}
	u, _ := signedInUser(r.Context())
	l, err := c.State.CreateJoinLink(u.Person, role)
	c.linkJSON(w, l, err)
}

func (c *Console) createReset(w http.ResponseWriter, r *http.Request) {
	u, _ := signedInUser(r.Context())
	l, err := c.State.CreateResetLink(u.Person, r.PathValue("person"))
	c.linkJSON(w, l, err)
}

func (c *Console) revokeInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := signedInUser(r.Context())
	if err := c.State.RevokeLink(u.Person, r.PathValue("id")); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) tryLink(w http.ResponseWriter, use func() error) bool {
	c.inviteMu.Lock()
	full := c.inviteFails.full(time.Now(), linkFailsPerMin)
	c.inviteMu.Unlock()
	if full {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, apiError{Error: "too many wrong links, try again in a minute", Code: "slow_down"})
		return false
	}
	err := use()
	if errors.Is(err, ErrLinkGone) {
		c.inviteMu.Lock()
		c.inviteFails.hit(time.Now())
		c.inviteMu.Unlock()
	}
	if err != nil {
		c.fail(w, err)
		return false
	}
	return true
}

func (c *Console) inspectInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var facts LinkFacts
	if c.tryLink(w, func() (err error) { facts, err = c.State.InspectLink(in.Token); return }) {
		writeJSON(w, http.StatusOK, facts)
	}
}

func (c *Console) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var u User
	if !c.tryLink(w, func() (err error) {
		u, err = c.State.AcceptLink(in.Token, strings.TrimSpace(in.Username), in.Password)
		return
	}) {
		return
	}
	if err := c.ReconcileRoles(r.Context()); err != nil {
		go c.untilReady(context.WithoutCancel(r.Context()), "roles", func(ctx context.Context) error { return c.ReconcileRoles(ctx) })
	}
	c.signIn(w, http.StatusCreated, u.Person)
}

func (c *Console) setRole(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	role, err := ParseRole(in.Role)
	if err == nil {
		err = c.SetUserRole(r.Context(), r.PathValue("person"), role)
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) removePerson(w http.ResponseWriter, r *http.Request) {
	if err := c.RemovePerson(r.Context(), r.PathValue("person")); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemovePerson removes a user as the acting Owner, refusing the last Owner and the Owner's own account: the engine forgets the person, their keys stop working, then the console drops the user; every step repeats safely.
func (c *Console) RemovePerson(ctx context.Context, person string) error {
	c.reconciling.Lock()
	defer c.reconciling.Unlock()
	actor := actorOf(ctx)
	target, ok := c.State.User(person)
	switch {
	case !ok:
		return notFound("no user is %s", person)
	case target.Role == RoleOwner && !slices.ContainsFunc(c.State.Users(), func(u User) bool { return u.Role == RoleOwner && u.Person != person }):
		return ErrLastOwner
	case person == actor:
		return errRemoveSelf
	}
	var keys []string
	for _, a := range c.State.Agents() {
		if a.Owner == person {
			keys = append(keys, a.Name)
		}
	}
	if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/people/"+url.PathEscape(person), nil, nil); err != nil && !IsCode(err, "not_found") {
		return err
	}
	for _, name := range keys {
		if err := c.RevokeAgent(ctx, name); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	return c.State.RemoveUser(actor, person, keys)
}
