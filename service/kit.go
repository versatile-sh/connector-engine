package service

import (
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/core/approval"
	corecatalog "github.com/agents-control/connector-engine/core/catalog"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/port"
)

func (s *Service) settings() table.Settings {
	return table.Settings{DestructiveMax: s.cfg.DestructiveMax, ForceAsk: s.cfg.ForceAsk}
}

func (s *Service) facts(tx port.Tx, sub port.Subject, connector port.ConnectorID, groups map[port.PersonID][]port.GroupID) (evaluate.Facts, error) {
	return table.LoadFacts(tx, s.now(), sub, connector, groups, s.settings())
}

func (s *Service) cancelPending(tx port.Tx, f port.ApprovalFilter, reason port.Code, keep func(port.Approval) bool) (int, error) {
	f.States = []port.ApprovalState{port.ApprovalPending}
	open, err := tx.ListApprovals(f)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range open {
		if keep != nil && !keep(a) {
			continue
		}
		c, err := approval.Cancel(a, reason)
		if err != nil {
			return n, err
		}
		if err := tx.UpdateApproval(c, port.ApprovalPending); err != nil {
			return n, err
		}
		if err := s.emit(tx, a.Tenant, string(a.ID), evApprovalCancelled, catalog.ApprovalCancelled{Approval: string(a.ID), Reason: string(reason)}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Service) revokeGrant(tx port.Tx, g port.Grant, by, reason string) error {
	if err := tx.DeleteGrant(g.ID); err != nil {
		return err
	}
	return s.record(tx, g.Tenant, by, string(g.ID), evGrantRevoked, catalog.GrantRevoked{Grant: string(g.ID), By: by, Reason: reason})
}

func (s *Service) dropSessionRows(tx port.Tx, connector port.ConnectorID, function, by string) error {
	rows, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeSession, Connector: connector})
	if err != nil {
		return err
	}
	for _, g := range rows {
		if function == "" || g.Selector == port.SelectorFunction+function {
			if err := s.revokeGrant(tx, g, by, "session_rows_dropped"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) newGrant(g port.Grant) port.Grant {
	g.ID, g.CreatedAt = port.GrantID(s.d.NewID()), s.now()
	return g
}

func (s *Service) putGrant(tx port.Tx, g port.Grant, by string) error {
	if err := tx.PutGrant(g); err != nil {
		return err
	}
	ev := catalog.GrantCreated{Grant: string(g.ID), ScopeKind: string(g.ScopeKind), ScopeID: g.ScopeID, Selector: g.Selector, Mode: string(g.Mode), Origin: string(g.Origin), By: by}
	if !g.ExpiresAt.IsZero() {
		ev.ExpiresAt = &g.ExpiresAt
	}
	return s.record(tx, g.Tenant, by, string(g.ID), evGrantCreated, ev)
}

func (s *Service) view(tx port.Tx, tenant port.TenantID, workspace port.WorkspaceID, groups map[port.PersonID][]port.GroupID) (corecatalog.View, error) {
	return table.View(tx, tenant, []port.WorkspaceID{workspace}, groups)
}
