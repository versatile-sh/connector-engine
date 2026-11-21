package console

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"maps"
	"net/http"
	"slices"
	"time"
)

// Purpose is what a link is for: joining with a role, or setting a new password for one user.
type Purpose string

const (
	// PurposeJoin makes a new user with the invite's role.
	PurposeJoin Purpose = "join"
	// PurposeReset sets a new password for the user the link names.
	PurposeReset Purpose = "reset"
)

const (
	inviteTTL  = 72 * time.Hour
	maxInvites = 20
)

// ErrLinkGone refuses a link that was used, revoked or expired, that never existed or whose creator lost the role that allowed it.
var ErrLinkGone = &statusError{status: http.StatusGone, code: "link_gone", err: errors.New("this link no longer works")}

// ErrTooManyLinks refuses a new link while twenty wait.
var ErrTooManyLinks = &statusError{status: http.StatusConflict, code: "too_many_links", err: errors.New("twenty links already wait: revoke one first")}

// Invite is a link waiting to be used: only the SHA-256 of its token is kept, so a leaked state file opens no link.
type Invite struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	Purpose   Purpose   `json:"purpose"`
	Role      Role      `json:"role,omitempty"`
	Person    string    `json:"person,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewLink is a link just made: its token is shown once and never stored.
type NewLink struct {
	ID        string    `json:"id"`
	Token     string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LinkFacts is what a live link says before it is used.
type LinkFacts struct {
	Purpose   Purpose   `json:"purpose"`
	Role      Role      `json:"role"`
	InvitedBy string    `json:"invited_by"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func newPersonID() string { return "p-" + randomHex(8) }

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func mayResetFor(creator, target User) bool {
	return creator.Can(CapPeopleReset) && creator.Person != target.Person && (creator.Role == RoleOwner || !target.Role.AtLeast(creator.Role))
}

func mayRevokeFor(by User, inv Invite) bool {
	return by.Can(CapPeopleManage) || inv.Purpose == PurposeReset && inv.CreatedBy == by.Person && by.Can(CapPeopleReset)
}

func linkLive(d *stateFile, inv Invite, now time.Time) bool {
	creator, ok := d.Users[inv.CreatedBy]
	if !ok || now.After(inv.ExpiresAt) {
		return false
	}
	if inv.Purpose == PurposeJoin {
		return creator.Role == RoleOwner
	}
	target, ok := d.Users[inv.Person]
	return ok && mayResetFor(creator, target)
}

func dropDeadLinks(d *stateFile, now time.Time) {
	maps.DeleteFunc(d.Invites, func(_ string, inv Invite) bool { return !linkLive(d, inv, now) })
}

func findInvite(d *stateFile, token string) (Invite, bool) {
	want := []byte(tokenHash(token))
	var found Invite
	ok := false
	for _, inv := range d.Invites {
		if subtle.ConstantTimeCompare(want, []byte(inv.Hash)) == 1 {
			found, ok = inv, true
		}
	}
	return found, ok
}

func (s *State) makeLink(creator string, purpose Purpose, role Role, person string) (NewLink, error) {
	raw := make([]byte, 32)
	rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC().Truncate(time.Second)
	link := NewLink{ID: randomHex(8), Token: token, ExpiresAt: now.Add(inviteTTL)}
	err := s.update(func(d *stateFile) error {
		by, ok := d.Users[creator]
		if !ok {
			return notFound("no user is %s", creator)
		}
		detail := map[string]string{"purpose": string(purpose)}
		action, target := "invite.created", link.ID
		switch purpose {
		case PurposeJoin:
			if !by.Can(CapPeopleManage) {
				return errForbidden
			}
			detail["role"] = string(role)
		case PurposeReset:
			t, ok := d.Users[person]
			if !ok {
				return notFound("no user is %s", person)
			}
			if !mayResetFor(by, t) {
				return errForbidden
			}
			maps.DeleteFunc(d.Invites, func(_ string, inv Invite) bool { return inv.Purpose == PurposeReset && inv.Person == person })
			action, target = "reset.created", person
		}
		dropDeadLinks(d, now)
		if len(d.Invites) >= maxInvites {
			return ErrTooManyLinks
		}
		put(&d.Invites, link.ID, Invite{ID: link.ID, Hash: tokenHash(token), Purpose: purpose, Role: role, Person: person, CreatedBy: creator, CreatedAt: now, ExpiresAt: link.ExpiresAt})
		record(d, creator, action, target, detail)
		return nil
	})
	return link, err
}

// CreateJoinLink makes a single-use link, good for 72 hours, that lets one person join with role; only an Owner makes one.
func (s *State) CreateJoinLink(creator string, role Role) (NewLink, error) {
	if _, err := ParseRole(string(role)); err != nil {
		return NewLink{}, err
	}
	return s.makeLink(creator, PurposeJoin, role, "")
}

// CreateResetLink makes a single-use link that sets a new password for person, replacing their earlier one; an Owner resets any other user and an Admin only a Member.
func (s *State) CreateResetLink(creator, person string) (NewLink, error) {
	return s.makeLink(creator, PurposeReset, "", person)
}

// Links lists the links still able to work, newest first.
func (s *State) Links() []Invite {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := []Invite{}
	for _, inv := range s.data.Invites {
		if linkLive(&s.data, inv, now) {
			out = append(out, inv)
		}
	}
	slices.SortFunc(out, func(a, b Invite) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out
}

// RevokeLink deletes a link: any join link for an Owner, and a reset link for its creator or an Owner.
func (s *State) RevokeLink(actor, id string) error {
	return s.update(func(d *stateFile) error {
		inv, ok := d.Invites[id]
		by := d.Users[actor]
		if !ok {
			return notFound("no link is %s", id)
		}
		if !mayRevokeFor(by, inv) {
			return errForbidden
		}
		delete(d.Invites, id)
		record(d, actor, "invite.revoked", id, map[string]string{"purpose": string(inv.Purpose)})
		return nil
	})
}

// InspectLink says what a live link is for, without using it.
func (s *State) InspectLink(token string) (LinkFacts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := findInvite(&s.data, token)
	if !ok || !linkLive(&s.data, inv, time.Now()) {
		return LinkFacts{}, ErrLinkGone
	}
	facts := LinkFacts{Purpose: inv.Purpose, Role: inv.Role, InvitedBy: s.data.Users[inv.CreatedBy].Username, ExpiresAt: inv.ExpiresAt}
	if inv.Purpose == PurposeReset {
		t := s.data.Users[inv.Person]
		facts.Username, facts.Role = t.Username, t.Role
	}
	return facts, nil
}

// AcceptLink uses a live link once: a join link makes the user username with password and the link's role, a reset link sets the named user's password and ends all their sessions and keeps everything else; a refused password or username leaves the link usable.
func (s *State) AcceptLink(token, username, password string) (User, error) {
	var out User
	err := s.update(func(d *stateFile) error {
		now := time.Now()
		inv, ok := findInvite(d, token)
		if !ok || !linkLive(d, inv, now) {
			return ErrLinkGone
		}
		if inv.Purpose == PurposeReset {
			u := d.Users[inv.Person]
			c, err := newCredentials(u.Username, password)
			if err != nil {
				return err
			}
			u.Salt, u.Hash, u.Iterations = c.Salt, c.Hash, c.Iterations
			d.Users[u.Person] = u
			maps.DeleteFunc(d.Sessions, func(_ string, v StoredSession) bool { return v.Person == u.Person })
			record(d, u.Person, "reset.used", u.Person, map[string]string{"by": inv.CreatedBy})
			out = u
		} else {
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
			c.Role, c.InvitedBy, c.CreatedAt = inv.Role, inv.CreatedBy, now.UTC().Truncate(time.Second)
			put(&d.Users, c.Person, c)
			record(d, inv.CreatedBy, "user.created", c.Person, map[string]string{"role": string(c.Role), "via": "invite"})
			out = c
		}
		delete(d.Invites, inv.ID)
		dropDeadLinks(d, now)
		return nil
	})
	return out, err
}
