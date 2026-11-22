package credential

import (
	"context"

	"github.com/versatile-sh/connector-engine/port"
)

// Registered is a pre-registered OAuth client; an empty Tenant makes it the default for the issuer.
type Registered struct {
	Tenant port.TenantID
	Client port.OAuthClient
}

type clientKey struct {
	tenant port.TenantID
	issuer string
}

// ClientResolver is the ClientResolver for pre-registered clients only; client metadata documents and dynamic registration are not built.
type ClientResolver struct {
	clients map[clientKey]port.OAuthClient
	store   port.Store
	secrets port.SecretStore
}

// WithStore makes the resolver read the clients an admin registered per tenant before the configured ones.
func (r *ClientResolver) WithStore(store port.Store, secrets port.SecretStore) *ClientResolver {
	r.store, r.secrets = store, secrets
	return r
}

func (r *ClientResolver) stored(ctx context.Context, tenant port.TenantID, issuer string) (port.OAuthClient, bool, error) {
	if r.store == nil || tenant == port.Installation {
		return port.OAuthClient{}, false, nil
	}
	var reg port.ClientRegistration
	var ok bool
	err := r.store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		reg, ok, err = tx.GetClient(issuer)
		return err
	})
	if err != nil || !ok {
		return port.OAuthClient{}, false, err
	}
	c := port.OAuthClient{Issuer: reg.Issuer, ClientID: reg.ClientID, RedirectURL: reg.RedirectURL, Registration: port.RegistrationPreRegistered}
	if reg.Secret != nil {
		if c.ClientSecret, err = r.secrets.Open(ctx, tenant, *reg.Secret); err != nil {
			return port.OAuthClient{}, false, err
		}
	}
	return c, true, nil
}

// NewClientResolver returns a resolver over the given clients, each marked pre_registered.
func NewClientResolver(clients ...Registered) *ClientResolver {
	r := &ClientResolver{clients: map[clientKey]port.OAuthClient{}}
	for _, c := range clients {
		c.Client.Registration = port.RegistrationPreRegistered
		r.clients[clientKey{c.Tenant, c.Client.Issuer}] = c.Client
	}
	return r
}

// Resolve returns the client registered for the tenant, else the configured one of the tenant, else the default, else a not_found error.
func (r *ClientResolver) Resolve(ctx context.Context, tenant port.TenantID, issuer string) (port.OAuthClient, error) {
	if c, ok, err := r.stored(ctx, tenant, issuer); err != nil || ok {
		return c, err
	}
	for _, t := range []port.TenantID{tenant, ""} {
		if c, ok := r.clients[clientKey{t, issuer}]; ok {
			return c, nil
		}
	}
	return port.OAuthClient{}, port.Errorf(port.CodeNotFound, "no pre-registered client for issuer %s", issuer)
}
