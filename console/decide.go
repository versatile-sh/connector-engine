package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/surface"
)

// ConnectorRef is how an approval view names its connector.
type ConnectorRef struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
}

// ApprovalView is one approval as the approval surfaces show it.
type ApprovalView struct {
	ID             string           `json:"id"`
	State          string           `json:"state"`
	Mode           string           `json:"mode"`
	Connector      ConnectorRef     `json:"connector"`
	Function       string           `json:"function"`
	FunctionTitle  string           `json:"function_title,omitempty"`
	Kind           port.Class       `json:"kind"`
	App            string           `json:"app"`
	Session        string           `json:"session"`
	CreatedAt      time.Time        `json:"created_at"`
	ExpiresAt      time.Time        `json:"expires_at"`
	Arguments      json.RawMessage  `json:"arguments,omitempty"`
	MayHaveRun     bool             `json:"may_have_run"`
	ReachesOutside bool             `json:"reaches_outside"`
	Choices        []surface.Choice `json:"choices"`
	DecidedBy      string           `json:"decided_by,omitempty"`
	DecidedAt      time.Time        `json:"decided_at,omitzero"`
	Channel        string           `json:"channel,omitempty"`
	approval       port.Approval
}

type approvalDetail struct {
	Approval       port.Approval `json:"approval"`
	Arguments      []byte        `json:"arguments,omitempty"`
	MayHaveRun     bool          `json:"may_have_run"`
	ReachesOutside bool          `json:"reaches_outside"`
}

func (c *Console) connectorRef(ctx context.Context, conn port.ConnectorID) (ConnectorRef, error) {
	var v connectorView
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(string(conn)), nil, &v); err != nil {
		return ConnectorRef{}, err
	}
	ref := ConnectorRef{ID: string(conn), Alias: v.Connector.Alias, Title: v.Connector.Alias}
	l := v.Connector.Listing
	var lv port.ListingVersion
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/listings/version"+q("workspace", c.Engine.Workspace, "owner", string(l.Listing.Owner), "name", l.Listing.Name, "version", l.Version), nil, &lv); err == nil {
		ref.Title, ref.Icon = firstOf(lv.Manifest.Title, lv.Manifest.Name, ref.Alias), lv.Manifest.Icon
	}
	return ref, nil
}

// Approval reads one approval with its connector, its function's nature and the choices still open.
func (c *Console) Approval(ctx context.Context, id string) (ApprovalView, error) {
	var d approvalDetail
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/approvals/"+esc(id), nil, &d); err != nil {
		return ApprovalView{}, err
	}
	a := d.Approval
	v := ApprovalView{ID: string(a.ID), State: string(a.State), Mode: string(a.Mode), Function: a.Function, Kind: a.Class, App: string(a.Agent), Session: string(a.Session),
		CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt, MayHaveRun: d.MayHaveRun, ReachesOutside: d.ReachesOutside, DecidedBy: a.DecisionNote, DecidedAt: a.DecidedAt,
		Channel: a.Channel, Choices: []surface.Choice{}, approval: a}
	if v.DecidedBy == "" {
		v.DecidedBy = a.DecidedBy
	}
	if json.Valid(d.Arguments) {
		v.Arguments = d.Arguments
	}
	ref, err := c.connectorRef(ctx, a.Connector)
	if err != nil {
		return ApprovalView{}, err
	}
	v.Connector = ref
	if f, err := c.function(ctx, string(a.Connector), a.Function); err == nil {
		v.Kind = Nature(f)
		v.FunctionTitle, _ = describe(f.Definition)
	}
	if a.State == port.ApprovalPending {
		v.Choices = append(v.Choices, surface.ChoiceOnce)
		if a.Mode == port.ModeAskOnce {
			v.Choices = append(v.Choices, surface.ChoiceSession)
		}
		v.Choices = append(v.Choices, surface.ChoiceAlways, surface.ChoiceDeny)
	}
	return v, nil
}

// DecideChoice turns an answer into the engine's decision and reports whether another decision won the race.
func (c *Console) DecideChoice(ctx context.Context, id string, choice surface.Choice, channel, note string) (ApprovalView, bool, error) {
	v, err := c.Approval(ctx, id)
	if err != nil {
		return v, false, err
	}
	if v.approval.State != port.ApprovalPending {
		return v, true, nil
	}
	decision := map[surface.Choice]port.Decision{surface.ChoiceOnce: port.DecisionApprove, surface.ChoiceSession: port.DecisionApproveForSession,
		surface.ChoiceAlways: port.DecisionApprove, surface.ChoiceDeny: port.DecisionDeny}[choice]
	switch {
	case decision == "":
		return v, false, fmt.Errorf("the choice is once, session, always or deny")
	case choice == surface.ChoiceSession && v.approval.Mode != port.ModeAskOnce:
		return v, false, fmt.Errorf("this call cannot be allowed for the session; allow it once or always")
	}
	var out port.Approval
	if err := c.Engine.Call(ctx, http.MethodPost, "/v1/approvals/"+esc(id)+":decide", map[string]any{"decision": decision, "channel": channel, "note": note}, &out); err != nil {
		return v, false, err
	}
	lost := out.State == port.ApprovalPending || out.Channel != channel || out.DecisionNote != note
	if choice == surface.ChoiceAlways && !lost {
		c.surf.markAlways(id)
		if err := c.SetAccess(ctx, string(v.approval.Connector), v.approval.Function, AccessOn); err != nil {
			return v, false, err
		}
	}
	if after, err := c.Approval(ctx, id); err == nil {
		v = after
	}
	return v, lost, nil
}

// Outcome is the surface outcome of a decided, expired or cancelled approval view.
func (c *Console) Outcome(v ApprovalView) surface.Outcome {
	o := surface.Outcome{By: v.DecidedBy, Channel: v.Channel, At: v.DecidedAt}
	switch a := v.approval; {
	case a.State == port.ApprovalDenied:
		o.State = surface.StateDenied
	case a.State == port.ApprovalExpired:
		o.State = surface.StateExpired
	case a.State == port.ApprovalCancelled:
		o.State = surface.StateCancelled
	case a.SessionGrant:
		o.State = surface.StateSession
	case c.surf.wasAlways(string(a.ID)):
		o.State = surface.StateAlways
	default:
		o.State = surface.StateApproved
	}
	return o
}
