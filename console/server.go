package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/console/sealed"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/skillcatalog"
)

const (
	maxIconBytes  = 512 << 10
	maxIconCache  = 256
	maxAPIBody    = 64 << 10
	oauthStateTTL = 10 * time.Minute
	newKeyFor     = 24 * time.Hour
)

var iconTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/vnd.microsoft.icon"}

type icon struct {
	body []byte
	kind string
}

type pendingOAuth struct {
	issuer  string
	expires time.Time
}

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

	loginMu sync.Mutex
	iconMu  sync.Mutex
	icons   map[string]icon
	oauthMu sync.Mutex
	oauth   map[string]pendingOAuth

	asMu       sync.Mutex
	pending    map[string]authRequest
	codes      map[string]authCode
	registered []time.Time
	refreshMu  sync.Mutex

	consentFails []time.Time
}

// Handler returns every route behind the guard.
func (c *Console) Handler() http.Handler {
	c.icons, c.oauth, c.pending, c.codes = map[string]icon{}, map[string]pendingOAuth{}, map[string]authRequest{}, map[string]authCode{}
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
	mux.HandleFunc("GET /api/activity", c.authed(c.listActivity))
	mux.HandleFunc("DELETE /api/activity", c.authed(c.deleteActivity))
	mux.HandleFunc("GET /api/activity/settings", c.authed(c.activitySettings))
	mux.HandleFunc("PUT /api/activity/settings", c.authed(c.setActivitySettings))
	mux.HandleFunc("GET /api/activity/{id}", c.authed(c.activityDetail))
	agent := c.gateMCP(c.refuseRevoked(c.capture(c.agentProxy())))
	for _, m := range []string{"GET", "POST", "DELETE"} {
		mux.Handle(m+" /mcp", agent)
	}
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		mux.HandleFunc("GET "+p, c.protectedResource)
		mux.HandleFunc("OPTIONS "+p, c.preflight)
	}
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", c.authServerMetadata)
	mux.HandleFunc("OPTIONS /.well-known/oauth-authorization-server", c.preflight)
	mux.HandleFunc("POST /oauth/register", c.register)
	mux.HandleFunc("OPTIONS /oauth/register", c.preflight)
	mux.HandleFunc("POST /oauth/token", c.token)
	mux.HandleFunc("OPTIONS /oauth/token", c.preflight)
	mux.HandleFunc("GET /oauth/authorize", c.authorize)
	mux.HandleFunc("GET /api/oauth/requests/{id}", c.authed(c.consentInfo))
	mux.HandleFunc("POST /api/oauth/requests/{id}", c.authed(c.consent))
	mux.HandleFunc("GET /", c.spa)
	return c.Guard.Wrap(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (c *Console) fail(w http.ResponseWriter, err error) {
	var ee *EngineError
	if errors.As(err, &ee) {
		status := ee.Status
		if status >= 500 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]string{"error": ee.Detail, "code": ee.Code})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "the body is JSON"})
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIBody)).Decode(into); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the body is not valid JSON"})
		return false
	}
	return true
}

func (c *Console) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		csrf, ok := c.Sessions.Get(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
			return
		}
		if !safeMethod(r.Method) && !csrfOK(csrf, r.Header.Get(csrfHeader)) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing or wrong CSRF token"})
			return
		}
		next(w, r)
	}
}

func (c *Console) session(w http.ResponseWriter, r *http.Request) {
	csrf, ok := c.Sessions.Get(r)
	out := map[string]any{"owner_exists": c.State.HasOwner(), "signed_in": ok, "mcp_url": c.MCPURL}
	if ok {
		out["csrf"] = csrf
		out["onboarding_dismissed"] = c.State.OnboardingDismissed()
	}
	writeJSON(w, http.StatusOK, out)
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c *Console) setup(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.State.CreateOwner(strings.TrimSpace(in.Username), in.Password); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrOwnerExists) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"csrf": c.Sessions.Start(w)})
}

func (c *Console) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	c.loginMu.Lock()
	ok, err := c.State.CheckLogin(strings.TrimSpace(in.Username), in.Password)
	if !ok {
		time.Sleep(500 * time.Millisecond)
	}
	c.loginMu.Unlock()
	switch {
	case errors.Is(err, ErrNoOwner):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "create the owner account first"})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot read the account"})
	case !ok:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong username or password"})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"csrf": c.Sessions.Start(w)})
	}
}

func (c *Console) logout(w http.ResponseWriter, r *http.Request) {
	c.Sessions.End(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) listConnectors(w http.ResponseWriter, r *http.Request) {
	list, err := c.Connectors(r.Context())
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": list})
}

type importRequest struct {
	MCP          string            `json:"mcp,omitempty"`
	Listing      string            `json:"listing,omitempty"`
	Local        bool              `json:"local,omitempty"`
	HeaderSecret bool              `json:"header_secret,omitempty"`
	Choices      map[string]Access `json:"choices,omitempty"`
	Access       Access            `json:"access,omitempty"`
	Digest       string            `json:"digest,omitempty"`
}

// Preview is what the confirm screen shows before an import: where it comes from, the listing and each function's proposed choice.
type Preview struct {
	Kind        string        `json:"kind"`
	URL         string        `json:"url"`
	Host        string        `json:"host"`
	Name        string        `json:"name"`
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Icon        string        `json:"icon,omitempty"`
	Publisher   string        `json:"publisher,omitempty"`
	Version     string        `json:"version"`
	RemoteURL   string        `json:"remote_url,omitempty"`
	WebsiteURL  string        `json:"website_url,omitempty"`
	Protocol    string        `json:"protocol,omitempty"`
	Auth        []string      `json:"auth,omitempty"`
	Tools       []ProbedTool  `json:"tools"`
	Items       []Item        `json:"items"`
	Warning     string        `json:"warning,omitempty"`
	Skill       *SkillPreview `json:"skill,omitempty"`
	manifest    port.Manifest
	entry       skillcatalog.Entry
}

func (c *Console) buildPreview(ctx context.Context, in importRequest) (Preview, error) {
	switch {
	case in.MCP != "" && in.Listing == "":
		p, err := Probe(ctx, strings.TrimSpace(in.MCP), Reach{Local: in.Local})
		if err != nil {
			return Preview{}, err
		}
		m, err := ManifestFor(p, in.HeaderSecret)
		if err != nil {
			return Preview{}, err
		}
		u, _ := url.Parse(p.URL)
		return Preview{Kind: "mcp", URL: p.URL, Host: u.Host, Name: m.Name, Title: m.Title, Description: m.Description, Icon: m.Icon, Publisher: m.Publisher.Name,
			Version: m.Version, RemoteURL: p.URL, WebsiteURL: p.WebsiteURL, Protocol: p.Protocol, Tools: nonNil(p.Tools), Items: p.Items, manifest: m}, nil
	case in.Listing != "" && in.MCP == "":
		src := strings.TrimSpace(in.Listing)
		body, err := c.get()(ctx, src, maxListingBytes)
		if err != nil {
			return Preview{}, err
		}
		u, _ := url.Parse(src)
		if skillcatalog.Kind(body) == string(port.KindSkill) {
			e, err := skillcatalog.Parse(body)
			if err != nil {
				return Preview{}, err
			}
			sp := c.skillPreview(e)
			return Preview{Kind: "skill", URL: src, Host: u.Host, Name: e.ID, Title: firstOf(e.Title, e.Name), Description: e.Description, Publisher: skillcatalog.Publisher(e.Source.Repository, u.Host),
				Version: e.Source.Commit, Tools: []ProbedTool{}, Items: []Item{}, Skill: &sp, entry: e}, nil
		}
		m, err := manifestFrom(src, body)
		if err != nil {
			return Preview{}, err
		}
		pv := Preview{Kind: "listing", URL: src, Host: u.Host, Name: m.Name, Title: firstOf(m.Title, m.Name), Description: m.Description, Icon: m.Icon, Publisher: m.Publisher.Name, Version: m.Version, Items: []Item{}}
		for _, a := range m.Auth {
			pv.Auth = append(pv.Auth, string(a.Kind))
		}
		for _, f := range m.Functions {
			ro := f.Annotations != nil && f.Annotations.ReadOnlyHint != nil && *f.Annotations.ReadOnlyHint
			de := !ro && (f.Annotations == nil || f.Annotations.DestructiveHint == nil || *f.Annotations.DestructiveHint)
			prop := AccessAsk
			if ro {
				prop = AccessOn
			}
			pv.Tools = append(pv.Tools, ProbedTool{Name: f.Name, Title: f.Title, Description: f.Description, ReadOnly: ro, Destructive: de, Kind: KindOf(ro, de), Proposed: prop})
		}
		if m.Remote != nil {
			pv.RemoteURL = m.Remote.URL
			p, err := Probe(ctx, m.Remote.URL, Reach{})
			if err != nil {
				pv.Warning = "the server could not be reached to list its functions: " + err.Error()
			} else {
				pv.Tools, pv.Items, pv.Protocol, pv.WebsiteURL = nonNil(p.Tools), p.Items, p.Protocol, p.WebsiteURL
				if pv.Icon == "" {
					pv.Icon = p.Icon
				}
			}
		}
		m.Icon = pv.Icon
		pv.Tools = nonNil(pv.Tools)
		pv.manifest = m
		return pv, nil
	}
	return Preview{}, errors.New("give exactly one of mcp or listing")
}

func (c *Console) get() skillcatalog.Get {
	if c.Fetch != nil {
		return c.Fetch
	}
	return func(ctx context.Context, raw string, limit int64) ([]byte, error) {
		body, _, err := Reach{}.Get(ctx, raw, limit, "application/yaml, application/json, text/plain, */*")
		return body, err
	}
}

func (c *Console) importSkill(w http.ResponseWriter, r *http.Request, in importRequest, p Preview) {
	access := in.Access
	if access == "" {
		access = AccessOn
	}
	if !access.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "access is off, on or ask"})
		return
	}
	if in.Digest != p.Skill.Digest {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the skill changed since the preview; review it again"})
		return
	}
	m, err := skillcatalog.Manifest(r.Context(), c.get(), p.URL, p.entry)
	if err != nil {
		c.fail(w, err)
		return
	}
	id, err := c.InstallSkill(r.Context(), m, access)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"skill": id})
}

func (c *Console) listSkills(w http.ResponseWriter, r *http.Request) {
	list, err := c.Skills(r.Context())
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": nonNil(list)})
}

func (c *Console) setSkillAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Access Access `json:"access"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.SetSkillAccess(r.Context(), r.PathValue("id"), r.PathValue("agent"), in.Access); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (c *Console) preview(w http.ResponseWriter, r *http.Request) {
	var in importRequest
	if !readJSON(w, r, &in) {
		return
	}
	p, err := c.buildPreview(r.Context(), in)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (c *Console) doImport(w http.ResponseWriter, r *http.Request) {
	var in importRequest
	if !readJSON(w, r, &in) {
		return
	}
	for name, a := range in.Choices {
		if !a.Valid() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "function " + name + ": access is off, on or ask"})
			return
		}
	}
	p, err := c.buildPreview(r.Context(), in)
	if err != nil {
		c.fail(w, err)
		return
	}
	if p.Kind == "skill" {
		c.importSkill(w, r, in, p)
		return
	}
	choices := map[string]Access{}
	for _, t := range p.Tools {
		choices[t.Name] = t.Proposed
		if a, ok := in.Choices[t.Name]; ok {
			choices[t.Name] = a
		}
	}
	for _, it := range p.Items {
		choices[it.Name] = it.Access
		if a, ok := in.Choices[it.Name]; ok {
			choices[it.Name] = a
		}
	}
	id, err := c.Install(r.Context(), p.manifest, choices)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"connector": id})
}

func (c *Console) setAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Access Access `json:"access"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.SetAccess(r.Context(), r.PathValue("id"), r.PathValue("name"), in.Access); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) setItemAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   string `json:"name"`
		Access Access `json:"access"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if port.ItemKind(in.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": in.Name + " is not a prompt, a resource or the server instructions"})
		return
	}
	if err := c.SetAccess(r.Context(), r.PathValue("id"), in.Name, in.Access); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) removeConnector(w http.ResponseWriter, r *http.Request) {
	if err := c.Remove(r.Context(), r.PathValue("id")); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) setHeaders(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Headers map[string]string `json:"headers"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	for k, v := range in.Headers {
		if k == "" || strings.ContainsAny(k, " :\r\n") || strings.ContainsAny(v, "\r\n") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a header name has no spaces or colons and a value has no line breaks"})
			return
		}
	}
	if len(in.Headers) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "give at least one header"})
		return
	}
	if err := c.SetHeaderSecret(r.Context(), r.PathValue("id"), in.Headers); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) beginOAuth(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	conn := r.PathValue("id")
	issuer, err := c.OAuthIssuer(r.Context(), conn)
	if err != nil {
		c.fail(w, err)
		return
	}
	if in.ClientID != "" {
		if err := c.SetOAuthClient(r.Context(), issuer, in.ClientID, in.ClientSecret); err != nil {
			c.fail(w, err)
			return
		}
	}
	state, to, err := c.BeginOAuth(r.Context(), conn)
	if err != nil {
		c.fail(w, err)
		return
	}
	c.oauthMu.Lock()
	now := time.Now()
	for k, v := range c.oauth {
		if now.After(v.expires) {
			delete(c.oauth, k)
		}
	}
	c.oauth[state] = pendingOAuth{issuer: issuer, expires: now.Add(oauthStateTTL)}
	c.oauthMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"url": to})
}

func (c *Console) oauthCallback(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	state := qv.Get("state")
	c.oauthMu.Lock()
	pend, ok := c.oauth[state]
	delete(c.oauth, state)
	c.oauthMu.Unlock()
	back := func(result string) { http.Redirect(w, r, "/?oauth="+url.QueryEscape(result), http.StatusSeeOther) }
	switch {
	case !ok || time.Now().After(pend.expires):
		back("unknown_state")
	case qv.Get("error") != "":
		back("refused")
	case qv.Has("iss") && strings.TrimRight(qv.Get("iss"), "/") != strings.TrimRight(pend.issuer, "/"):
		back("issuer_mismatch")
	default:
		if err := c.CompleteOAuth(r.Context(), state, qv.Get("code")); err != nil {
			c.Log.Warn("oauth completion failed", "error", err)
			back("failed")
			return
		}
		back("connected")
	}
}

// AgentView is an access key as the key list shows it: new while an app that connected itself through the browser sign-in is unseen and less than a day old.
type AgentView struct {
	Agent
	New bool `json:"new"`
}

func (c *Console) listAgents(w http.ResponseWriter, _ *http.Request) {
	list := c.State.Agents()
	slices.SortFunc(list, func(a, b Agent) int { return strings.Compare(a.Name, b.Name) })
	views := make([]AgentView, 0, len(list))
	for _, a := range list {
		views = append(views, AgentView{Agent: a, New: a.ViaBrowser && !a.Seen && time.Since(a.ConnectedAt) < newKeyFor})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": views, "mcp_url": c.MCPURL, "max_ttl_hours": int(c.MaxAgentTTL.Hours())})
}

func (c *Console) seeAgents(w http.ResponseWriter, _ *http.Request) {
	if err := c.State.SeeAgents(); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) mintAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		TTLHours int    `json:"ttl_hours"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	tok, err := c.MintAgent(r.Context(), in.Name, time.Duration(in.TTLHours)*time.Hour)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tok)
}

func (c *Console) revokeAgent(w http.ResponseWriter, r *http.Request) {
	if err := c.RevokeAgent(r.Context(), r.PathValue("name")); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) dismissOnboarding(w http.ResponseWriter, _ *http.Request) {
	if err := c.State.DismissOnboarding(); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) listApprovals(w http.ResponseWriter, r *http.Request) {
	list, err := c.Approvals(r.Context())
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": nonNil(list)})
}

func (c *Console) decide(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve bool `json:"approve"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.Decide(r.Context(), r.PathValue("id"), in.Approve); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FetchIcon loads an untrusted icon as image bytes: https or a data URI, at most 512 KiB, and only PNG, JPEG, GIF, WebP or ICO by content sniffing; SVG is refused.
func FetchIcon(ctx context.Context, raw string) ([]byte, string, error) {
	var body []byte
	if rest, ok := strings.CutPrefix(raw, "data:"); ok {
		meta, data, found := strings.Cut(rest, ",")
		if !found || !strings.HasSuffix(meta, ";base64") {
			return nil, "", errors.New("only base64 data URIs are read")
		}
		var err error
		if body, err = base64.StdEncoding.DecodeString(data); err != nil {
			return nil, "", err
		}
		if len(body) > maxIconBytes {
			return nil, "", ErrTooLarge
		}
	} else {
		var err error
		if body, _, err = (Reach{}).Get(ctx, raw, maxIconBytes, "image/png, image/webp, image/jpeg, image/gif, image/x-icon"); err != nil {
			return nil, "", err
		}
	}
	kind := http.DetectContentType(body)
	if !slices.Contains(iconTypes, kind) {
		return nil, "", errors.New("the icon is not a PNG, JPEG, GIF, WebP or ICO image")
	}
	return body, kind, nil
}

func (c *Console) icon(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	c.iconMu.Lock()
	ic, ok := c.icons[raw]
	c.iconMu.Unlock()
	if !ok {
		body, kind, err := FetchIcon(r.Context(), raw)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ic = icon{body: body, kind: kind}
		c.iconMu.Lock()
		if len(c.icons) >= maxIconCache {
			clear(c.icons)
		}
		c.icons[raw] = ic
		c.iconMu.Unlock()
	}
	h := w.Header()
	h.Set("Content-Type", ic.kind)
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Content-Disposition", "inline; filename=icon")
	h.Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(ic.body)
}

type tokenClaims struct {
	Agent    string  `json:"agent"`
	Session  string  `json:"session"`
	IssuedAt float64 `json:"iat"`
}

// bearerClaims reads the agent, session and issue time of a bearer JWT without checking its signature: the engine checks it, and the console only uses them to refuse or to label a call it answered.
func bearerClaims(header string) (tokenClaims, bool) {
	scheme, raw, found := strings.Cut(header, " ")
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if !found || !strings.EqualFold(scheme, "Bearer") || len(parts) != 3 {
		return tokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, false
	}
	var claims tokenClaims
	if json.Unmarshal(payload, &claims) != nil || claims.Agent == "" {
		return tokenClaims{}, false
	}
	return claims, true
}

func (c *Console) refuseRevoked(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, ok := bearerClaims(r.Header.Get("Authorization")); ok && c.State.Revoked(claims.Agent, int64(claims.IssuedAt)) {
			w.Header().Set("WWW-Authenticate", c.challengeValue(c.origin(r), `error="invalid_token", error_description="the access key was revoked"`))
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "the access key was revoked"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (c *Console) gateMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, "/access", http.StatusSeeOther)
			return
		}
		c.challenge(w, r, "")
	})
}

func (c *Console) agentProxy() http.Handler {
	target, _ := url.Parse(c.Engine.PublicURL)
	return &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del(csrfHeader)
			pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			if resp.StatusCode == http.StatusUnauthorized {
				resp.Header.Set("WWW-Authenticate", c.challengeValue(c.base(resp.Request.Header.Get("X-Forwarded-Host")), `error="invalid_token"`))
			}
			if calls, ok := resp.Request.Context().Value(captureKey{}).(*inflight); ok {
				calls.wrap(resp)
			}
			return nil
		},
	}
}

func (c *Console) spa(w http.ResponseWriter, r *http.Request) {
	if c.Web == nil {
		http.Error(w, "the console frontend is not built", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(name, "api/") {
		http.NotFound(w, r)
		return
	}
	if name != "" && !strings.HasSuffix(name, "/") {
		if f, err := c.Web.Open(name); err == nil {
			st, err := f.Stat()
			f.Close()
			if err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, c.Web, name)
				return
			}
		}
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
	}
	index, err := fs.ReadFile(c.Web, "index.html")
	if err != nil {
		http.Error(w, "the console frontend is not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(index)
}
