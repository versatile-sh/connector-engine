package service

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/port"
)

type pendingConnect struct {
	Tenant    port.TenantID
	Person    port.PersonID
	Connector port.ConnectorID
	Owner     port.OwnerKind
	OwnerID   string
	Request   port.AuthorizationRequest
	Expires   time.Time
}

type tokenSecret struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token,omitempty"`
}

func (s *Service) credentialAuth(ctx context.Context, p port.Principal, kind port.OwnerKind, ownerID string, c port.Connector) error {
	tenant := string(p.Claims.Tenant)
	if kind == port.OwnerUser {
		return s.authorize(ctx, p, authz.CredentialOwn, authz.Person(tenant, ownerID))
	}
	return s.authorize(ctx, p, authz.CredentialWorkspace, wsResource(p, c.Workspace))
}

// CredentialInput is a credential to store: the secret is sealed before it reaches the store.
type CredentialInput struct {
	Connector port.ConnectorID    `json:"connector"`
	OwnerKind port.OwnerKind      `json:"owner_kind"`
	OwnerID   string              `json:"owner_id"`
	Kind      port.CredentialKind `json:"kind"`
	Issuer    string              `json:"issuer"`
	Resource  string              `json:"resource"`
	Secret    []byte              `json:"secret,omitempty"`
	Scopes    []string            `json:"scopes,omitempty"`
	ExpiresAt time.Time           `json:"expires_at"`
}

func (s *Service) storeCredential(ctx context.Context, by string, c port.Connector, in CredentialInput, reg port.Registration) (port.Credential, error) {
	sealed, err := s.d.Secrets.Seal(ctx, c.Tenant, in.Secret)
	if err != nil {
		return port.Credential{}, err
	}
	cred := port.Credential{ID: port.CredentialID(s.d.NewID()), Tenant: c.Tenant, OwnerKind: in.OwnerKind, OwnerID: in.OwnerID, Connector: c.ID, Kind: in.Kind, Issuer: in.Issuer,
		Resource: in.Resource, ClientRegistration: reg, ScopesGranted: in.Scopes, Secret: sealed, ExpiresAt: in.ExpiresAt, Status: port.CredentialActive, CreatedBy: by, CreatedAt: s.now()}
	if !in.ExpiresAt.IsZero() {
		cred.RefreshAfter = in.ExpiresAt.Add(-s.cfg.RefreshMargin)
	}
	err = s.write(ctx, port.Scope{Tenant: c.Tenant}, "StoreCredential", func(tx port.Tx) error {
		old, err := tx.ListCredentials(port.CredentialFilter{OwnerKind: in.OwnerKind, OwnerID: in.OwnerID, Connector: c.ID})
		if err != nil {
			return err
		}
		if i := slices.IndexFunc(old, func(o port.Credential) bool { return o.Kind == in.Kind && o.Issuer == in.Issuer }); i >= 0 {
			cred.ID, cred.Revision, cred.CreatedAt = old[i].ID, old[i].Revision, old[i].CreatedAt
		}
		if cred, err = tx.PutCredential(cred, port.AnyRevision); err != nil {
			return err
		}
		return s.record(tx, c.Tenant, by, string(cred.ID), evCredentialConnected, catalog.CredentialConnected{Credential: string(cred.ID), OwnerKind: string(cred.OwnerKind), Connector: string(c.ID), ScopesGranted: nonNil(in.Scopes)})
	})
	cred.Secret = port.Sealed{}
	return cred, err
}

// CreateCredential stores a sealed credential for an owner of a connector; the answer carries metadata only (§11.1).
func (s *Service) CreateCredential(ctx context.Context, p port.Principal, in CredentialInput) (port.Credential, error) {
	c, err := s.connectorOf(ctx, p, in.Connector, authz.ConnectorRead)
	if err != nil {
		return port.Credential{}, err
	}
	if err := s.credentialAuth(ctx, p, in.OwnerKind, in.OwnerID, c); err != nil {
		return port.Credential{}, err
	}
	return s.storeCredential(ctx, actor(p), c, in, port.RegistrationManual)
}

// ConnectStart is the address of the provider's authorization and the single-use state that binds it to the person.
type ConnectStart struct {
	State string `json:"state"`
	URL   string `json:"url"`
}

// BeginConnect starts an authorization for the signed-in person; CompleteConnect refuses any other person (§11.3).
func (s *Service) BeginConnect(ctx context.Context, p port.Principal, connector port.ConnectorID, owner port.OwnerKind) (ConnectStart, error) {
	var out ConnectStart
	if s.d.Broker == nil || s.d.Registrar == nil {
		return out, port.Errorf(port.CodeUnavailable, "no OAuth broker is configured")
	}
	c, err := s.connectorOf(ctx, p, connector, authz.ConnectorRead)
	if err != nil {
		return out, err
	}
	ownerID := string(p.Person)
	if owner != port.OwnerUser {
		ownerID = string(c.Workspace)
	}
	if err := s.credentialAuth(ctx, p, owner, ownerID, c); err != nil {
		return out, err
	}
	var m port.Manifest
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: c.Tenant}, func(tx port.Tx) error {
		lv, err := tx.GetVersion(c.Listing)
		m = lv.Manifest
		return err
	}); err != nil {
		return out, err
	}
	i := slices.IndexFunc(m.Auth, func(a port.AuthSpec) bool { return a.Kind == port.CredentialOAuth2User })
	if i < 0 {
		return out, port.Errorf(port.CodeInvalidArgument, "listing %s has no OAuth authorization", c.Listing.Listing)
	}
	spec := m.Auth[i]
	client, err := s.d.Registrar.Resolve(ctx, c.Tenant, spec.Issuer)
	if err != nil {
		return out, err
	}
	var scopes []string
	for _, sc := range spec.ScopesByFunction {
		scopes = append(scopes, sc...)
	}
	slices.Sort(scopes)
	req := port.AuthorizationRequest{Client: client, Resource: spec.Resource, Scopes: slices.Compact(scopes), State: s.d.NewID(), Verifier: s.d.NewID()}
	url, err := s.d.Broker.Begin(ctx, req)
	if err != nil {
		return out, err
	}
	now := s.now()
	s.mu.Lock()
	maps.DeleteFunc(s.connect, func(_ string, v pendingConnect) bool { return now.After(v.Expires) })
	s.connect[req.State] = pendingConnect{Tenant: c.Tenant, Person: p.Person, Connector: connector, Owner: owner, OwnerID: ownerID, Request: req, Expires: now.Add(10 * time.Minute)}
	s.mu.Unlock()
	return ConnectStart{State: req.State, URL: url}, nil
}

// CompleteConnect exchanges the code and seals the tokens for the person the connect was begun for.
func (s *Service) CompleteConnect(ctx context.Context, p port.Principal, state, code string) (port.Credential, error) {
	s.mu.Lock()
	pend, ok := s.connect[state]
	delete(s.connect, state)
	s.mu.Unlock()
	if !ok || s.now().After(pend.Expires) || pend.Tenant != p.Claims.Tenant {
		return port.Credential{}, port.Errorf(port.CodeNotFound, "no authorization is waiting for this state")
	}
	if pend.Owner == port.OwnerUser && pend.Person != p.Person {
		return port.Credential{}, port.Errorf(port.CodeNotTheBoundPerson, "this authorization was begun for another person")
	}
	tokens, err := s.d.Broker.Complete(ctx, pend.Request, code)
	if err != nil {
		return port.Credential{}, err
	}
	secret, _ := json.Marshal(tokenSecret{Access: string(tokens.AccessToken), Refresh: string(tokens.RefreshToken)})
	c, err := s.connectorOf(ctx, p, pend.Connector, authz.ConnectorRead)
	if err != nil {
		return port.Credential{}, err
	}
	return s.storeCredential(ctx, actor(p), c, CredentialInput{Connector: pend.Connector, OwnerKind: pend.Owner, OwnerID: pend.OwnerID, Kind: port.CredentialOAuth2User,
		Issuer: pend.Request.Client.Issuer, Resource: pend.Request.Resource, Secret: secret, Scopes: tokens.Scopes, ExpiresAt: tokens.ExpiresAt}, pend.Request.Client.Registration)
}

// RefreshCredential refreshes one credential under its revision; invalid_grant (unauthenticated) moves it to needs_reauth (§11.3).
func (s *Service) RefreshCredential(ctx context.Context, tenant port.TenantID, id port.CredentialID) error {
	var cred port.Credential
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		cred, err = tx.GetCredential(id)
		return err
	}); err != nil {
		return err
	}
	if cred.Status != port.CredentialActive || s.d.Broker == nil || cred.Kind != port.CredentialOAuth2User {
		return nil
	}
	plain, err := s.d.Secrets.Open(ctx, tenant, cred.Secret)
	if err != nil {
		return err
	}
	var tok tokenSecret
	if err := json.Unmarshal(plain, &tok); err != nil {
		return port.Errorf(port.CodeInternal, "credential %s holds no token set", id)
	}
	client, err := s.d.Registrar.Resolve(ctx, tenant, cred.Issuer)
	if err != nil {
		return err
	}
	fresh, err := s.d.Broker.Refresh(ctx, client, []byte(tok.Refresh))
	if port.IsCode(err, port.CodeUnauthenticated) {
		return s.write(ctx, port.Scope{Tenant: tenant}, "CredentialNeedsReauth", func(tx port.Tx) error {
			cred.Status = port.CredentialNeedsReauth
			if _, err := tx.PutCredential(cred, cred.Revision); err != nil {
				return err
			}
			return s.record(tx, tenant, "worker", string(id), evCredentialReauth, catalog.CredentialNeedsReauth{Credential: string(id), OwnerKind: string(cred.OwnerKind), OwnerID: cred.OwnerID})
		})
	}
	if err != nil {
		return err
	}
	secret, _ := json.Marshal(tokenSecret{Access: string(fresh.AccessToken), Refresh: string(fresh.RefreshToken)})
	if len(fresh.RefreshToken) == 0 {
		secret, _ = json.Marshal(tokenSecret{Access: string(fresh.AccessToken), Refresh: tok.Refresh})
	}
	sealed, err := s.d.Secrets.Seal(ctx, tenant, secret)
	if err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: tenant}, "RefreshCredential", func(tx port.Tx) error {
		cred.Secret, cred.ExpiresAt, cred.LastRefreshedAt = sealed, fresh.ExpiresAt, s.now()
		cred.RefreshAfter = fresh.ExpiresAt.Add(-s.cfg.RefreshMargin)
		_, err := tx.PutCredential(cred, cred.Revision)
		return err
	})
}

func (s *Service) revokeCredentialRow(tx port.Tx, c port.Credential, by string) error {
	c.Status, c.Secret = port.CredentialRevoked, port.Sealed{}
	if _, err := tx.PutCredential(c, port.AnyRevision); err != nil {
		return err
	}
	return s.record(tx, c.Tenant, by, string(c.ID), evCredentialRevoked, catalog.CredentialRevoked{Credential: string(c.ID), By: by})
}

func (s *Service) revokeUpstream(ctx context.Context, creds []port.Credential) {
	if s.d.Broker == nil || s.d.Registrar == nil {
		return
	}
	for _, c := range creds {
		if c.Kind != port.CredentialOAuth2User || c.Status != port.CredentialActive {
			continue
		}
		plain, err := s.d.Secrets.Open(ctx, c.Tenant, c.Secret)
		var tok tokenSecret
		if err != nil || json.Unmarshal(plain, &tok) != nil {
			continue
		}
		if client, err := s.d.Registrar.Resolve(ctx, c.Tenant, c.Issuer); err == nil {
			_ = s.d.Broker.Revoke(ctx, client, []byte(tok.Access))
		}
	}
}

// RevokeCredential revokes a credential at its issuer where possible and clears its sealed secret (§11.3 Disconnect).
func (s *Service) RevokeCredential(ctx context.Context, p port.Principal, id port.CredentialID) error {
	var cred port.Credential
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		cred, err = tx.GetCredential(id)
		return err
	}); err != nil {
		return err
	}
	c, err := s.connectorOf(ctx, p, cred.Connector, authz.ConnectorRead)
	if err != nil {
		return err
	}
	if err := s.credentialAuth(ctx, p, cred.OwnerKind, cred.OwnerID, c); err != nil {
		return err
	}
	s.revokeUpstream(ctx, []port.Credential{cred})
	return s.write(ctx, port.Scope{Tenant: cred.Tenant}, "RevokeCredential", func(tx port.Tx) error {
		return s.revokeCredentialRow(tx, cred, actor(p))
	})
}

// ListCredentials returns credential metadata: all for a workspace admin, the caller's own otherwise.
func (s *Service) ListCredentials(ctx context.Context, p port.Principal, f port.CredentialFilter) ([]port.Credential, error) {
	if err := s.authorize(ctx, p, authz.ConnectorRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	admin := s.authorize(ctx, p, authz.CredentialWorkspace, s.tenantResource(p)) == nil
	var out []port.Credential
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		all, err := tx.ListCredentials(f)
		for _, c := range all {
			if admin || (c.OwnerKind == port.OwnerUser && c.OwnerID == string(p.Person)) {
				c.Secret = port.Sealed{}
				out = append(out, c)
			}
		}
		return err
	})
	return out, err
}

type resolution struct {
	Credential *port.Credential
	Code       port.Code
	Scopes     []string
	Connect    bool
}

func (s *Service) resolveCredential(tx port.Tx, c port.Connector, b port.Binding, onBehalfOf port.PersonID, function string, m port.Manifest) (resolution, error) {
	if len(m.Auth) == 0 {
		return resolution{}, nil
	}
	kind, id := port.OwnerUser, string(onBehalfOf)
	switch b.CredentialPolicy {
	case port.CredentialPerAgent:
		kind, id = port.OwnerAgent, string(b.Agent)
	case port.CredentialShared:
		kind, id = port.OwnerWorkspace, string(c.Workspace)
	}
	need := resolution{Code: port.CodeCredentialRequired, Connect: kind == port.OwnerUser && onBehalfOf != ""}
	if id == "" {
		return need, nil
	}
	creds, err := tx.ListCredentials(port.CredentialFilter{OwnerKind: kind, OwnerID: id, Connector: c.ID})
	if err != nil {
		return need, err
	}
	i := slices.IndexFunc(creds, func(cr port.Credential) bool { return cr.Status == port.CredentialActive })
	if i < 0 {
		return need, nil
	}
	for _, a := range m.Auth {
		for _, sc := range a.ScopesByFunction[function] {
			if !slices.Contains(creds[i].ScopesGranted, sc) {
				need.Scopes = append(need.Scopes, sc)
			}
		}
	}
	if len(need.Scopes) > 0 {
		return need, nil
	}
	return resolution{Credential: &creds[i]}, nil
}
