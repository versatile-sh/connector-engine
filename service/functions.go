package service

import (
	"context"
	"slices"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/core/grant"
	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/core/projection"
	"github.com/agents-control/connector-engine/port"
)

// AcceptResult is an accepted function and the fn: rows its new hash left stale (§9 "Clearing").
type AcceptResult struct {
	Function   port.Function
	StaleGrant []port.Grant
}

// ListFunctionsOf returns the stored functions of a connector (§13.1 listFunctions).
func (s *Service) ListFunctionsOf(ctx context.Context, p port.Principal, id port.ConnectorID) ([]port.Function, error) {
	v, err := s.GetConnector(ctx, p, id)
	return v.Functions, err
}

func (s *Service) functionTx(ctx context.Context, p port.Principal, id port.ConnectorID, action, op string, fn func(tx port.Tx, c port.Connector) error) error {
	c, err := s.connectorOf(ctx, p, id, action)
	if err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: c.Tenant}, op, func(tx port.Tx) error {
		cur, err := tx.GetConnector(id)
		if err != nil {
			return err
		}
		return fn(tx, cur)
	})
}

func (s *Service) acceptOne(tx port.Tx, c port.Connector, name string, seen port.Hash, by string) (AcceptResult, error) {
	var out AcceptResult
	f, err := tx.GetFunction(c.ID, name)
	if err != nil {
		return out, err
	}
	lv, err := tx.GetVersion(c.Listing)
	if err != nil {
		return out, err
	}
	if f, err = pinning.Accept(f, seen, by, s.now()); err != nil {
		return out, err
	}
	class, source := classOf(f, lv.Manifest)
	f, eff := pinning.Reclassify(f, class, source)
	if eff.DropSessionRows {
		if err := s.dropSessionRows(tx, c.ID, name, by); err != nil {
			return out, err
		}
	}
	if err := tx.PutFunction(f); err != nil {
		return out, err
	}
	rows, err := tx.ListGrants(port.GrantFilter{Connector: c.ID})
	if err != nil {
		return out, err
	}
	for _, g := range rows {
		if grant.Covers(g.Selector, f) && g.Selector == port.SelectorFunction+name && grant.Stale(g, f) {
			out.StaleGrant = append(out.StaleGrant, g)
		}
	}
	out.Function = f
	return out, s.record(tx, c.Tenant, by, string(c.ID), evFunctionAccepted, catalog.FunctionAccepted{Connector: string(c.ID), Function: name,
		Hash: string(f.DefinitionHash), Class: string(f.Class), ClassSource: string(f.ClassSource), By: by})
}

// AcceptFunction clears a quarantine; the admin names the hash they saw and the class is set again from §5.2 (§9).
func (s *Service) AcceptFunction(ctx context.Context, p port.Principal, id port.ConnectorID, name string, seen port.Hash) (AcceptResult, error) {
	var out AcceptResult
	err := s.functionTx(ctx, p, id, authz.FunctionManage, "AcceptFunction", func(tx port.Tx, c port.Connector) (err error) {
		out, err = s.acceptOne(tx, c, name, seen, actor(p))
		return err
	})
	return out, err
}

// AcceptFunctions accepts several functions in one act; every hash must match or nothing is accepted.
func (s *Service) AcceptFunctions(ctx context.Context, p port.Principal, id port.ConnectorID, seen map[string]port.Hash) ([]AcceptResult, error) {
	var out []AcceptResult
	err := s.functionTx(ctx, p, id, authz.FunctionManage, "AcceptFunctions", func(tx port.Tx, c port.Connector) error {
		for _, name := range slices.Sorted(mapKeys(seen)) {
			r, err := s.acceptOne(tx, c, name, seen[name], actor(p))
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// AcceptInstructions takes the pending instructions hash of a connector as its pinned one (relayInstructions = pinned).
func (s *Service) AcceptInstructions(ctx context.Context, p port.Principal, id port.ConnectorID) (port.Connector, error) {
	var out port.Connector
	err := s.functionTx(ctx, p, id, authz.FunctionManage, "AcceptInstructions", func(tx port.Tx, c port.Connector) error {
		pending, ok := c.Config[pendingInstructions]
		if !ok {
			return port.Errorf(port.CodeInvalidTransition, "connector %s has no instructions to accept", id)
		}
		c.InstructionsHash = port.Hash(pending)
		delete(c.Config, pendingInstructions)
		var err error
		out, err = tx.PutConnector(c, c.Revision)
		return err
	})
	return out, err
}

// ClassifyFunction sets the class by an admin at the current hash; a raise drops session rows and cancels pending approvals (§6.4 G7).
func (s *Service) ClassifyFunction(ctx context.Context, p port.Principal, id port.ConnectorID, name string, class port.Class) (port.Function, error) {
	var out port.Function
	err := s.functionTx(ctx, p, id, authz.FunctionManage, "ClassifyFunction", func(tx port.Tx, c port.Connector) error {
		f, err := tx.GetFunction(id, name)
		if err != nil {
			return err
		}
		f, eff := pinning.Reclassify(f, class, port.ClassSourceAdmin)
		if err := s.applyEffects(tx, c, name, eff, actor(p)); err != nil {
			return err
		}
		out = f
		return tx.PutFunction(f)
	})
	return out, err
}

func (s *Service) applyEffects(tx port.Tx, c port.Connector, name string, eff pinning.Effects, by string) error {
	if eff.DropSessionRows {
		if err := s.dropSessionRows(tx, c.ID, name, by); err != nil {
			return err
		}
	}
	if eff.Cancel != "" {
		_, err := s.cancelPending(tx, port.ApprovalFilter{Connector: c.ID}, eff.Cancel, func(a port.Approval) bool { return a.Function == name })
		return err
	}
	return nil
}

// SetFunctionDisabled disables a function (an admin's act) or returns it to the state it had.
func (s *Service) SetFunctionDisabled(ctx context.Context, p port.Principal, id port.ConnectorID, name string, disabled bool) (port.Function, error) {
	var out port.Function
	err := s.functionTx(ctx, p, id, authz.FunctionManage, "SetFunctionDisabled", func(tx port.Tx, c port.Connector) error {
		f, err := tx.GetFunction(id, name)
		if err != nil {
			return err
		}
		switch {
		case disabled && f.State != port.FunctionDisabled:
			f.PreviousState, f.State = f.State, port.FunctionDisabled
			if err := s.applyEffects(tx, c, name, pinning.Effects{DropSessionRows: true, Cancel: port.ReasonDisabled}, actor(p)); err != nil {
				return err
			}
		case !disabled && f.State == port.FunctionDisabled:
			f.State, f.PreviousState = f.PreviousState, ""
			if f.State == "" {
				f.State = port.FunctionQuarantined
			}
		default:
			return port.Errorf(port.CodeInvalidTransition, "function %s is %s", name, f.State)
		}
		out = f
		return tx.PutFunction(f)
	})
	return out, err
}

// SetProjection turns response projection on or off for a function; a function without an outputSchema is refused (§10.5).
func (s *Service) SetProjection(ctx context.Context, p port.Principal, id port.ConnectorID, name string, on bool) (port.Function, error) {
	var out port.Function
	err := s.functionTx(ctx, p, id, authz.FunctionManage, "SetProjection", func(tx port.Tx, c port.Connector) error {
		f, err := tx.GetFunction(id, name)
		if err != nil {
			return err
		}
		if on {
			if err := projection.Validate(outputSchemaOf(f), nil, s.cfg.ProjectionMaxPaths); err != nil {
				return err
			}
		}
		f.Projection = on
		out = f
		return tx.PutFunction(f)
	})
	return out, err
}
