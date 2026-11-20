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
	MCPURL        string
	OAuthRedirect string
	MaxAgentTTL   time.Duration
	Log           *slog.Logger
	Fetch         skillcatalog.Get
	Activity      *Activity
	Secrets       *sealed.Box
	NewChannel    ChannelFactory

	loginMu       sync.Mutex
	passwordFails minuteWindow
	icons         iconCache
	connects      connectStates
	as            authServer
	surf          surfaces
	signing       signingKey
	sep           separatorCache
}

// Handler returns every route behind the guard.
func (c *Console) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", c.session)
	mux.HandleFunc("POST /api/setup", c.setup)
	mux.HandleFunc("POST /api/login", c.login)
	mux.HandleFunc("POST /api/logout", c.authed(c.logout))
	mux.HandleFunc("GET /api/connectors", c.authed(c.listConnectors))
	mux.HandleFunc("POST /api/preview", c.authed(c.preview))
	mux.HandleFunc("POST /api/import", c.authed(c.doImport))
	mux.HandleFunc("PUT /api/connectors/{id}/functions/{name}", c.authed(c.setAccess))
	mux.HandleFunc("PUT /api/connectors/{id}/items", c.authed(c.setItemAccess))
	mux.HandleFunc("DELETE /api/connectors/{id}", c.authed(c.removeConnector))
	mux.HandleFunc("GET /api/skills", c.authed(c.listSkills))
	mux.HandleFunc("PUT /api/skills/{id}/keys", c.authed(c.setSkillAccessAll))
	mux.HandleFunc("PUT /api/skills/{id}/keys/{agent}", c.authed(c.setSkillAccess))
	mux.HandleFunc("DELETE /api/skills/{id}", c.authed(c.removeConnector))
	mux.HandleFunc("POST /api/connectors/{id}/headers", c.authed(c.setHeaders))
	mux.HandleFunc("POST /api/connectors/{id}/oauth", c.authed(c.beginOAuth))
	mux.HandleFunc("GET /oauth/callback", c.oauthCallback)
	mux.HandleFunc("GET /api/agents", c.authed(c.listAgents))
	mux.HandleFunc("POST /api/agents", c.authed(c.mintAgent))
	mux.HandleFunc("DELETE /api/agents/{name}", c.authed(c.revokeAgent))
	mux.HandleFunc("POST /api/agents/seen", c.authed(c.seeAgents))
	mux.HandleFunc("POST /api/onboarding/dismiss", c.authed(c.dismissOnboarding))
	mux.HandleFunc("GET /api/approvals", c.authed(c.listApprovals))
	mux.HandleFunc("POST /api/approvals/{id}", c.authed(c.decide))
	mux.HandleFunc("GET /api/icon", c.authed(c.icon))
	mux.HandleFunc("GET /api/activity", c.authed(c.withActivity(c.listActivity)))
	mux.HandleFunc("DELETE /api/activity", c.authed(c.withActivity(c.deleteActivity)))
	mux.HandleFunc("GET /api/activity/settings", c.authed(c.withActivity(c.activitySettings)))
	mux.HandleFunc("PUT /api/activity/settings", c.authed(c.withActivity(c.setActivitySettings)))
	mux.HandleFunc("GET /api/activity/{id}", c.authed(c.withActivity(c.activityDetail)))
	agent := c.gateMCP(c.refuseRevoked(c.capture(c.elicitProxy(c.agentProxy()))))
	for _, m := range []string{"GET", "POST", "DELETE"} {
		mux.Handle(m+" /mcp", agent)
	}
	c.appRoutes(mux)
	mux.HandleFunc("GET /oauth/authorize", c.authorize)
	mux.HandleFunc("GET /api/oauth/requests/{id}", c.authed(c.consentInfo))
	mux.HandleFunc("POST /api/oauth/requests/{id}", c.authed(c.consent))
	c.surfaceRoutes(mux)
	mux.HandleFunc("GET /", c.spa)
	return c.Guard.Wrap(mux)
}
