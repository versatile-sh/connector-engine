package console

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Role is what a console user may do, ordered member < admin < owner.
type Role string

const (
	// RoleMember uses the gateway with their own keys.
	RoleMember Role = "member"
	// RoleAdmin also manages connectors, skills and policy.
	RoleAdmin Role = "admin"
	// RoleOwner may do everything, including managing people and full backups.
	RoleOwner Role = "owner"
)

var roleRank = map[Role]int{RoleMember: 1, RoleAdmin: 2, RoleOwner: 3}

// ParseRole returns the role named s and refuses any value outside owner, admin and member.
func ParseRole(s string) (Role, error) {
	if r := Role(s); roleRank[r] != 0 {
		return r, nil
	}
	return "", invalid("the role %q is not owner, admin or member", s)
}

// AtLeast reports whether r ranks at or above o; an unknown role ranks below every known one.
func (r Role) AtLeast(o Role) bool { return roleRank[r] >= roleRank[o] && roleRank[r] > 0 }

// User is one console account: its engine person id, its username, its role and a PBKDF2-SHA256 hash of its password.
type User struct {
	Person     string    `json:"person"`
	Username   string    `json:"username"`
	Role       Role      `json:"role"`
	Salt       string    `json:"salt"`
	Hash       string    `json:"hash"`
	Iterations int       `json:"iterations"`
	CreatedAt  time.Time `json:"created_at,omitzero"`
	InvitedBy  string    `json:"invited_by,omitempty"`
}

// ErrLastOwner refuses a change that would leave the console without an Owner.
var ErrLastOwner = &statusError{status: http.StatusConflict, err: errors.New("the console keeps at least one owner")}

const (
	defaultPerson        = "owner"
	defaultServicePerson = "console"
	defaultManagersGroup = "managers"
)

var dummySalt = []byte("connector-engine/no-such-user")

func newCredentials(username, password string) (User, error) {
	if err := checkUsername(username); err != nil {
		return User{}, err
	}
	if len(password) < minPassword {
		return User{}, invalid("the password has at least %d characters", minPassword)
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	hash, err := hashPassword(password, salt, pbkdf2Iterations)
	if err != nil {
		return User{}, err
	}
	return User{Username: username, Salt: base64.StdEncoding.EncodeToString(salt), Hash: base64.StdEncoding.EncodeToString(hash), Iterations: pbkdf2Iterations}, nil
}

func owners(d *stateFile) int {
	n := 0
	for _, u := range d.Users {
		if u.Role == RoleOwner {
			n++
		}
	}
	return n
}

func findUser(d *stateFile, username string) (User, bool) {
	for _, u := range d.Users {
		if strings.EqualFold(u.Username, username) {
			return u, true
		}
	}
	return User{}, false
}

func usernameTaken(d *stateFile, username, except string) error {
	if u, ok := findUser(d, username); ok && u.Person != except {
		return &statusError{status: http.StatusConflict, err: errors.New("this username is taken")}
	}
	return nil
}

func migrateUsers(d *stateFile, person string) {
	if d.Owner != nil && len(d.Users) == 0 {
		o := d.Owner
		put(&d.Users, person, User{Person: person, Username: o.Username, Role: RoleOwner, Salt: o.Salt, Hash: o.Hash, Iterations: o.Iterations})
		d.LegacyPerson = person
	}
	d.Owner = nil
	for k, a := range d.Agents {
		if a.Owner == "" {
			a.Owner = firstOf(d.LegacyPerson, person)
			d.Agents[k] = a
		}
	}
	for k, ch := range d.Notify.Channels {
		if ch.Owner == "" {
			ch.Owner = firstOf(d.LegacyPerson, person)
			d.Notify.Channels[k] = ch
		}
	}
	for k, dev := range d.Notify.Devices {
		if dev.Owner == "" {
			dev.Owner = firstOf(d.LegacyPerson, person)
			d.Notify.Devices[k] = dev
		}
	}
	for k, v := range d.Sessions {
		if v.Person == "" {
			v.Person = d.LegacyPerson
			d.Sessions[k] = v
		}
	}
}

// HasOwner reports whether any user exists, which ends the first run.
func (s *State) HasOwner() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Users) > 0
}

// User returns the user whose person id is person.
func (s *State) User(person string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.data.Users[person]
	return u, ok
}

// Users lists every console user.
func (s *State) Users() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Values(s.data.Users))
}

// CreateFirstOwner writes the first Owner, as the configured person, while no user exists; the password has at least ten characters.
func (s *State) CreateFirstOwner(username, password string) (User, error) {
	u, err := newCredentials(username, password)
	if err != nil {
		return User{}, err
	}
	u.Person, u.Role, u.CreatedAt = s.person, RoleOwner, time.Now().UTC().Truncate(time.Second)
	err = s.update(func(d *stateFile) error {
		if len(d.Users) > 0 {
			return ErrOwnerExists
		}
		put(&d.Users, u.Person, u)
		return nil
	})
	return u, err
}

// SetPassword replaces the password of person and ends every session of that user except the one keyed keep; an empty keep ends them all.
func (s *State) SetPassword(person, password, keep string) error {
	return s.update(func(d *stateFile) error {
		u, ok := d.Users[person]
		if !ok {
			return notFound("no user is %s", person)
		}
		c, err := newCredentials(u.Username, password)
		if err != nil {
			return err
		}
		u.Salt, u.Hash, u.Iterations = c.Salt, c.Hash, c.Iterations
		d.Users[person] = u
		maps.DeleteFunc(d.Sessions, func(k string, v StoredSession) bool { return v.Person == person && k != keep })
		return nil
	})
}

// Rename changes the username of person, which has 1 to 64 characters and is unique case-insensitively.
func (s *State) Rename(person, username string) error {
	if err := checkUsername(username); err != nil {
		return err
	}
	return s.update(func(d *stateFile) error {
		u, ok := d.Users[person]
		if !ok {
			return notFound("no user is %s", person)
		}
		if err := usernameTaken(d, username, person); err != nil {
			return err
		}
		u.Username = username
		d.Users[person] = u
		return nil
	})
}

// SetRole changes the role of person; demoting the last Owner is refused.
func (s *State) SetRole(person string, role Role) error {
	if _, err := ParseRole(string(role)); err != nil {
		return err
	}
	return s.update(func(d *stateFile) error {
		u, ok := d.Users[person]
		if !ok {
			return notFound("no user is %s", person)
		}
		if u.Role == role {
			return errUnchanged
		}
		u.Role = role
		d.Users[person] = u
		return nil
	})
}

// RemoveUser deletes person and their sessions; removing the last Owner is refused.
func (s *State) RemoveUser(person string) error {
	return s.update(func(d *stateFile) error {
		if _, ok := d.Users[person]; !ok {
			return notFound("no user is %s", person)
		}
		delete(d.Users, person)
		maps.DeleteFunc(d.Sessions, func(_ string, v StoredSession) bool { return v.Person == person })
		return nil
	})
}

// CheckLogin returns the user whose case-folded username and password match, hashing a dummy for an unknown username so timing does not tell; the password is compared in constant time.
func (s *State) CheckLogin(username, password string) (User, bool, error) {
	s.mu.Lock()
	none := len(s.data.Users) == 0
	u, found := findUser(&s.data, username)
	s.mu.Unlock()
	if none {
		return User{}, false, ErrNoOwner
	}
	if !found {
		_, err := hashPassword(password, dummySalt, pbkdf2Iterations)
		return User{}, false, err
	}
	salt, err1 := base64.StdEncoding.DecodeString(u.Salt)
	want, err2 := base64.StdEncoding.DecodeString(u.Hash)
	if err := errors.Join(err1, err2); err != nil {
		return User{}, false, err
	}
	got, err := hashPassword(password, salt, u.Iterations)
	if err != nil {
		return User{}, false, err
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return User{}, false, nil
	}
	return u, true, nil
}

// HostUser returns the user a host reset acts on: the one named username, or the only user when username is empty.
func (s *State) HostUser(username string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case username != "":
		if u, ok := findUser(&s.data, username); ok {
			return u, nil
		}
		return User{}, notFound("no user is named %s", username)
	case len(s.data.Users) == 1:
		return slices.Collect(maps.Values(s.data.Users))[0], nil
	case len(s.data.Users) == 0:
		return User{}, ErrNoOwner
	}
	return User{}, invalid("this console has several users: name one with USER=<username>")
}

// HostOwner returns the Owner a host request acts as: the first Owner while they still are one, else the Owner with the smallest person id.
func (s *State) HostOwner() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := firstOf(s.data.LegacyPerson, s.person)
	if s.data.Users[first].Role == RoleOwner {
		return first
	}
	persons := slices.Sorted(maps.Keys(s.data.Users))
	if i := slices.IndexFunc(persons, func(p string) bool { return s.data.Users[p].Role == RoleOwner }); i >= 0 {
		return persons[i]
	}
	return ""
}

type userKey struct{}

func withUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

func signedInUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}
