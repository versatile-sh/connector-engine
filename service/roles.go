package service

import (
	"context"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/port"
)

func (s *Service) roleGate(ctx context.Context, p port.Principal, tenant port.TenantID) (port.Scope, error) {
	if tenant == port.Installation {
		return port.Scope{Installation: true}, s.authorize(ctx, p, authz.RoleInstallation, authz.Installation)
	}
	if tenant != p.Claims.Tenant {
		return port.Scope{}, port.Errorf(port.CodeNotFound, "tenant %s not found", tenant)
	}
	return port.Scope{Tenant: tenant}, s.authorize(ctx, p, authz.RoleManage, authz.Tenant(string(tenant)))
}

func validRole(r port.RoleAssignment) error {
	if err := port.CheckIDs("person", string(r.Person)); err != nil {
		return err
	}
	switch {
	case r.Person == "" || r.Scope == "":
		return port.Errorf(port.CodeInvalidArgument, "a role names a person and a scope")
	case !table.Known(r.Role):
		return port.Errorf(port.CodeInvalidArgument, "unknown role %q", r.Role)
	case (table.Role(r.Role) == table.Operator) != (r.Tenant == port.Installation):
		return port.Errorf(port.CodeInvalidArgument, "the operator role is the installation's alone, every other role belongs to a tenant")
	case r.Tenant != port.Installation && !authz.Covers(authz.Tenant(string(r.Tenant)), r.Scope):
		return port.Errorf(port.CodeInvalidArgument, "scope %q is outside tenant %s", r.Scope, r.Tenant)
	case r.Tenant == port.Installation && r.Scope != authz.Installation:
		return port.Errorf(port.CodeInvalidArgument, "an installation role covers the installation")
	case r.Tenant == port.Installation && r.Issuer == "":
		return port.Errorf(port.CodeInvalidArgument, "an installation role names the issuer of its person")
	}
	return nil
}

// PutRole gives a person a role over a scope of a tenant, or the operator role over the installation; a second call writes nothing new.
func (s *Service) PutRole(ctx context.Context, p port.Principal, r port.RoleAssignment) (port.RoleAssignment, error) {
	scope, err := s.roleGate(ctx, p, r.Tenant)
	if err != nil {
		return r, err
	}
	if err := validRole(r); err != nil {
		return r, err
	}
	return r, s.write(ctx, scope, "PutRole", func(tx port.Tx) error {
		if err := tx.PutRole(r); err != nil {
			return err
		}
		return tx.Audit(port.AuditRow{At: s.now(), Tenant: r.Tenant, Actor: actor(p), Action: "role.put", Subject: string(r.Person), Detail: map[string]any{"role": r.Role, "scope": r.Scope}})
	})
}

// DeleteRole takes a role away from a person.
func (s *Service) DeleteRole(ctx context.Context, p port.Principal, r port.RoleAssignment) error {
	scope, err := s.roleGate(ctx, p, r.Tenant)
	if err != nil {
		return err
	}
	return s.write(ctx, scope, "DeleteRole", func(tx port.Tx) error {
		if err := tx.DeleteRole(r); err != nil {
			return err
		}
		return tx.Audit(port.AuditRow{At: s.now(), Tenant: r.Tenant, Actor: actor(p), Action: "role.delete", Subject: string(r.Person), Detail: map[string]any{"role": r.Role, "scope": r.Scope}})
	})
}

// ListRoles returns the roles of a tenant, or the installation's for the empty tenant.
func (s *Service) ListRoles(ctx context.Context, p port.Principal, tenant port.TenantID, f port.RoleFilter) ([]port.RoleAssignment, error) {
	scope, err := s.roleGate(ctx, p, tenant)
	if err != nil {
		return nil, err
	}
	var out []port.RoleAssignment
	err = s.d.Store.Read(ctx, scope, func(tx port.Tx) (err error) {
		out, err = tx.ListRoles(f)
		return err
	})
	return out, err
}

// PutGroupMember makes a person a member of a group of the caller's tenant.
func (s *Service) PutGroupMember(ctx context.Context, p port.Principal, m port.GroupMember) (port.GroupMember, error) {
	m.Tenant = p.Claims.Tenant
	scope, err := s.roleGate(ctx, p, m.Tenant)
	if err != nil || m.Tenant == port.Installation {
		return m, orRefused(err)
	}
	if m.Person == "" || m.Group == "" {
		return m, port.Errorf(port.CodeInvalidArgument, "a membership names a person and a group")
	}
	if err := port.CheckIDs("person", string(m.Person), "group", string(m.Group)); err != nil {
		return m, err
	}
	return m, s.write(ctx, scope, "PutGroupMember", func(tx port.Tx) error { return tx.PutMember(m) })
}

// DeleteGroupMember removes a person from a group of the caller's tenant.
func (s *Service) DeleteGroupMember(ctx context.Context, p port.Principal, m port.GroupMember) error {
	m.Tenant = p.Claims.Tenant
	scope, err := s.roleGate(ctx, p, m.Tenant)
	if err != nil || m.Tenant == port.Installation {
		return orRefused(err)
	}
	return s.write(ctx, scope, "DeleteGroupMember", func(tx port.Tx) error { return tx.DeleteMember(m) })
}

// ListGroupMembers returns the memberships of the caller's tenant.
func (s *Service) ListGroupMembers(ctx context.Context, p port.Principal, f port.MemberFilter) ([]port.GroupMember, error) {
	scope, err := s.roleGate(ctx, p, p.Claims.Tenant)
	if err != nil || p.Claims.Tenant == port.Installation {
		return nil, orRefused(err)
	}
	var out []port.GroupMember
	err = s.d.Store.Read(ctx, scope, func(tx port.Tx) (err error) {
		out, err = tx.ListMembers(f)
		return err
	})
	return out, err
}

func orRefused(err error) error {
	if err != nil {
		return err
	}
	return port.Errorf(port.CodeInvalidArgument, "groups belong to a tenant")
}
