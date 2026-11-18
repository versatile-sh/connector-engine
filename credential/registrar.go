package credential

import (
	"context"

	"github.com/agents-control/connector-engine/port"
)

// Registered is a pre-registered OAuth client; an empty Tenant makes it the default for the issuer.
type Registered struct {
	Tenant port.TenantID
	Client port.OAuthClient
}

type registrarKey struct {
	tenant port.TenantID
	issuer string
}

// Registrar is the ClientRegistrar for pre-registered clients only; client metadata documents and dynamic registration are not built.
type Registrar struct {
	clients map[registrarKey]port.OAuthClient
}

// NewRegistrar returns a registrar over the given clients, each marked pre_registered.
func NewRegistrar(clients ...Registered) *Registrar {
	r := &Registrar{clients: map[registrarKey]port.OAuthClient{}}
	for _, c := range clients {
		c.Client.Registration = port.RegistrationPreRegistered
		r.clients[registrarKey{c.Tenant, c.Client.Issuer}] = c.Client
	}
	return r
}

// Resolve returns the tenant's client for the issuer, else the default one, else a not_found error.
func (r *Registrar) Resolve(_ context.Context, tenant port.TenantID, issuer string) (port.OAuthClient, error) {
	for _, t := range []port.TenantID{tenant, ""} {
		if c, ok := r.clients[registrarKey{t, issuer}]; ok {
			return c, nil
		}
	}
	return port.OAuthClient{}, port.Errorf(port.CodeNotFound, "no pre-registered client for issuer %s", issuer)
}
