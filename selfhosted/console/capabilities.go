package console

import (
	"context"
	"errors"
	"net/http"
	"slices"
)

// Capability is one thing a console role may do; each route declares the one it needs.
type Capability string

const (
	// CapPeopleManage invites people, revokes invites, changes roles and removes people.
	CapPeopleManage Capability = "people.manage"
	// CapPeopleRead sees people, invites and the console audit.
	CapPeopleRead Capability = "people.read"
	// CapPeopleReset creates a reset link for a user of a lower role.
	CapPeopleReset Capability = "people.reset"
	// CapBackupFull exports or restores a full backup.
	CapBackupFull Capability = "backup.full"
	// CapConnectorsManage imports and removes connectors and sets their function access, credentials and new-key default.
	CapConnectorsManage Capability = "connectors.manage"
	// CapSkillsManage installs, removes and opens skills.
	CapSkillsManage Capability = "skills.manage"
	// CapSetupManage exports or imports a setup file.
	CapSetupManage Capability = "setup.manage"
	// CapActivitySettings sets what the activity records.
	CapActivitySettings Capability = "activity.settings"
	// CapActivityDelete deletes all activity.
	CapActivityDelete Capability = "activity.delete"
	// CapKeysAny sees, narrows or revokes anyone's key, applies a slot to any key, discards a slot and allows a function always.
	CapKeysAny Capability = "keys.any"
	// CapApprovalsAny sees everyone's pending calls and decides one on a shared-credential connector.
	CapApprovalsAny Capability = "approvals.any"
	// CapActivityAny sees everyone's activity.
	CapActivityAny Capability = "activity.any"
	// CapKeysOwn makes, renews, narrows and revokes own keys, connects apps and claims a slot for an own key.
	CapKeysOwn Capability = "keys.own"
	// CapApprovalsOwn decides own calls once or for the session.
	CapApprovalsOwn Capability = "approvals.own"
	// CapActivityOwn sees own activity.
	CapActivityOwn Capability = "activity.own"
	// CapNotificationsOwn manages own notification channels, devices and app prompts.
	CapNotificationsOwn Capability = "notifications.own"
	// CapSigninOwn signs in to providers as oneself.
	CapSigninOwn Capability = "signin.own"
	// CapAccountOwn changes own username and password and signs out.
	CapAccountOwn Capability = "account.own"
)

var capabilityRole = map[Capability]Role{
	CapPeopleManage: RoleOwner, CapPeopleRead: RoleAdmin, CapPeopleReset: RoleAdmin, CapBackupFull: RoleOwner,
	CapConnectorsManage: RoleAdmin, CapSkillsManage: RoleAdmin, CapSetupManage: RoleAdmin, CapActivitySettings: RoleAdmin,
	CapActivityDelete: RoleOwner, CapKeysAny: RoleAdmin, CapApprovalsAny: RoleAdmin, CapActivityAny: RoleAdmin,
	CapKeysOwn: RoleMember, CapApprovalsOwn: RoleMember, CapActivityOwn: RoleMember, CapNotificationsOwn: RoleMember,
	CapSigninOwn: RoleMember, CapAccountOwn: RoleMember,
}

// Can reports whether the user's role holds capability c; an unknown capability is held by nobody.
func (u User) Can(c Capability) bool {
	least, ok := capabilityRole[c]
	return ok && u.Role.AtLeast(least)
}

// Capabilities lists every capability the user's role holds, sorted.
func (u User) Capabilities() []Capability {
	out := []Capability{}
	for c := range capabilityRole {
		if u.Can(c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out
}

func may(u User, own, any Capability, owner string) bool {
	return u.Can(any) || owner != "" && owner == u.Person && u.Can(own)
}

var errForbidden = &statusError{status: http.StatusForbidden, code: "forbidden", err: errors.New("your role cannot do this")}

var routeCapabilities = map[string]Capability{
	"POST /api/logout":                           CapAccountOwn,
	"GET /api/people":                            CapPeopleRead,
	"POST /api/invites":                          CapPeopleManage,
	"DELETE /api/invites/{id}":                   CapPeopleReset,
	"POST /api/people/{person}/reset":            CapPeopleReset,
	"PUT /api/people/{person}/role":              CapPeopleManage,
	"DELETE /api/people/{person}":                CapPeopleManage,
	"GET /api/account":                           CapAccountOwn,
	"PUT /api/account/password":                  CapAccountOwn,
	"PUT /api/account/username":                  CapAccountOwn,
	"POST /api/onboarding/dismiss":               CapAccountOwn,
	"POST /api/backup/export":                    CapSetupManage,
	"POST /api/backup/inspect":                   CapSetupManage,
	"POST /api/backup/import":                    CapSetupManage,
	"GET /api/connectors":                        CapKeysOwn,
	"GET /api/icon":                              CapKeysOwn,
	"POST /api/preview":                          CapConnectorsManage,
	"POST /api/import":                           CapConnectorsManage,
	"PUT /api/connectors/{id}/functions/{name}":  CapConnectorsManage,
	"PUT /api/connectors/{id}/items":             CapConnectorsManage,
	"PUT /api/connectors/{id}/functions":         CapConnectorsManage,
	"PUT /api/connectors/{id}/new-keys":          CapConnectorsManage,
	"PUT /api/connectors/{id}/credential-mode":   CapConnectorsManage,
	"DELETE /api/connectors/{id}":                CapConnectorsManage,
	"POST /api/connectors/{id}/headers":          CapSigninOwn,
	"POST /api/connectors/{id}/oauth":            CapSigninOwn,
	"POST /api/connectors/{id}/profiles:copy":    CapKeysOwn,
	"PUT /api/connectors/{id}/profiles":          CapKeysOwn,
	"GET /api/connectors/{id}/profiles/{agent}":  CapKeysOwn,
	"PUT /api/connectors/{id}/profiles/{agent}":  CapKeysOwn,
	"GET /api/slots":                             CapKeysOwn,
	"POST /api/slots/{name}/claim":               CapKeysOwn,
	"DELETE /api/slots/{name}":                   CapKeysAny,
	"GET /api/skills":                            CapKeysOwn,
	"PUT /api/skills/{id}/keys":                  CapKeysOwn,
	"PUT /api/skills/{id}/keys/{agent}":          CapKeysOwn,
	"DELETE /api/skills/{id}":                    CapSkillsManage,
	"GET /api/agents":                            CapKeysOwn,
	"POST /api/agents":                           CapKeysOwn,
	"DELETE /api/agents/{name}":                  CapKeysOwn,
	"POST /api/agents/seen":                      CapKeysOwn,
	"GET /api/oauth/requests/{id}":               CapKeysOwn,
	"POST /api/oauth/requests/{id}":              CapKeysOwn,
	"GET /api/approvals":                         CapApprovalsOwn,
	"GET /api/approvals/recent":                  CapApprovalsOwn,
	"POST /api/approvals/{id}":                   CapApprovalsOwn,
	"POST /api/approvals/{id}/cancel":            CapApprovalsAny,
	"GET /api/activity":                          CapActivityOwn,
	"GET /api/activity/{id}":                     CapActivityOwn,
	"GET /api/activity/settings":                 CapActivityOwn,
	"PUT /api/activity/settings":                 CapActivitySettings,
	"POST /api/activity/prune":                   CapActivityDelete,
	"DELETE /api/activity":                       CapActivityDelete,
	"GET /api/notifications":                     CapNotificationsOwn,
	"POST /api/notifications/channels":           CapNotificationsOwn,
	"DELETE /api/notifications/channels/{id}":    CapNotificationsOwn,
	"POST /api/notifications/channels/{id}/test": CapNotificationsOwn,
	"PUT /api/notifications/apps/{name}":         CapNotificationsOwn,
	"POST /api/notifications/devices":            CapNotificationsOwn,
	"PATCH /api/notifications/devices/{id}":      CapNotificationsOwn,
	"DELETE /api/notifications/devices/{id}":     CapNotificationsOwn,
}

var publicRoutes = []string{
	"GET /api/session", "GET /api/ready", "POST /api/setup", "POST /api/login",
	"GET /oauth/callback", "GET /oauth/authorize",
	"GET /mcp", "POST /mcp", "DELETE /mcp",
	"GET /.well-known/oauth-protected-resource", "OPTIONS /.well-known/oauth-protected-resource",
	"GET /.well-known/oauth-protected-resource/mcp", "OPTIONS /.well-known/oauth-protected-resource/mcp",
	"GET /.well-known/oauth-authorization-server", "OPTIONS /.well-known/oauth-authorization-server",
	"POST /oauth/register", "OPTIONS /oauth/register", "POST /oauth/token", "OPTIONS /oauth/token",
	"GET /api/approvals/{id}", "POST /api/approvals/{id}/decide", "GET /api/notify/stream",
	"POST /api/invite/inspect", "POST /api/invite/accept",
	"GET /",
}

var firstRunRoutes = []string{"POST /api/backup/inspect", "POST /api/backup/import"}

type router struct {
	c        *Console
	mux      *http.ServeMux
	patterns []string
}

func (rt *router) handle(pattern string, h http.HandlerFunc) {
	rt.patterns = append(rt.patterns, pattern)
	rt.mux.HandleFunc(pattern, rt.c.gate(pattern, h))
}

func (c *Console) gate(pattern string, next http.HandlerFunc) http.HandlerFunc {
	if slices.Contains(publicRoutes, pattern) {
		return next
	}
	need, ok := routeCapabilities[pattern]
	if !ok {
		return func(w http.ResponseWriter, _ *http.Request) { c.fail(w, errForbidden) }
	}
	checked := c.authed(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := signedInUser(r.Context()); !u.Can(need) {
			c.fail(w, errForbidden)
			return
		}
		next(w, r)
	})
	if !slices.Contains(firstRunRoutes, pattern) {
		return checked
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if c.State.HasOwner() {
			checked(w, r)
			return
		}
		next(w, r)
	}
}

// OwnerRef names the person a key, approval or activity row belongs to, with a username that is empty once the person is removed.
type OwnerRef struct {
	Person   string `json:"person"`
	Username string `json:"username"`
}

func (c *Console) ownerRef(person string) OwnerRef {
	person = firstOf(person, c.State.LegacyOwner())
	u, _ := c.State.User(person)
	return OwnerRef{Person: person, Username: u.Username}
}

func everyone(r *http.Request, any Capability) (bool, error) {
	switch r.URL.Query().Get("scope") {
	case "", "mine":
		return false, nil
	case "everyone":
		u, _ := signedInUser(r.Context())
		return u.Can(any), nil
	}
	return false, invalid("scope is mine or everyone")
}

func (c *Console) holds(w http.ResponseWriter, r *http.Request, need Capability) bool {
	if u, ok := signedInUser(r.Context()); ok && !u.Can(need) {
		c.fail(w, errForbidden)
		return false
	}
	return true
}

func (c *Console) mayKey(ctx context.Context, name string) bool {
	owner, _ := c.State.KeyOwner(name)
	return c.mayOwnKey(ctx, owner)
}

func (c *Console) mayOwnKey(ctx context.Context, owner string) bool {
	u, ok := signedInUser(ctx)
	return !ok || may(u, CapKeysOwn, CapKeysAny, owner)
}

func (c *Console) keyFor(ctx context.Context, name string) error {
	if !c.mayKey(ctx, name) {
		return noKey(name)
	}
	return nil
}
