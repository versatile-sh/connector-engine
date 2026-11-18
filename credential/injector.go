// Package credential injects upstream credentials into outbound requests and brokers pre-registered OAuth clients.
package credential

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/agents-control/connector-engine/port"
)

// Injector writes a credential into the headers of an outbound request; the zero value sends API keys as a bearer token.
type Injector struct {
	APIKeyHeader string
	APIKeyPrefix string
}

// Inject sets the header or headers the credential kind calls for; basic secrets are user:password and custom_headers secrets are a JSON object.
func (i Injector) Inject(_ context.Context, kind port.CredentialKind, secret []byte, out *port.Outbound) error {
	if out.Headers == nil {
		out.Headers = map[string][]string{}
	}
	switch kind {
	case port.CredentialAPIKey:
		header, prefix := i.APIKeyHeader, i.APIKeyPrefix
		if header == "" {
			header, prefix = "Authorization", "Bearer "
		}
		out.Headers[header] = []string{prefix + string(secret)}
	case port.CredentialOAuth2User, port.CredentialOAuth2ClientCredentials, port.CredentialIDJAG:
		out.Headers["Authorization"] = []string{"Bearer " + string(secret)}
	case port.CredentialBasic:
		out.Headers["Authorization"] = []string{"Basic " + base64.StdEncoding.EncodeToString(secret)}
	case port.CredentialCustomHeaders:
		var headers map[string]string
		if err := json.Unmarshal(secret, &headers); err != nil {
			return port.Errorf(port.CodeInvalidArgument, "a custom_headers secret is a JSON object of strings")
		}
		for k, v := range headers {
			out.Headers[k] = []string{v}
		}
	default:
		return port.Errorf(port.CodeInvalidArgument, "credential kind %q cannot be injected", kind)
	}
	return nil
}
