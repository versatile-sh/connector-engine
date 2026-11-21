// Package service holds the use cases and the gateway-facing Call and ListFunctions; every mutation is one store transaction.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/port"
)

// Deps are the ports a Service runs on; Store, Clock, Authorizer, Secrets and Runtime are required.
// Log receives the operational failures a caller is not told about; nil discards them.
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
	Log        *slog.Logger
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
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
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

type idem struct {
	key, hash string
	id        string
	scope     port.Scope
	replayed  bool
	stored    idemPayload
}

type idemPayload struct {
	Hash   string          `json:"hash"`
	Result json.RawMessage `json:"result,omitempty"`
}

// WithIdempotencyKey marks the mutation run with ctx as a retry-safe one: a second run with the key and the same requestHash runs nothing and Replay returns the first result, one with another hash is a conflict.
func WithIdempotencyKey(ctx context.Context, key, requestHash string) context.Context {
	if key == "" {
		return context.WithValue(ctx, idemKey{}, (*idem)(nil))
	}
	return context.WithValue(ctx, idemKey{}, &idem{key: key, hash: requestHash})
}

// Replay reports whether the mutation run with ctx was a retry; it returns the stored JSON result (empty when none was kept) or a conflict when the key was first used for another request.
func Replay(ctx context.Context) (result []byte, replayed bool, err error) {
	st, _ := ctx.Value(idemKey{}).(*idem)
	if st == nil || !st.replayed {
		return nil, false, nil
	}
	if st.stored.Hash != "" && st.stored.Hash != st.hash {
		return nil, true, port.Errorf(port.CodeConflict, "the idempotency key was used for a different request")
	}
	return st.stored.Result, true, nil
}

// Remember keeps the JSON of result for the retries of the mutation run with ctx; it does nothing without a key or on a retry.
func (s *Service) Remember(ctx context.Context, result any) error {
	st, _ := ctx.Value(idemKey{}).(*idem)
	if st == nil || st.id == "" || st.replayed {
		return nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(idemPayload{Hash: st.hash, Result: raw})
	if err != nil {
		return err
	}
	return s.writeTx(ctx, st.scope, func(tx port.Tx) error { return tx.SetInboxPayload(st.id, payload) })
}

func (s *Service) write(ctx context.Context, scope port.Scope, op string, fn func(port.Tx) error) error {
	st, _ := ctx.Value(idemKey{}).(*idem)
	var seen *touched
	err := s.d.Store.Write(ctx, scope, func(tx port.Tx) error {
		if st != nil {
			id := "idem:" + op + ":" + st.key
			fresh, err := tx.MarkInbox(id, s.now())
			if err != nil {
				return err
			}
			if !fresh {
				raw, err := tx.InboxPayload(id)
				if err != nil {
					return err
				}
				st.replayed, st.stored = true, idemPayload{}
				if len(raw) > 0 {
					_ = json.Unmarshal(raw, &st.stored)
				}
				return nil
			}
			first, err := json.Marshal(idemPayload{Hash: st.hash})
			if err != nil {
				return err
			}
			if err := tx.SetInboxPayload(id, first); err != nil {
				return err
			}
			st.id, st.scope = id, scope
		}
		seen = newTouched(tx, slices.Contains(catalogOps, op))
		return recordLast(seen, fn)
	})
	if err == nil && !(st != nil && st.replayed) && scope.Tenant != "" && slices.Contains(exposureOps, op) {
		s.refreshExposures(ctx, scope.Tenant, seen)
	}
	return err
}

func (s *Service) writeTx(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	return s.d.Store.Write(ctx, scope, func(tx port.Tx) error { return recordLast(tx, fn) })
}

type heldEvents struct {
	port.Tx
	events []port.Event
}

func (t *heldEvents) Emit(e port.Event) (port.Event, error) {
	t.events = append(t.events, e)
	return e, nil
}

func (t *heldEvents) flush() error {
	for _, e := range t.events {
		if _, err := t.Tx.Emit(e); err != nil {
			return err
		}
	}
	t.events = nil
	return nil
}

// recordLast runs fn and emits its events, in order, once fn returned: a write locks its rows, then the tenant stream row.
// emitNow takes the stream row early, where a read must see the tenant's other recorded writes committed.
func recordLast(tx port.Tx, fn func(port.Tx) error) error {
	h := &heldEvents{Tx: tx}
	if err := fn(h); err != nil {
		return err
	}
	return h.flush()
}

func emitNow(tx port.Tx) error {
	if h, ok := tx.(*heldEvents); ok {
		return h.flush()
	}
	return nil
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
