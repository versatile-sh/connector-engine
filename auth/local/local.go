// Package local is a token issuer for the quickstart: it signs EdDSA tokens and publishes their JWKS. It is an acknowledged opt-in, never a default.
package local

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/agents-control/connector-engine/auth/internal/wire"
	"github.com/agents-control/connector-engine/port"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Issuer signs tokens under one name with its newest key and publishes every key it still keeps.
type Issuer struct {
	name   string
	signer jose.Signer
	keys   jose.JSONWebKeySet
	now    func() time.Time
}

// New makes an issuer named name, which becomes the iss claim, with a key made now that is never persisted; a nil clock takes time.Now.
func New(name string, now func() time.Time) (*Issuer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return build(name, now, []ed25519.PrivateKey{priv})
}

func keyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}

func build(name string, now func() time.Time, privs []ed25519.PrivateKey) (*Issuer, error) {
	if now == nil {
		now = time.Now
	}
	signing := privs[len(privs)-1]
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: signing, KeyID: keyID(signing.Public().(ed25519.PublicKey))}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return nil, err
	}
	i := &Issuer{name: name, signer: signer, now: now}
	for _, p := range privs {
		pub := p.Public().(ed25519.PublicKey)
		i.keys.Keys = append(i.keys.Keys, jose.JSONWebKey{Key: pub, KeyID: keyID(pub), Algorithm: string(jose.EdDSA), Use: "sig"})
	}
	return i, nil
}

// Name is the iss claim of every token.
func (i *Issuer) Name() string { return i.name }

// Issue signs the claims as issuer i; a zero IssuedAt becomes now, and an expiry is required.
func (i *Issuer) Issue(c port.Claims) (string, error) {
	if c.ExpiresAt.IsZero() {
		return "", port.Errorf(port.CodeInvalidArgument, "a token expires")
	}
	c.Issuer = i.name
	if c.IssuedAt.IsZero() {
		c.IssuedAt = i.now()
	}
	tok, err := jwt.Signed(i.signer).Claims(wire.From(c)).Serialize()
	if err != nil {
		return "", port.Errorf(port.CodeInternal, "sign token: %v", err)
	}
	return tok, nil
}

// Handler serves the public key set as JSON on every path.
func (i *Issuer) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(i.keys)
	})
}
