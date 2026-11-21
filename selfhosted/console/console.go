package console

import (
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	"github.com/versatile-sh/connector-engine/source/skillcatalog"
)

const (
	maxIconBytes  = 512 << 10
	maxIconCache  = 256
	maxAPIBody    = 64 << 10
	oauthStateTTL = 10 * time.Minute
	newKeyFor     = 24 * time.Hour
)

// Console is the console server: the engine client, the owner's state and sessions, and the addresses it answers on.
type Console struct {
	Engine        *Engine
	State         *State
	Sessions      *Sessions
	Guard         Guard
	Web           fs.FS
	PublicURL     string
	MCPURL        string
	OAuthRedirect string
	MaxAgentTTL   time.Duration
	Log           *slog.Logger
	Fetch         skillcatalog.Get
	Activity      *Activity
	Secrets       *sealed.Box
	NewChannel    ChannelFactory
	Restart       func()
	LoopbackHost  string
	ManagersGroup string
	SetupToken    string

	inviteMu      sync.Mutex
	restoring     sync.Mutex
	reconciling   sync.Mutex
	keying        sync.Mutex
	passwordFails lockout
	inviteFails   minuteWindow
	signupHits    lockout
	icons         iconCache
	iconTries     iconTries
	connects      connectStates
	as            authServer
	surf          surfaces
	signing       signingKey
	sep           separatorCache
	writing       sync.Map
}

// Handler returns every route behind the guard, each gated by the capability the route table declares or listed as public.
func (c *Console) Handler() http.Handler {
	rt := &router{c: c, mux: http.NewServeMux()}
	c.routes(rt)
	return c.Guard.Wrap(rt.mux)
}

func (c *Console) routes(rt *router) {
	rt.handle("GET /api/session", c.session)
	rt.handle("GET /api/people", c.listPeople)
	rt.handle("POST /api/invites", c.createInvite)
	rt.handle("DELETE /api/invites/{id}", c.revokeInvite)
	rt.handle("POST /api/people/{person}/reset", c.createReset)
	rt.handle("PUT /api/people/signup", c.setSignup)
	rt.handle("PUT /api/people/{person}/role", c.setRole)
	rt.handle("DELETE /api/people/{person}", c.removePerson)
	rt.handle("POST /api/invite/inspect", c.inspectInvite)
	rt.handle("POST /api/invite/accept", c.acceptInvite)
	rt.handle("GET /api/ready", c.ready)
	rt.handle("POST /api/setup", c.setup)
	rt.handle("POST /api/login", c.login)
	rt.handle("POST /api/signup", c.signup)
	rt.handle("POST /api/logout", c.logout)
	rt.handle("GET /api/account", c.account)
	rt.handle("PUT /api/account/password", c.changePassword)
	rt.handle("PUT /api/account/username", c.rename)
	rt.handle("POST /api/backup/export", c.exportBackup)
	rt.handle("POST /api/backup/inspect", c.inspectUpload)
	rt.handle("POST /api/backup/import", c.importUpload)
	rt.handle("GET /api/connectors", c.listConnectors)
	rt.handle("POST /api/preview", c.preview)
	rt.handle("POST /api/import", c.doImport)
	rt.handle("PUT /api/connectors/{id}/functions/{name}", c.setAccess)
	rt.handle("PUT /api/connectors/{id}/items", c.setItemAccess)
	rt.handle("PUT /api/connectors/{id}/functions", c.setAccessAll)
	rt.handle("POST /api/connectors/{id}/profiles:copy", c.copyProfile)
	rt.handle("PUT /api/connectors/{id}/profiles", c.setProfileMode)
	rt.handle("GET /api/connectors/{id}/profiles/{agent}", c.profileEffects)
	rt.handle("PUT /api/connectors/{id}/profiles/{agent}", c.setProfileAccess)
	rt.handle("PUT /api/connectors/{id}/new-keys", c.setNewKeys)
	rt.handle("PUT /api/connectors/{id}/credential-mode", c.setCredentialMode)
	rt.handle("GET /api/slots", c.listSlots)
	rt.handle("POST /api/slots/{name}/claim", c.claimSlot)
	rt.handle("DELETE /api/slots/{name}", c.discardSlot)
	rt.handle("DELETE /api/connectors/{id}", c.removeConnector)
	rt.handle("GET /api/skills", c.listSkills)
	rt.handle("PUT /api/skills/{id}/keys", c.setSkillAccessAll)
	rt.handle("PUT /api/skills/{id}/keys/{agent}", c.setSkillAccess)
	rt.handle("DELETE /api/skills/{id}", c.removeConnector)
	rt.handle("POST /api/connectors/{id}/headers", c.setHeaders)
	rt.handle("POST /api/connectors/{id}/oauth", c.beginOAuth)
	rt.handle("GET /oauth/callback", c.oauthCallback)
	rt.handle("GET /api/agents", c.listAgents)
	rt.handle("POST /api/agents", c.mintAgent)
	rt.handle("DELETE /api/agents/{name}", c.revokeAgent)
	rt.handle("POST /api/agents/seen", c.seeAgents)
	rt.handle("POST /api/onboarding/dismiss", c.dismissOnboarding)
	rt.handle("GET /api/approvals", c.listApprovals)
	rt.handle("GET /api/approvals/recent", c.recentDecisions)
	rt.handle("POST /api/approvals/{id}", c.decide)
	rt.handle("POST /api/approvals/{id}/cancel", c.cancelApproval)
	rt.handle("GET /api/icon", c.icon)
	rt.handle("GET /api/activity", c.withActivity(c.listActivity))
	rt.handle("DELETE /api/activity", c.withActivity(c.deleteActivity))
	rt.handle("POST /api/activity/prune", c.withActivity(c.pruneActivity))
	rt.handle("GET /api/activity/settings", c.withActivity(c.activitySettings))
	rt.handle("PUT /api/activity/settings", c.withActivity(c.setActivitySettings))
	rt.handle("GET /api/activity/summary", c.withActivity(c.activitySummary))
	rt.handle("GET /api/activity/{id}", c.withActivity(c.activityDetail))
	agent := c.gateMCP(c.refuseRevoked(c.capture(c.elicitProxy(c.agentProxy()))))
	for _, m := range []string{"GET", "POST", "DELETE"} {
		rt.handle(m+" /mcp", agent.ServeHTTP)
	}
	c.appRoutes(rt)
	rt.handle("GET /oauth/authorize", c.authorize)
	rt.handle("GET /api/oauth/requests/{id}", c.consentInfo)
	rt.handle("POST /api/oauth/requests/{id}", c.consent)
	c.surfaceRoutes(rt)
	rt.handle("GET /", c.spa)
}
