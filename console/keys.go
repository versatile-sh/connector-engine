package console

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/agents-control/plugkit/discovery"
)

// keyProtection reports the weakest way the engine's and the console's root keys rest: locked, plaintext or wrapped; empty when neither says.
func (c *Console) keyProtection(ctx context.Context) string {
	console := c.Secrets.Protection()
	if console == "locked" || console == "plaintext" {
		return console
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var doc discovery.Document
	if err := c.Engine.do(ctx, c.Engine.OperatorURL, c.Engine.Operator, http.MethodGet, "/v1/adapters", nil, &doc); err != nil {
		return console
	}
	for _, row := range doc.Adapters {
		if row.Port == "secrets" && (row.Name == "localfile" || slices.Contains(row.Capabilities, "secrets.root_key_plaintext")) {
			return "plaintext"
		}
	}
	return console
}
