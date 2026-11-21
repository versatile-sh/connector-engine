package console

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	decisionsRange = 30 * day
	decisionsMax   = 10
)

// DecisionRow is one held call that was answered or ran out of time: what it asked for, the key that asked, how it ended, who answered and when.
type DecisionRow struct {
	ID        string     `json:"id"`
	Agent     string     `json:"agent"`
	Connector string     `json:"connector"`
	Function  string     `json:"function"`
	Class     port.Class `json:"class,omitempty"`
	State     string     `json:"state"`
	DecidedBy string     `json:"decided_by,omitempty"`
	Channel   string     `json:"channel,omitempty"`
	At        time.Time  `json:"at"`
	Owner     OwnerRef   `json:"owner"`
}

func decisionAt(a port.Approval) time.Time {
	switch {
	case !a.DecidedAt.IsZero():
		return a.DecidedAt
	case a.State == port.ApprovalExpired && !a.ExpiresAt.IsZero():
		return a.ExpiresAt
	}
	return a.CreatedAt
}

func (c *Console) recentDecisions(w http.ResponseWriter, r *http.Request) {
	all, err := everyone(r, CapApprovalsAny)
	if err != nil {
		c.fail(w, err)
		return
	}
	since := time.Now().UTC().Add(-decisionsRange).Format(time.RFC3339Nano)
	held, err := list[port.Approval](r.Context(), c.Engine, "/v1/approvals"+q("since", since))
	if err != nil {
		c.fail(w, err)
		return
	}
	u, _ := signedInUser(r.Context())
	rows := []DecisionRow{}
	for _, a := range held {
		if a.State == port.ApprovalPending {
			continue
		}
		if owner := c.approvalOwner(a); all || owner == u.Person {
			rows = append(rows, DecisionRow{ID: string(a.ID), Agent: string(a.Agent), Connector: string(a.Connector), Function: a.Function, Class: a.Class, State: string(a.State), DecidedBy: firstOf(a.DecisionNote, a.DecidedBy), Channel: a.Channel, At: decisionAt(a), Owner: c.ownerRef(owner)})
		}
	}
	slices.SortFunc(rows, func(x, y DecisionRow) int { return cmp.Or(y.At.Compare(x.At), strings.Compare(y.ID, x.ID)) })
	writeJSON(w, http.StatusOK, map[string]any{"decisions": rows[:min(len(rows), decisionsMax)]})
}
