package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
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
	SessionUntil   time.Time        `json:"session_until,omitzero"`
	DecidedBy      string           `json:"decided_by,omitempty"`
	DecidedAt      time.Time        `json:"decided_at,omitzero"`
	Channel        string           `json:"channel,omitempty"`
	AlwaysError    string           `json:"always_error,omitempty"`
	Owner          OwnerRef         `json:"owner"`
	approval       port.Approval
	known          bool
	offered        []surface.Choice
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

// Approval reads one approval with its connector, its function's nature, its owner and the choices still open to the context's actor, or to the key's owner without one: deciding when the call names them an approver, always only with keys.any, and cancelling someone else's call with approvals.any.
func (c *Console) Approval(ctx context.Context, id string) (ApprovalView, error) {
	d, err := c.Engine.approval(ctx, id)
	if err != nil {
		return ApprovalView{}, err
	}
	a := d.Approval
	v := ApprovalView{ID: string(a.ID), State: string(a.State), Mode: string(a.Mode), Function: a.Function, Kind: a.Class, App: string(a.Agent), Session: string(a.Session),
		CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt, MayHaveRun: d.MayHaveRun, ReachesOutside: d.ReachesOutside, DecidedBy: a.DecisionNote, DecidedAt: a.DecidedAt,
		Channel: a.Channel, Choices: []surface.Choice{}, AlwaysError: c.surf.alwaysFailed(string(a.ID)), Owner: c.ownerRef(c.approvalOwner(a)), approval: a}
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
	if a.State != port.ApprovalPending {
		return v, nil
	}
	if a.Mode == port.ModeAskOnce {
		v.SessionUntil = d.SessionUntil
	}
	var open []surface.Choice
	v.offered, open = c.choices(ctx, a, v.known)
	v.Choices = append(v.Choices, open...)
	return v, nil
}

func (c *Console) choices(ctx context.Context, a port.Approval, known bool) (offered, open []surface.Choice) {
	if a.State != port.ApprovalPending {
		return nil, nil
	}
	offered = append(offered, surface.ChoiceOnce)
	if a.Mode == port.ModeAskOnce {
		offered = append(offered, surface.ChoiceSession)
	}
	if known && c.mayAlways(ctx, a) {
		offered = append(offered, surface.ChoiceAlways)
	}
	offered = append(offered, surface.ChoiceDeny)
	viewer := actorOf(ctx)
	if viewer == "" || c.mayDecide(a, viewer) {
		open = append(open, offered...)
	}
	if viewer != "" && viewer != string(a.OnBehalfOf) && c.seesEveryApproval(viewer) {
		open = append(open, surface.ChoiceCancel)
	}
	return offered, open
}

// DecideChoice turns an answer into the engine's decision, which refuses an actor the call does not name as an approver, and reports whether another decision won the race.
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
	case choice == surface.ChoiceAlways && !c.mayAlways(ctx, v.approval):
		return v, false, errForbidden
	case !slices.Contains(v.offered, choice):
		return v, false, invalid("%s is not offered for this call", choice)
	}
	out, err := c.Engine.decide(ctx, id, map[string]any{"decision": decision, "channel": channel, "note": note})
	if err != nil {
		return v, false, err
	}
	lost := out.State == port.ApprovalPending || out.Channel != channel || out.DecisionNote != note
	if choice == surface.ChoiceAlways && !lost {
		c.surf.markAlways(id)
		go c.allowAfterRun(context.WithoutCancel(ctx), v)
	}
	if after, err := c.Approval(ctx, id); err == nil {
		v = after
	}
	return v, lost, nil
}

func (c *Console) allowAfterRun(ctx context.Context, v ApprovalView) {
	ctx, cancel := context.WithTimeout(ctx, allowWait)
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
		err = c.allowForKey(ctx, string(v.approval.Connector), string(v.approval.Agent), v.approval.Function)
	}
	if err != nil {
		c.surf.failAlways(v.ID, err)
		c.logger().Warn("always allow", "approval", v.ID, "error", err)
	}
}

func (c *Console) allowForKey(ctx context.Context, conn, agent, name string) error {
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := c.function(ctx, conn, name)
	if err != nil {
		return err
	}
	workspace, err := c.workspaceGrants(ctx, conn)
	if err != nil {
		return err
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, "", conn)
	if err != nil {
		return err
	}
	held := func(key string) bool {
		return slices.ContainsFunc(rows, func(g port.Grant) bool { return g.ScopeID == key && g.Selector == port.SelectorOf(name) })
	}
	base := functionAccess(f, workspace)
	if base != AccessOn {
		for _, a := range c.allAgents() {
			if a.Name != agent && !held(a.Name) {
				if err := c.writeOverride(ctx, conn, a.Name, f, base); err != nil {
					return err
				}
			}
		}
		if err := c.State.LimitNewKeys(conn, name, base); err != nil {
			return err
		}
	}
	if err := c.Engine.deleteGrants(ctx, slices.DeleteFunc(rows, func(g port.Grant) bool { return g.ScopeID != agent || g.Selector != port.SelectorOf(name) })); err != nil {
		return err
	}
	if base == AccessOn {
		return nil
	}
	return c.writeAccess(ctx, conn, name, AccessOn)
}

var errNotApprover = errors.New("this call is not yours to answer: it waits for its key's owner")

func notApprover(err error) bool {
	var ee *EngineError
	return errors.As(err, &ee) && ee.Code == string(port.CodeNotAnApprover)
}

func (c *Console) approvalOwner(a port.Approval) string {
	if owner, ok := c.State.KeyOwner(string(a.Agent)); ok {
		return owner
	}
	return firstOf(string(a.OnBehalfOf), c.State.LegacyOwner())
}

func (c *Console) mayAlways(ctx context.Context, a port.Approval) bool {
	u, ok := c.State.User(firstOf(actorOf(ctx), c.approvalOwner(a)))
	return ok && u.Can(CapKeysAny)
}

func (c *Console) mayApproval(u User, a port.Approval) bool {
	return may(u, CapApprovalsOwn, CapApprovalsAny, c.approvalOwner(a))
}

func (c *Console) seeApproval(ctx context.Context, id string) error {
	u, ok := signedInUser(ctx)
	if !ok {
		return nil
	}
	a, err := c.record(ctx, id)
	if err != nil {
		return err
	}
	if !c.mayApproval(u, a) {
		return noApproval(id)
	}
	return nil
}

func noApproval(id string) error {
	return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "approval " + id + " not found"}
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
