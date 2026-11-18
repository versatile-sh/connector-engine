package credential_test

import (
	"context"
	"testing"

	"github.com/agents-control/connector-engine/credential"
	"github.com/agents-control/connector-engine/port"
	secretsmemory "github.com/agents-control/connector-engine/secrets/memory"
	"github.com/agents-control/connector-engine/store/memory"
)

func TestResolverOpensOnlyTheCredentialOfItsTenantAndUnwrapsOAuthTokens(t *testing.T) {
	ctx, store, secrets := context.Background(), memory.New(), secretsmemory.New()
	seal := func(tenant port.TenantID, plain string) port.Sealed {
		s, err := secrets.Seal(ctx, tenant, []byte(plain))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	put := func(c port.Credential) {
		if err := store.Write(ctx, port.Scope{Tenant: c.Tenant}, func(tx port.Tx) error { _, err := tx.PutCredential(c, port.AnyRevision); return err }); err != nil {
			t.Fatal(err)
		}
	}
	put(port.Credential{ID: "key", Tenant: "t1", Kind: port.CredentialAPIKey, Status: port.CredentialActive, Secret: seal("t1", "sk-1")})
	put(port.Credential{ID: "oauth", Tenant: "t1", Kind: port.CredentialOAuth2User, Status: port.CredentialActive, Secret: seal("t1", `{"access_token":"at-1","refresh_token":"rt"}`)})
	put(port.Credential{ID: "dead", Tenant: "t1", Kind: port.CredentialAPIKey, Status: port.CredentialRevoked, Secret: seal("t1", "sk-2")})
	resolve := credential.NewResolver(store, secrets)
	for ref, want := range map[port.CredentialID]string{"key": "sk-1", "oauth": "at-1"} {
		if _, got, err := resolve(ctx, port.SecretsRef{Tenant: "t1", Credential: ref}); err != nil || string(got) != want {
			t.Errorf("%s: %q %v", ref, got, err)
		}
	}
	if _, _, err := resolve(ctx, port.SecretsRef{Tenant: "t2", Credential: "key"}); !port.IsCode(err, port.CodeNotFound) {
		t.Errorf("another tenant's credential: %v", err)
	}
	if _, _, err := resolve(ctx, port.SecretsRef{Tenant: "t1", Credential: "dead"}); !port.IsCode(err, port.CodeCredentialRequired) {
		t.Errorf("a revoked credential: %v", err)
	}
}
