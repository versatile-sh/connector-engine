package memory_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/memory"
	"github.com/versatile-sh/connector-engine/secrets/secretstest"
)

func TestConformance(t *testing.T) {
	secretstest.Run(t, func(*testing.T) port.SecretStore { return memory.New() })
}
