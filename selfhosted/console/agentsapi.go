package console

import (
	"net/http"
	"slices"
	"strings"
	"time"
)

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
		Slot     string `json:"slot"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	tok, err := c.MintAgent(r.Context(), in.Name, time.Duration(in.TTLHours)*time.Hour, in.Slot)
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
