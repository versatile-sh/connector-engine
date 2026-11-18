package credential

import (
	"context"
	"encoding/json"

	"github.com/agents-control/connector-engine/port"
)

// NewResolver opens the credential a SecretsRef names from the store of its tenant; OAuth credentials give their access token.
func NewResolver(store port.Store, secrets port.SecretStore) Resolver {
	return func(ctx context.Context, ref port.SecretsRef) (port.CredentialKind, []byte, error) {
		var c port.Credential
		err := store.Read(ctx, port.Scope{Tenant: ref.Tenant}, func(tx port.Tx) (err error) {
			c, err = tx.GetCredential(ref.Credential)
			return err
		})
		if err != nil {
			return "", nil, err
		}
		if c.Status != port.CredentialActive {
			return "", nil, port.Errorf(port.CodeCredentialRequired, "credential %s is %s", c.ID, c.Status)
		}
		secret, err := secrets.Open(ctx, ref.Tenant, c.Secret)
		if err != nil {
			return "", nil, err
		}
		if c.Kind == port.CredentialOAuth2User || c.Kind == port.CredentialOAuth2ClientCredentials || c.Kind == port.CredentialIDJAG {
			var tokens struct {
				Access string `json:"access_token"`
			}
			if err := json.Unmarshal(secret, &tokens); err != nil || tokens.Access == "" {
				return "", nil, port.Errorf(port.CodeCredentialRequired, "credential %s holds no access token", c.ID)
			}
			secret = []byte(tokens.Access)
		}
		return c.Kind, secret, nil
	}
}
