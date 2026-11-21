package console

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// ErrSignupClosed refuses a sign-up while the Owner keeps sign-up off.
var ErrSignupClosed = &statusError{status: http.StatusForbidden, code: "signup_closed", err: errors.New("sign-up is closed: ask an Owner or Admin for an invite link")}

// SignupSettings is the Owner's choice about open sign-up: whether anyone may create an account, and the role that account gets, member or admin.
type SignupSettings struct {
	Enabled bool `json:"enabled"`
	Role    Role `json:"role"`
}

// Signup returns the sign-up settings, with Member as the role when none was chosen.
func (s *State) Signup() SignupSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.data.Signup
	if out.Role == "" {
		out.Role = RoleMember
	}
	return out
}

// SetSignup sets the sign-up settings as actor and records it; the role is member or admin, never owner, and only an Owner may call it.
func (s *State) SetSignup(actor string, in SignupSettings) error {
	if in.Role != RoleMember && in.Role != RoleAdmin {
		return invalid("people who sign up are members or admins")
	}
	return s.update(func(d *stateFile) error {
		if by, ok := d.Users[actor]; !ok || !by.Can(CapPeopleManage) {
			return errForbidden
		}
		if d.Signup == in {
			return errUnchanged
		}
		record(d, actor, "signup.changed", "signup", map[string]string{"enabled": boolText(in.Enabled), "role": string(in.Role)})
		d.Signup = in
		return nil
	})
}

func boolText(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// SignUp makes a user with the sign-up role when sign-up is on and an Owner already exists; the role is never taken from the caller.
func (s *State) SignUp(username, password string) (User, error) {
	var out User
	err := s.update(func(d *stateFile) error {
		if len(d.Users) == 0 || !d.Signup.Enabled {
			return ErrSignupClosed
		}
		c, err := newCredentials(username, password)
		if err != nil {
			return err
		}
		if err := usernameTaken(d, username, ""); err != nil {
			return err
		}
		c.Person = newPersonID()
		for d.Users[c.Person].Person != "" {
			c.Person = newPersonID()
		}
		c.Role, c.CreatedAt = RoleMember, time.Now().UTC().Truncate(time.Second)
		if d.Signup.Role == RoleAdmin {
			c.Role = RoleAdmin
		}
		put(&d.Users, c.Person, c)
		record(d, c.Person, "user.created", c.Person, map[string]string{"role": string(c.Role), "via": "signup"})
		out = c
		return nil
	})
	return out, err
}

func (c *Console) signup(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	username, ip := strings.TrimSpace(in.Username), clientIP(r)
	if c.signupHits.locked(time.Now(), ip, "") {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, apiError{Error: "too many sign-ups from here, try again in a minute", Code: "slow_down"})
		return
	}
	c.signupHits.fail(time.Now(), ip, "")
	u, err := c.State.SignUp(username, in.Password)
	if err != nil {
		c.fail(w, err)
		return
	}
	if err := c.ReconcileRoles(r.Context()); err != nil {
		go c.untilReady(context.WithoutCancel(r.Context()), "roles", func(ctx context.Context) error { return c.ReconcileRoles(ctx) })
	}
	c.signIn(w, http.StatusCreated, u.Person)
}

func (c *Console) setSignup(w http.ResponseWriter, r *http.Request) {
	var in SignupSettings
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := signedInUser(r.Context())
	if err := c.State.SetSignup(u.Person, in); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
