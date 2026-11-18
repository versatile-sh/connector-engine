// Package digest is the ArtifactVerifier that checks the SHA-256 digest of a listing artifact.
package digest

import (
	"context"

	internal "github.com/agents-control/connector-engine/internal/digest"
	"github.com/agents-control/connector-engine/port"
)

// Verifier checks a digest; a signature is out of scope, so a listing is never reported as signed.
type Verifier struct{}

// Verify compares the artifact with the manifest digest; a mismatch is an invalid_manifest error and a missing digest verifies nothing.
func (Verifier) Verify(_ context.Context, m port.Manifest, artifact []byte) (port.Verification, error) {
	if m.Artifact == nil || m.Artifact.Digest == "" {
		return port.Verification{}, nil
	}
	if !internal.Matches(m.Artifact.Digest, artifact) {
		return port.Verification{}, port.Errorf(port.CodeInvalidManifest, "the artifact of %s %s does not match its digest", m.Name, m.Version)
	}
	return port.Verification{DigestVerified: true}, nil
}
