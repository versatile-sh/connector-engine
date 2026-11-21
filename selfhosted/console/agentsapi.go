package console

import (
	"net/http"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
	"github.com/versatile-sh/connector-engine/source/skillcompat"
	"slices"
	"strings"
	"time"
)

// AgentView is an access key as the key list shows it, with its owner: new while an app that connected itself through the browser sign-in is unseen and less than a day old.
type AgentView struct {
	Agent
	Owner OwnerRef `json:"owner"`
	New   bool     `json:"new"`
}

func (c *Console) listAgents(w http.ResponseWriter, r *http.Request) {
	all, err := everyone(r, CapKeysAny)
	if err != nil {
		c.fail(w, err)
		return
	}
	u, _ := signedInUser(r.Context())
	list := c.State.Agents()
	slices.SortFunc(list, func(a, b Agent) int { return strings.Compare(a.Name, b.Name) })
	views := make([]AgentView, 0, len(list))
	for _, a := range list {
		if all || a.Owner == u.Person {
			views = append(views, AgentView{Agent: a, Owner: c.ownerRef(a.Owner), New: a.ViaBrowser && !a.Seen && time.Since(a.ConnectedAt) < newKeyFor})
		}
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
		App      string `json:"app"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if _, ok := skillcompat.Find(in.App); in.App != "" && !ok {
		c.fail(w, invalid("unknown app %q", in.App))
		return
	}
	tok, err := c.MintAgent(r.Context(), in.Name, time.Duration(in.TTLHours)*time.Hour, in.Slot)
	if err == nil && in.App != "" {
		err = c.State.SetAgentApp(tok.Agent, in.App)
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tok)
}

func (c *Console) revokeAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := c.keyFor(r.Context(), name)
	if err == nil {
		err = c.RevokeAgent(r.Context(), name)
	}
	if err != nil {
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

// ApprovalRow is a waiting call as the approval list shows it, with the person whose key made it and the choices open to the viewer.
type ApprovalRow struct {
	port.Approval
	Owner   OwnerRef         `json:"owner"`
	Choices []surface.Choice `json:"choices"`
}

func (c *Console) listApprovals(w http.ResponseWriter, r *http.Request) {
	all, err := everyone(r, CapApprovalsAny)
	if err != nil {
		c.fail(w, err)
		return
	}
	list, err := c.Approvals(r.Context())
	if err != nil {
		c.fail(w, err)
		return
	}
	u, _ := signedInUser(r.Context())
	rows := []ApprovalRow{}
	for _, a := range list {
		if owner := c.approvalOwner(a); all || owner == u.Person {
			pr, err := c.present(r.Context(), a)
			_, open := c.choices(r.Context(), a, err == nil && pr.known)
			rows = append(rows, ApprovalRow{Approval: a, Owner: c.ownerRef(owner), Choices: nonNil(open)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": rows})
}

func (c *Console) decide(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve bool `json:"approve"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	err := c.seeApproval(r.Context(), id)
	if err == nil {
		err = c.Decide(r.Context(), id, in.Approve)
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
