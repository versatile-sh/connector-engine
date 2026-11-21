package service_test

import (
	"context"
	"testing"

	"github.com/versatile-sh/connector-engine/store/postgres"
	"github.com/versatile-sh/connector-engine/store/storetest"
)

func postgresStore(tb testing.TB) *postgres.Store {
	tb.Helper()
	pg, err := storetest.StartPostgres()
	if err != nil {
		tb.Skip(err)
	}
	tb.Cleanup(pg.Stop)
	dsn, err := pg.NewDatabase()
	if err != nil {
		tb.Fatal(err)
	}
	s, err := postgres.New(context.Background(), dsn)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(s.Close)
	if err := s.Migrate(context.Background()); err != nil {
		tb.Fatal(err)
	}
	return s
}
