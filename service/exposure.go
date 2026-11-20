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

func (s *Service) exposureEntries(ctx context.Context, tenant port.TenantID, agent port.AgentID) ([]exposure.Entry, error) {
	var conns []port.Connector
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		conns, err = s.boundConnectors(tx, port.Subject{Tenant: tenant, Agent: agent})
		return err
	}); err != nil {
		return nil, err
	}
	var entries []exposure.Entry
	for _, c := range conns {
		decisions, err := s.d.Authorizer.Search(ctx, port.Subject{Tenant: tenant, Workspace: c.Workspace, Agent: agent}, c.ID)
		if err != nil {
			return nil, err
		}
		err = s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
			for _, d := range decisions {
				if d.Decision.Mode == port.ModeDeny {
					continue
				}
				f, err := tx.GetFunction(c.ID, d.Function)
				if err != nil {
					return err
				}
				entries = append(entries, exposure.Entry{Key: exposure.Key(c.ID, d.Function), Function: f})
			}
			return nil
		})
		if err != nil {
			return nil, err
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

func (s *Service) refreshExposures(ctx context.Context, tenant port.TenantID) {
	var agents []port.AgentID
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		binds, err := tx.ListBindings(port.BindingFilter{})
		for _, b := range binds {
			if !slices.Contains(agents, b.Agent) {
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
		s.refreshExposures(context.Background(), tenant)
		return nil, nil
	})
}
