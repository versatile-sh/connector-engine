package digest_test

import (
	"context"
	"testing"

	"github.com/agents-control/connector-engine/artifact/digest"
	internal "github.com/agents-control/connector-engine/internal/digest"
	"github.com/agents-control/connector-engine/port"
)

func TestVerify(t *testing.T) {
	data := []byte("artifact bytes")
	sum := internal.Sum(data)
	signed := &port.Signature{Type: "cosign", Identity: "me"}
	cases := []struct {
		name     string
		m        port.Manifest
		verified bool
		code     port.Code
	}{
		{"matching digest", port.Manifest{Artifact: &port.Artifact{Digest: sum}}, true, ""},
		{"prefixed digest", port.Manifest{Artifact: &port.Artifact{Digest: "sha256:" + sum}}, true, ""},
		{"signature present is not verified", port.Manifest{Artifact: &port.Artifact{Digest: sum}, Signature: signed}, true, ""},
		{"wrong digest", port.Manifest{Artifact: &port.Artifact{Digest: "00"}}, false, port.CodeInvalidManifest},
		{"no digest", port.Manifest{Artifact: &port.Artifact{}}, false, ""},
		{"no artifact", port.Manifest{}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := digest.Verifier{}.Verify(context.Background(), c.m, data)
			if c.code != "" && !port.IsCode(err, c.code) || c.code == "" && err != nil {
				t.Fatalf("err = %v", err)
			}
			if v.DigestVerified != c.verified || v.Signed {
				t.Fatalf("verification %+v", v)
			}
		})
	}
}
