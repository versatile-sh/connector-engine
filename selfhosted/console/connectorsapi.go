package console

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func (c *Console) listConnectors(w http.ResponseWriter, r *http.Request) {
	list, err := c.Connectors(r.Context())
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": list})
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

func (c *Console) setSkillAccessAll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Access Access   `json:"access"`
		Keys   []string `json:"keys"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	results, err := c.SetSkillAccessAll(r.Context(), r.PathValue("id"), in.Keys, in.Access)
	c.replyResults(w, "keys", "agent", results, err)
}

func (c *Console) replyResults(w http.ResponseWriter, list, field string, results []Result, err error) {
	if err != nil {
		c.fail(w, err)
		return
	}
	out := make([]map[string]string, len(results))
	for i, res := range results {
		out[i] = map[string]string{field: res.Name}
		if res.Err != nil {
			_, refused := c.refusal(res.Err)
			out[i]["error"], out[i]["code"] = refused.Error, refused.Code
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{list: out})
}

func (c *Console) setAccessAll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names  []string `json:"names"`
		Access Access   `json:"access"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	results, err := c.SetAccessAll(r.Context(), r.PathValue("id"), in.Names, in.Access)
	c.replyResults(w, "items", "name", results, err)
}

func (c *Console) copyProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From string   `json:"from"`
		Keys []string `json:"keys"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	results, err := c.CopyProfile(r.Context(), r.PathValue("id"), in.From, in.Keys)
	c.replyResults(w, "keys", "agent", results, err)
}

func (c *Console) setProfileMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode ProfileMode `json:"mode"`
		Keys []string    `json:"keys"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	results, err := c.SetProfileMode(r.Context(), r.PathValue("id"), in.Keys, in.Mode)
	c.replyResults(w, "keys", "agent", results, err)
}

func (c *Console) setProfileAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names  []string `json:"names"`
		Access Access   `json:"access"`
		Reset  bool     `json:"reset"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	results, err := c.SetProfileAccessAll(r.Context(), r.PathValue("id"), r.PathValue("agent"), in.Names, in.Access, in.Reset)
	c.replyResults(w, "items", "name", results, err)
}

func (c *Console) profileEffects(w http.ResponseWriter, r *http.Request) {
	effects, err := c.ProfileEffects(r.Context(), r.PathValue("id"), r.PathValue("agent"))
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"effects": effects})
}

func (c *Console) setNewKeys(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Off bool `json:"off"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.SetNewKeysOff(r.Context(), r.PathValue("id"), in.Off); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		writeError(w, http.StatusBadRequest, in.Name+" is not a prompt, a resource or the server instructions")
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
			writeError(w, http.StatusBadRequest, "a header name has no spaces or colons and a value has no line breaks")
			return
		}
	}
	if len(in.Headers) == 0 {
		writeError(w, http.StatusBadRequest, "give at least one header")
		return
	}
	if err := c.SetHeaderSecret(r.Context(), r.PathValue("id"), in.Headers); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pendingOAuth struct {
	issuer  string
	person  string
	expires time.Time
}

type connectStates struct {
	mu sync.Mutex
	m  map[string]pendingOAuth
}

func (cs *connectStates) add(state, issuer, person string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	now := time.Now()
	expire(cs.m, now, func(v pendingOAuth) time.Time { return v.expires })
	put(&cs.m, state, pendingOAuth{issuer: issuer, person: person, expires: now.Add(oauthStateTTL)})
}

func (cs *connectStates) take(state string) (pendingOAuth, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	v, ok := cs.m[state]
	delete(cs.m, state)
	return v, ok && time.Now().Before(v.expires)
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
	c.connects.add(state, issuer, actorOf(r.Context()))
	writeJSON(w, http.StatusOK, map[string]string{"url": to})
}

func (c *Console) oauthCallback(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	state := qv.Get("state")
	pend, ok := c.connects.take(state)
	back := func(result string) { http.Redirect(w, r, "/?oauth="+url.QueryEscape(result), http.StatusSeeOther) }
	switch {
	case !ok:
		back("unknown_state")
	case qv.Get("error") != "":
		back("refused")
	case qv.Has("iss") && strings.TrimRight(qv.Get("iss"), "/") != strings.TrimRight(pend.issuer, "/"):
		back("issuer_mismatch")
	default:
		if err := c.CompleteOAuth(WithActor(r.Context(), pend.person), state, qv.Get("code")); err != nil {
			c.logger().Warn("oauth completion failed", "error", err)
			back("failed")
			return
		}
		back("connected")
	}
}
