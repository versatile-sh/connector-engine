package memory_test

import (
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/secrets/memory"
	"github.com/agents-control/connector-engine/secrets/secretstest"
)

func TestConformance(t *testing.T) {
	secretstest.Run(t, func(*testing.T) port.SecretStore { return memory.New() })
}
