package console

import "testing"

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
