package credential

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sync/singleflight"

	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
)

const maxClientName = 64

// Registered is a pre-registered OAuth client; an empty Tenant makes it the default for the issuer.
type Registered struct {
	Tenant port.TenantID
	Client port.OAuthClient
}

type clientKey struct {
	tenant port.TenantID
	issuer string
}

// ClientResolver is the ClientResolver: stored and configured clients first, then, once WithDiscovery is set, a client metadata document (CIMD) and dynamic registration (RFC 7591).
type ClientResolver struct {
	clients   map[clientKey]port.OAuthClient
	store     port.Store
	secrets   port.SecretStore
	discovery *Discoverer
	clock     port.Clock
	flights   singleflight.Group
}

// WithStore makes the resolver read the clients an admin registered per tenant before the configured ones, and keep the ones it registers itself.
func (r *ClientResolver) WithStore(store port.Store, secrets port.SecretStore) *ClientResolver {
	r.store, r.secrets = store, secrets
	return r
}

// WithClock sets the clock that dates the clients it registers and judges their expiry; without one nothing expires.
func (r *ClientResolver) WithClock(c port.Clock) *ClientResolver {
	r.clock = c
	return r
}

func (r *ClientResolver) now() time.Time {
	if r.clock == nil {
		return time.Time{}
	}
	return r.clock.Now()
}

// WithDiscovery lets Acquire register clients on its own through the issuers that allow it.
func (r *ClientResolver) WithDiscovery(d *Discoverer) *ClientResolver {
	r.discovery = d
	return r
}

func (r *ClientResolver) storedRow(ctx context.Context, tenant port.TenantID, issuer string) (port.ClientRegistration, bool, error) {
	if r.store == nil || tenant == port.Installation {
		return port.ClientRegistration{}, false, nil
	}
	var reg port.ClientRegistration
	var ok bool
	err := r.store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		reg, ok, err = tx.GetClient(issuer)
		return err
	})
	return reg, ok && err == nil, err
}

func (r *ClientResolver) open(ctx context.Context, tenant port.TenantID, reg port.ClientRegistration) (port.OAuthClient, error) {
	c := port.OAuthClient{Issuer: reg.Issuer, ClientID: reg.ClientID, RedirectURL: reg.RedirectURL, Registration: registrationOrDefault(reg.Registration)}
	if reg.Secret != nil {
		secret, err := r.secrets.Open(ctx, tenant, *reg.Secret)
		if err != nil {
			return port.OAuthClient{}, err
		}
		c.ClientSecret = secret
	}
	return c, nil
}

func registrationOrDefault(v port.Registration) port.Registration {
	if v == "" {
		return port.RegistrationPreRegistered
	}
	return v
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

func (r *ClientResolver) configured(tenant port.TenantID, issuer string) (port.OAuthClient, bool) {
	for _, t := range []port.TenantID{tenant, ""} {
		if c, ok := r.clients[clientKey{t, issuer}]; ok {
			return c, true
		}
	}
	return port.OAuthClient{}, false
}

// Resolve returns the client stored for the tenant, else the configured one of the tenant, else the default, else a not_found error.
func (r *ClientResolver) Resolve(ctx context.Context, tenant port.TenantID, issuer string) (port.OAuthClient, error) {
	reg, ok, err := r.storedRow(ctx, tenant, issuer)
	if err != nil {
		return port.OAuthClient{}, err
	}
	if ok {
		return r.open(ctx, tenant, reg)
	}
	if c, ok := r.configured(tenant, issuer); ok {
		return c, nil
	}
	return port.OAuthClient{}, port.Errorf(port.CodeNotFound, "no pre-registered client for issuer %s", issuer)
}

func (r *ClientResolver) reusable(reg port.ClientRegistration, app port.ClientApp) bool {
	if reg.Registration != port.RegistrationCIMD && reg.Registration != port.RegistrationDCR {
		return true
	}
	live := reg.ExpiresAt.IsZero() || r.now().Before(reg.ExpiresAt)
	return live && reg.RedirectURL == app.RedirectURL && (reg.Registration != port.RegistrationCIMD || reg.ClientID == app.MetadataURL)
}

// Acquire returns the stored or configured client when one fits, else registers one: through a client metadata document when the issuer supports it and the app has an https address, else through dynamic registration.
// An issuer that allows neither fails with client_registration_required, so the caller asks for a client id and secret.
func (r *ClientResolver) Acquire(ctx context.Context, tenant port.TenantID, issuer string, app port.ClientApp) (port.OAuthClient, error) {
	reg, ok, err := r.storedRow(ctx, tenant, issuer)
	if err != nil {
		return port.OAuthClient{}, err
	}
	if ok && r.reusable(reg, app) {
		return r.open(ctx, tenant, reg)
	}
	if c, ok := r.configured(tenant, issuer); ok {
		return c, nil
	}
	v, err, _ := r.flights.Do(string(tenant)+"|"+issuer+"|"+app.RedirectURL, func() (any, error) { return r.enroll(ctx, tenant, issuer, app) })
	if err != nil {
		return port.OAuthClient{}, err
	}
	return v.(port.OAuthClient), nil
}

func required(format string, args ...any) error {
	return port.Errorf(port.CodeClientRegistrationRequired, format, args...)
}

func (r *ClientResolver) enroll(ctx context.Context, tenant port.TenantID, issuer string, app port.ClientApp) (port.OAuthClient, error) {
	if r.discovery == nil || r.store == nil || tenant == port.Installation || app.RedirectURL == "" {
		return port.OAuthClient{}, required("this server does not allow automatic registration")
	}
	meta, err := r.discovery.Server(ctx, issuer)
	if err != nil {
		return port.OAuthClient{}, err
	}
	if !meta.SupportsS256() {
		return port.OAuthClient{}, required("this server does not allow automatic registration")
	}
	var reg port.ClientRegistration
	switch u, https := fetchable(app.MetadataURL); {
	case meta.ClientIDMetadataDocument && https && u.Scheme == "https" && !egress.IsLoopback(u.Hostname()):
		reg = port.ClientRegistration{ClientID: app.MetadataURL, Registration: port.RegistrationCIMD}
	case meta.RegistrationEndpoint != "":
		if reg, err = r.dynamic(ctx, tenant, meta, app); err != nil {
			return port.OAuthClient{}, err
		}
	default:
		return port.OAuthClient{}, required("this server does not allow automatic registration")
	}
	reg.Tenant, reg.Issuer, reg.RedirectURL, reg.UpdatedAt = tenant, issuer, app.RedirectURL, r.now()
	if err := r.store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		if err := tx.PutClient(reg); err != nil {
			return err
		}
		return tx.Audit(port.AuditRow{At: reg.UpdatedAt, Tenant: tenant, Actor: "client-resolver", Action: "client.registered", Subject: issuer, Detail: map[string]any{"registration": string(reg.Registration)}})
	}); err != nil {
		return port.OAuthClient{}, err
	}
	return r.open(ctx, tenant, reg)
}

func clientName(raw string) string {
	name := strings.TrimSpace(strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return -1
		}
		return c
	}, raw))
	if runes := []rune(name); len(runes) > maxClientName {
		name = string(runes[:maxClientName])
	}
	if name == "" {
		return "connector-engine"
	}
	return name
}

func authMethod(supported []string) string {
	switch {
	case len(supported) == 0 || slices.Contains(supported, "none"):
		return "none"
	case slices.Contains(supported, "client_secret_post"):
		return "client_secret_post"
	}
	return "client_secret_basic"
}

type registeredClient struct {
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret"`
	RegistrationToken     string   `json:"registration_access_token"`
	RegistrationClientURI string   `json:"registration_client_uri"`
	SecretExpiresAt       int64    `json:"client_secret_expires_at"`
	RedirectURIs          []string `json:"redirect_uris"`
}

func (r *ClientResolver) dynamic(ctx context.Context, tenant port.TenantID, meta ServerMetadata, app port.ClientApp) (port.ClientRegistration, error) {
	kind := "web"
	if u, ok := fetchable(app.RedirectURL); ok && u.Scheme == "http" {
		kind = "native"
	}
	body, _ := json.Marshal(map[string]any{
		"client_name": clientName(app.Name), "redirect_uris": []string{app.RedirectURL}, "application_type": kind,
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": authMethod(meta.TokenEndpointAuthMethods),
	})
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.RegistrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return port.ClientRegistration{}, required("this server does not allow automatic registration")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := r.discovery.client().Do(req)
	if err != nil {
		return port.ClientRegistration{}, port.Errorf(port.CodeUnavailable, "the registration endpoint is unreachable")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxMetadataBytes+1))
	switch {
	case res.StatusCode >= 500 || err != nil || len(data) > maxMetadataBytes:
		return port.ClientRegistration{}, port.Errorf(port.CodeUnavailable, "the registration endpoint failed")
	case res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK:
		return port.ClientRegistration{}, required("this server refused automatic registration")
	}
	var got registeredClient
	if json.Unmarshal(data, &got) != nil || got.ClientID == "" || len(got.ClientID) > 2048 || got.RedirectURIs != nil && !slices.Contains(got.RedirectURIs, app.RedirectURL) {
		return port.ClientRegistration{}, port.Errorf(port.CodeUnavailable, "the registration endpoint answered an invalid client")
	}
	return r.seal(ctx, tenant, got)
}

func (r *ClientResolver) seal(ctx context.Context, tenant port.TenantID, got registeredClient) (port.ClientRegistration, error) {
	reg := port.ClientRegistration{ClientID: got.ClientID, Registration: port.RegistrationDCR}
	if secure(got.RegistrationClientURI) {
		reg.RegistrationClientURI = got.RegistrationClientURI
	}
	if got.SecretExpiresAt > 0 {
		reg.ExpiresAt = time.Unix(got.SecretExpiresAt, 0)
	}
	for _, f := range []struct {
		plain string
		into  **port.Sealed
	}{{got.ClientSecret, &reg.Secret}, {got.RegistrationToken, &reg.RegistrationAccessToken}} {
		if f.plain == "" {
			continue
		}
		sealed, err := r.secrets.Seal(ctx, tenant, []byte(f.plain))
		if err != nil {
			return port.ClientRegistration{}, err
		}
		*f.into = &sealed
	}
	return reg, nil
}
