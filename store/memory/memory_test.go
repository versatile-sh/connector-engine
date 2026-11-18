package memory_test

import (
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/store/memory"
	"github.com/agents-control/connector-engine/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(*testing.T) (port.Store, port.Outbox) {
		s := memory.New()
		return s, s
	})
}
