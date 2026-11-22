package service

import (
	"context"
	"net/url"
	"slices"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/port"
)

// ListCalls returns the audit rows of the caller's tenant, oldest first and the newest limit of them when limit is positive; arguments are references and hashes only.
func (s *Service) ListCalls(ctx context.Context, p port.Principal, f port.CallFilter, limit int) ([]port.Call, error) {
	if err := s.authorize(ctx, p, authz.AuditRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.Call
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.ListCalls(f)
		return err
	})
	return newest(out, limit), err
}

func newest[T any](rows []T, limit int) []T {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[len(rows)-limit:]
}

// SourceStatus is one configured listing source and what its last sync did; a source never synced has a zero time.
type SourceStatus struct {
	Name     string          `json:"name"`
	LastSync port.SourceSync `json:"last_sync"`
}

// ListSources returns the configured listing sources with their last sync, by name.
func (s *Service) ListSources(ctx context.Context, p port.Principal) ([]SourceStatus, error) {
	if err := s.authorize(ctx, p, authz.SourceRead, s.scopeResource(p)); err != nil {
		return nil, err
	}
	var syncs []port.SourceSync
	if err := s.d.Store.Read(ctx, port.Scope{Installation: true}, func(tx port.Tx) (err error) {
		syncs, err = tx.ListSourceSyncs()
		return err
	}); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(s.d.Sources))
	for name := range s.d.Sources {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]SourceStatus, len(names))
	for i, name := range names {
		out[i].Name = name
		if j := slices.IndexFunc(syncs, func(x port.SourceSync) bool { return x.Source == name }); j >= 0 {
			out[i].LastSync = syncs[j]
		}
	}
	return out, nil
}

// RotateKey re-wraps the root key, or the key-encryption key of one tenant, through the secret store; the operator alone may.
func (s *Service) RotateKey(ctx context.Context, p port.Principal, scope port.KeyScope) (port.Operation, error) {
	op := port.Operation{ID: port.OperationID(s.d.NewID()), Kind: "rotate_key", Done: true, CreatedAt: s.now(), UpdatedAt: s.now()}
	if err := s.authorize(ctx, p, authz.KeyRotate, authz.Installation); err != nil {
		return op, err
	}
	if scope.Root == (scope.Tenant != "") {
		return op, port.Errorf(port.CodeInvalidArgument, "rotate the root key or one tenant, not both and not neither")
	}
	if scope.Tenant != "" {
		if err := s.d.Store.Read(ctx, port.Scope{Tenant: scope.Tenant, Installation: true}, func(tx port.Tx) error {
			_, err := tx.GetTenant(scope.Tenant)
			return err
		}); err != nil {
			return op, err
		}
	}
	if err := s.d.Secrets.Rotate(ctx, scope); err != nil {
		return op, err
	}
	return op, s.write(ctx, port.Scope{Installation: true}, "RotateKey", func(tx port.Tx) error {
		subject := "root"
		if scope.Tenant != "" {
			subject = string(scope.Tenant)
		}
		if err := tx.Audit(port.AuditRow{At: s.now(), Actor: actor(p), Action: "key.rotated", Subject: subject}); err != nil {
			return err
		}
		return tx.PutOperation(op)
	})
}

// ClientRegistrationInput is a pre-registered OAuth client of one issuer; the secret is sealed and never read back.
type ClientRegistrationInput struct {
	Issuer      string `json:"issuer"`
	ClientID    string `json:"client_id"`
	Secret      string `json:"secret,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

// PutClientRegistration stores the pre-registered client the tenant uses for an issuer; credential.ClientResolver reads it.
func (s *Service) PutClientRegistration(ctx context.Context, p port.Principal, in ClientRegistrationInput) (port.ClientRegistration, error) {
	tenant := p.Claims.Tenant
	if err := s.authorize(ctx, p, authz.ClientRegister, s.tenantResource(p)); err != nil {
		return port.ClientRegistration{}, err
	}
	if u, err := url.Parse(in.Issuer); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || in.ClientID == "" {
		return port.ClientRegistration{}, port.Errorf(port.CodeInvalidArgument, "a client registration names an https issuer and a client id")
	}
	reg := port.ClientRegistration{Tenant: tenant, Issuer: in.Issuer, ClientID: in.ClientID, RedirectURL: in.RedirectURL, UpdatedAt: s.now()}
	if in.Secret != "" {
		sealed, err := s.d.Secrets.Seal(ctx, tenant, []byte(in.Secret))
		if err != nil {
			return reg, err
		}
		reg.Secret = &sealed
	}
	err := s.write(ctx, port.Scope{Tenant: tenant}, "PutClientRegistration", func(tx port.Tx) error {
		if err := tx.PutClient(reg); err != nil {
			return err
		}
		return tx.Audit(port.AuditRow{At: s.now(), Tenant: tenant, Actor: actor(p), Action: "client.registered", Subject: in.Issuer})
	})
	reg.Secret = nil
	return reg, err
}
