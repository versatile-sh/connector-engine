// Package service holds the use cases and the gateway-facing Call and ListFunctions; every mutation is one store transaction.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/port"
)

// Deps are the ports a Service runs on; Store, Clock, Authorizer, Secrets and Runtime are required.
type Deps struct {
	Store      port.Store
	Clock      port.Clock
	Authorizer port.Authorizer
	Secrets    port.SecretStore
	Runtime    port.ConnectorRuntime
	Sources    map[string]port.ListingSource
	Verifier   port.ArtifactVerifier
	Broker     port.OAuthBroker
	Registrar  port.ClientRegistrar
	Filter     port.ResultFilter
	Scanner    port.DefinitionScanner
	Suggester  port.DescriptionSuggester
	NewID      func() string
}

// Service runs the use cases over its ports.
type Service struct {
	d       Deps
	cfg     Config
	mu      sync.Mutex
	handles map[string]*started
	flights singleflight.Group
	connect map[string]pendingConnect
	kinds   sync.Map
}

// New returns a Service; it refuses missing required ports.
func New(d Deps, cfg Config) (*Service, error) {
	if d.Store == nil || d.Clock == nil || d.Authorizer == nil || d.Secrets == nil || d.Runtime == nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "store, clock, authorizer, secrets and runtime are required")
	}
	if d.NewID == nil {
		d.NewID = randomID
	}
	return &Service{d: d, cfg: cfg, handles: map[string]*started{}, connect: map[string]pendingConnect{}}, nil
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) now() time.Time { return s.d.Clock.Now() }

type idemKey struct{}

// WithIdempotencyKey marks the mutation run with ctx as a retry-safe one: a second run with the key does nothing and returns zero values.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, idemKey{}, key)
}

func (s *Service) write(ctx context.Context, scope port.Scope, op string, fn func(port.Tx) error) error {
	key, _ := ctx.Value(idemKey{}).(string)
	var seen *touched
	err := s.d.Store.Write(ctx, scope, func(tx port.Tx) error {
		if key != "" {
			fresh, err := tx.MarkInbox("idem:"+op+":"+key, s.now())
			if err != nil || !fresh {
				return err
			}
		}
		seen = newTouched(tx, slices.Contains(catalogOps, op))
		return fn(seen)
	})
	if err == nil && scope.Tenant != "" && slices.Contains(exposureOps, op) {
		s.refreshExposures(ctx, scope.Tenant, seen)
	}
	return err
}

func (s *Service) emit(tx port.Tx, tenant port.TenantID, subject, typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	_, err = tx.Emit(port.Event{Type: typ, Tenant: tenant, Subject: subject, Time: s.now(), Data: data})
	return err
}

func (s *Service) record(tx port.Tx, tenant port.TenantID, by, subject, typ string, payload any) error {
	if err := tx.Audit(port.AuditRow{At: s.now(), Tenant: tenant, Actor: by, Action: typ, Subject: subject}); err != nil {
		return err
	}
	return s.emit(tx, tenant, subject, typ, payload)
}

func actor(p port.Principal) string {
	if p.Person != "" {
		return string(p.Person)
	}
	return string(p.Kind)
}

func (s *Service) authorize(ctx context.Context, p port.Principal, action, resource string) error {
	if p.Kind == port.PrincipalAgent || p.Kind == port.PrincipalView || p.Claims.Agent != "" {
		return port.Errorf(port.CodeAgentPrincipalRefused, "an agent principal is refused on every management operation")
	}
	if t := p.Claims.Tenant; t != port.Installation {
		if err := s.TenantActive(ctx, t); err != nil {
			return err
		}
	}
	ok, err := s.d.Authorizer.Allowed(ctx, port.Permission{Principal: p, Action: action, Resource: resource})
	if err != nil {
		return err
	}
	if !ok {
		return port.Errorf(port.CodePermissionDenied, "%s may not %s on %s", actor(p), action, resource)
	}
	return nil
}

// TenantActive refuses a tenant that was never created, is removing or is removed; the agent endpoint asks it on every request.
func (s *Service) TenantActive(ctx context.Context, t port.TenantID) error {
	var state port.TenantState
	err := s.d.Store.Read(ctx, port.Scope{Tenant: t}, func(tx port.Tx) error {
		row, err := tx.GetTenant(t)
		state = row.State
		return err
	})
	if port.IsCode(err, port.CodeNotFound) {
		return port.Errorf(port.CodeTenantRemoved, "tenant %s is not registered", t)
	}
	if err == nil && (state == port.TenantRemoving || state == port.TenantRemoved) {
		return port.Errorf(port.CodeTenantRemoved, "tenant %s is %s", t, state)
	}
	return err
}

func (s *Service) scopeResource(p port.Principal) string {
	if p.Claims.Tenant == port.Installation {
		return authz.Installation
	}
	return s.tenantResource(p)
}

func (s *Service) tenantResource(p port.Principal) string {
	return authz.Tenant(string(p.Claims.Tenant))
}

func (s *Service) groups(ctx context.Context, tenant port.TenantID, people ...port.PersonID) (map[port.PersonID][]port.GroupID, error) {
	out := map[port.PersonID][]port.GroupID{}
	for _, p := range people {
		if p == "" {
			continue
		}
		g, err := s.d.Authorizer.Groups(ctx, tenant, p)
		if err != nil {
			return nil, err
		}
		out[p] = g
	}
	return out, nil
}
