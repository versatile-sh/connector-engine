package console

import (
	"context"
	"testing"
)

// UseListingProbe replaces how a listing preview probes its remote server.
func (c *Console) UseListingProbe(probe func(context.Context, string, Reach) (Probed, error)) {
	c.probe = probe
}

func AddTestUser(t testing.TB, s *State, person, username, password string, role Role) {
	t.Helper()
	u, err := newCredentials(username, password)
	if err != nil {
		t.Fatal(err)
	}
	u.Person, u.Role = person, role
	if err := s.update(func(d *stateFile) error { put(&d.Users, person, u); return nil }); err != nil {
		t.Fatal(err)
	}
}
