package service_test

import (
	"testing"

	st "github.com/versatile-sh/connector-engine/service/servicetest"
	"github.com/versatile-sh/connector-engine/store/memory"
)

func TestEveryRowTheServiceWritesRestoresOnMemory(t *testing.T) {
	st.RestoreKeepsDecisions(t, memory.New(), memory.New())
}

func TestEveryRowTheServiceWritesRestoresOnPostgres(t *testing.T) {
	st.RestoreKeepsDecisions(t, postgresStore(t), postgresStore(t))
}
