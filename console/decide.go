package console

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/surface"
)

const (
	allowWait = 3 * time.Minute
	allowPoll = 250 * time.Millisecond
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
	Skill          bool             `json:"skill"`
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
	known          bool
}

func (c *Console) connectorRef(ctx context.Context, conn port.ConnectorID) (ConnectorRef, port.Manifest, error) {
	v, err := c.Engine.connector(ctx, string(conn))
	if err != nil {
		return ConnectorRef{}, port.Manifest{}, err
	}
	ref := ConnectorRef{ID: string(conn), Alias: v.Alias, Title: v.Alias}
	m, err := c.Engine.manifest(ctx, v)
	if err == nil {
		ref.Title, ref.Icon = firstOf(m.Title, m.Name, ref.Alias), m.Icon
	}
	return ref, m, nil
}

func (c *Console) record(ctx context.Context, id string) (port.Approval, error) {
	d, err := c.Engine.approval(ctx, id)
	return d.Approval, err
}

type presented struct {
	connector ConnectorRef
	skill     bool
	kind      port.Class
	title     string
	known     bool
	at        time.Time
}

func (c *Console) present(ctx context.Context, a port.Approval) (presented, error) {
	key := string(a.Connector) + "/" + a.Function + "/" + string(a.DefinitionHash)
	if pr, ok := c.surf.presented(key); ok {
		return pr, nil
	}
	ref, m, err := c.connectorRef(ctx, a.Connector)
	if err != nil {
		return presented{}, err
	}
	pr := presented{connector: ref, at: time.Now()}
	if m.Kind == port.KindSkill {
		pr.skill, pr.kind, pr.title, pr.known = true, port.ClassRead, firstOf(m.Title, ref.Alias), true
	} else if f, err := c.function(ctx, string(a.Connector), a.Function); err == nil {
		pr.kind, pr.known = Nature(f), true
		pr.title, _ = describe(f.Definition)
	}
	if pr.known {
		c.surf.present(key, pr)
	}
	return pr, nil
}

// Approval reads one approval with its connector, its function's nature and the choices still open.
func (c *Console) Approval(ctx context.Context, id string) (ApprovalView, error) {
	d, err := c.Engine.approval(ctx, id)
	if err != nil {
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
	pr, err := c.present(ctx, a)
	if err != nil {
		return ApprovalView{}, err
	}
	v.Connector, v.Skill, v.known = pr.connector, pr.skill, pr.known
	if pr.kind != "" {
		v.Kind = pr.kind
	}
	if pr.skill {
		v.Function = pr.connector.Alias
		v.Connector.Title = "Skill"
	}
	v.FunctionTitle = pr.title
	if a.State == port.ApprovalPending {
		v.Choices = append(v.Choices, surface.ChoiceOnce)
		if a.Mode == port.ModeAskOnce {
			v.Choices = append(v.Choices, surface.ChoiceSession)
		}
		if v.known {
			v.Choices = append(v.Choices, surface.ChoiceAlways)
		}
		v.Choices = append(v.Choices, surface.ChoiceDeny)
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
		return v, false, invalid("the choice is once, session, always or deny")
	case !slices.Contains(v.Choices, choice):
		return v, false, invalid("%s is not offered for this call", choice)
	}
	out, err := c.Engine.decide(ctx, id, map[string]any{"decision": decision, "channel": channel, "note": note})
	if err != nil {
		return v, false, err
	}
	lost := out.State == port.ApprovalPending || out.Channel != channel || out.DecisionNote != note
	if choice == surface.ChoiceAlways && !lost {
		c.surf.markAlways(id)
		go c.allowAfterRun(v)
	}
	if after, err := c.Approval(ctx, id); err == nil {
		v = after
	}
	return v, lost, nil
}

// allowAfterRun turns the approval's function or skill on once its approved call has run, since replacing the row earlier would cancel that run.
func (c *Console) allowAfterRun(v ApprovalView) {
	ctx, cancel := context.WithTimeout(context.Background(), allowWait)
	defer cancel()
	for failures := 0; ctx.Err() == nil && failures < 20; {
		a, err := c.record(ctx, v.ID)
		if err != nil {
			failures++
		} else if a.State != port.ApprovalApproved && a.State != port.ApprovalExecuting {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(allowPoll):
		}
	}
	var err error
	if v.Skill {
		err = c.SetSkillAccess(ctx, string(v.approval.Connector), string(v.approval.Agent), AccessOn)
	} else {
		err = c.SetAccess(ctx, string(v.approval.Connector), v.approval.Function, AccessOn)
	}
	if err != nil {
		c.logger().Warn("always allow", "approval", v.ID, "error", err)
	}
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
