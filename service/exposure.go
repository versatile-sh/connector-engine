package service

import (
	"context"
	"slices"

	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/core/exposure"
	"github.com/agents-control/connector-engine/port"
)

var exposureOps = []string{"PutBinding", "DeleteBinding", "SetAgentLevel", "SetConnectorLevel", "CreateGrant", "DeleteGrant", "ConfirmGrant", "InstallConnector",
	"RemoveConnector", "SetConnectorEnabled", "AcceptFunction", "AcceptFunctions", "ClassifyFunction", "SetFunctionDisabled", "PutCatalogRule", "DeleteCatalogRule",
	"PutCatalogPolicy", "UpdateConnector"}

var catalogOps = []string{"PutCatalogRule", "DeleteCatalogRule", "PutCatalogPolicy"}

type touched struct {
	port.Tx
	all    bool
	agents map[port.AgentID]bool
	conns  map[port.ConnectorID]bool
}

func newTouched(tx port.Tx, all bool) *touched {
	return &touched{Tx: tx, all: all, agents: map[port.AgentID]bool{}, conns: map[port.ConnectorID]bool{}}
}

func (t *touched) grant(g port.Grant) {
	switch {
	case g.ScopeKind == port.ScopeAgent:
		t.agents[port.AgentID(g.ScopeID)] = true
	case g.Connector != "" && g.Listing == nil:
		t.conns[g.Connector] = true
	default:
		t.all = true
	}
}

func (t *touched) PutBinding(b port.Binding) error {
	t.agents[b.Agent] = true
	return t.Tx.PutBinding(b)
}

func (t *touched) DeleteBinding(c port.ConnectorID, a port.AgentID) error {
	t.agents[a] = true
	return t.Tx.DeleteBinding(c, a)
}

func (t *touched) PutGrant(g port.Grant) error {
	t.grant(g)
	return t.Tx.PutGrant(g)
}

func (t *touched) DeleteGrant(id port.GrantID) error {
	if g, err := t.Tx.GetGrant(id); err == nil {
		t.grant(g)
	} else {
		t.all = true
	}
	return t.Tx.DeleteGrant(id)
}

func (t *touched) PutConnector(c port.Connector, expect int64) (port.Connector, error) {
	t.conns[c.ID] = true
	return t.Tx.PutConnector(c, expect)
}

func (t *touched) PutFunction(f port.Function) error {
	t.conns[f.Connector] = true
	return t.Tx.PutFunction(f)
}

func (t *touched) everyone() bool {
	return t == nil || t.all || len(t.agents) == 0 && len(t.conns) == 0
}

func (t *touched) affects(b port.Binding) bool {
	return t.everyone() || t.agents[b.Agent] || t.conns[b.Connector]
}

func (s *Service) search(ctx context.Context, tenant port.TenantID, qs []port.SearchQuery) ([][]port.FunctionDecision, error) {
	if b, ok := s.d.Authorizer.(port.BatchSearcher); ok {
		return b.SearchBatch(ctx, tenant, qs)
	}
	out := make([][]port.FunctionDecision, len(qs))
	for i, q := range qs {
		var err error
		if out[i], err = s.d.Authorizer.Search(ctx, q.Subject, q.Connector); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) exposureEntries(ctx context.Context, tenant port.TenantID, agent port.AgentID) ([]exposure.Entry, error) {
	var conns []port.Connector
	fns := map[port.ConnectorID]map[string]port.Function{}
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		if conns, err = s.boundConnectors(tx, port.Subject{Tenant: tenant, Agent: agent}); err != nil {
			return err
		}
		for _, c := range conns {
			list, err := tx.ListFunctions(c.ID)
			if err != nil {
				return err
			}
			fns[c.ID] = map[string]port.Function{}
			for _, f := range list {
				fns[c.ID][f.Name] = f
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	qs := make([]port.SearchQuery, len(conns))
	for i, c := range conns {
		qs[i] = port.SearchQuery{Subject: port.Subject{Tenant: tenant, Workspace: c.Workspace, Agent: agent}, Connector: c.ID}
	}
	answers, err := s.search(ctx, tenant, qs)
	if err != nil {
		return nil, err
	}
	var entries []exposure.Entry
	for i, c := range conns {
		for _, d := range answers[i] {
			if d.Decision.Mode == port.ModeDeny {
				continue
			}
			f, ok := fns[c.ID][d.Function]
			if !ok {
				return nil, port.Errorf(port.CodeNotFound, "function %s of %s not found", d.Function, c.ID)
			}
			entries = append(entries, exposure.Entry{Key: exposure.Key(c.ID, d.Function), Function: f})
		}
	}
	return entries, nil
}

// RefreshExposure computes the trifecta check of an agent, stores it and emits event 13 when it is or was exposed and changed.
func (s *Service) RefreshExposure(ctx context.Context, tenant port.TenantID, agent port.AgentID) (port.Exposure, error) {
	entries, err := s.exposureEntries(ctx, tenant, agent)
	if err != nil {
		return port.Exposure{}, err
	}
	next := exposure.Check(entries)
	next.Tenant, next.Agent = tenant, agent
	ctx = WithIdempotencyKey(ctx, "")
	err = s.write(ctx, port.Scope{Tenant: tenant}, "RefreshExposure", func(tx port.Tx) error {
		prev, _, err := tx.GetExposure(agent)
		if err != nil {
			return err
		}
		if prev.Exposed == next.Exposed && slices.Equal(prev.LegA, next.LegA) && slices.Equal(prev.LegB, next.LegB) && slices.Equal(prev.LegC, next.LegC) {
			return nil
		}
		if err := tx.PutExposure(next); err != nil {
			return err
		}
		if !next.Exposed && !prev.Exposed {
			return nil
		}
		return s.emit(tx, tenant, string(agent), evExposureWarning, catalog.ExposureWarning{Agent: string(agent), Exposed: next.Exposed, LegA: next.LegA, LegB: next.LegB, LegC: next.LegC})
	})
	return next, err
}

func (s *Service) refreshExposures(ctx context.Context, tenant port.TenantID, seen *touched) {
	var agents []port.AgentID
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		binds, err := tx.ListBindings(port.BindingFilter{})
		for _, b := range binds {
			if seen.affects(b) && !slices.Contains(agents, b.Agent) {
				agents = append(agents, b.Agent)
			}
		}
		return err
	})
	if err != nil {
		return
	}
	for _, a := range agents {
		_, _ = s.RefreshExposure(ctx, tenant, a)
	}
}

func (s *Service) refreshLater(tenant port.TenantID) {
	s.flights.DoChan("exposure|"+string(tenant), func() (any, error) {
		s.refreshExposures(context.Background(), tenant, nil)
		return nil, nil
	})
}
