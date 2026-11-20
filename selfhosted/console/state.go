package console

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"
)

const (
	pbkdf2Iterations = 600_000
	pbkdf2KeyLen     = 32
	minPassword      = 10
)

// ErrNoOwner is returned by a login before the owner account exists.
var ErrNoOwner = errors.New("no owner account yet")

// ErrOwnerExists is returned by a second setup.
var ErrOwnerExists = errors.New("the owner account already exists")

// Owner is the single local account: a username and a PBKDF2-SHA256 hash of the password.
type Owner struct {
	Username   string `json:"username"`
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}

// Agent is an agent the owner connected, with the expiry of the last token minted for it and, for an app that connected itself through the browser sign-in, the OAuth client that owns it, when it connected, whether the owner has seen it since and when a replayed refresh token signed it out.
type Agent struct {
	Name        string    `json:"name"`
	ExpiresAt   time.Time `json:"expires_at"`
	ViaBrowser  bool      `json:"via_browser,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
	ConnectedAt time.Time `json:"connected_at,omitzero"`
	Seen        bool      `json:"seen,omitempty"`
	SignedOutAt time.Time `json:"signed_out_at,omitzero"`
}

// StoredSession is a console session as the state file keeps it, keyed by the SHA-256 of its cookie value.
type StoredSession struct {
	CSRF    string    `json:"csrf"`
	Expires time.Time `json:"expires"`
}

type stateFile struct {
	Owner               *Owner                       `json:"owner,omitempty"`
	Agents              map[string]Agent             `json:"agents,omitempty"`
	Sessions            map[string]StoredSession     `json:"sessions,omitempty"`
	Revoked             map[string]time.Time         `json:"revoked,omitempty"`
	OnboardingDismissed bool                         `json:"onboarding_dismissed,omitempty"`
	Clients             map[string]OAuthClient       `json:"oauth_clients,omitempty"`
	Refresh             map[string]RefreshGrant      `json:"oauth_refresh,omitempty"`
	Spent               map[string]RefreshGrant      `json:"oauth_refresh_spent,omitempty"`
	Notify              notifyState                  `json:"notifications,omitzero"`
	LastExport          time.Time                    `json:"last_export,omitzero"`
	Origins             map[string]Origin            `json:"origins,omitempty"`
	NewKeysOff          map[string]bool              `json:"new_keys_off,omitempty"`
	Pending             map[string][]Profile         `json:"pending_profiles,omitempty"`
	NewKeyLimits        map[string]map[string]Access `json:"new_key_limits,omitempty"`
	Slots               map[string]KeySlot           `json:"key_slots,omitempty"`
}

// Origin is where a connector or a skill was imported from, as the import asked for it, so a setup file can import it again.
type Origin struct {
	MCP          string `json:"mcp,omitempty"`
	Listing      string `json:"listing,omitempty"`
	Local        bool   `json:"local,omitempty"`
	HeaderSecret bool   `json:"header_secret,omitempty"`
	Digest       string `json:"digest,omitempty"`
}

// State is the console's file in its volume: the owner account, the connected agents and the revoked ones, written atomically with mode 0600.
type State struct {
	path string
	mu   sync.Mutex
	data stateFile
}

// OpenState reads path, or starts empty when it does not exist.
func OpenState(path string) (*State, error) {
	s := &State{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	migrateSlots(&s.data)
	return s, nil
}

var errUnchanged = errors.New("unchanged")

func (s *State) update(fn func(d *stateFile) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	var next stateFile
	if err := json.Unmarshal(raw, &next); err != nil {
		return err
	}
	switch err := fn(&next); {
	case errors.Is(err, errUnchanged):
		return nil
	case err != nil:
		return err
	}
	if raw, err = json.MarshalIndent(next, "", "  "); err != nil {
		return err
	}
	if err := writeAtomic(s.path, raw); err != nil {
		return err
	}
	s.data = next
	return nil
}

func put[K comparable, V any](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = map[K]V{}
	}
	(*m)[k] = v
}

func writeAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = tmp.Chmod(0o600)
	if err == nil {
		_, err = tmp.Write(raw)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// HasOwner reports whether the owner account exists.
func (s *State) HasOwner() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Owner != nil
}

func hashPassword(password string, salt []byte, iterations int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iterations, pbkdf2KeyLen)
}

func checkUsername(username string) error {
	if username == "" || len(username) > 64 {
		return invalid("the username has 1 to 64 characters")
	}
	return nil
}

func newOwner(username, password string) (*Owner, error) {
	if err := checkUsername(username); err != nil {
		return nil, err
	}
	if len(password) < minPassword {
		return nil, invalid("the password has at least %d characters", minPassword)
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	hash, err := hashPassword(password, salt, pbkdf2Iterations)
	if err != nil {
		return nil, err
	}
	return &Owner{Username: username, Salt: base64.StdEncoding.EncodeToString(salt), Hash: base64.StdEncoding.EncodeToString(hash), Iterations: pbkdf2Iterations}, nil
}

// CreateOwner writes the owner account once; the password has at least ten characters.
func (s *State) CreateOwner(username, password string) error {
	o, err := newOwner(username, password)
	if err != nil {
		return err
	}
	return s.update(func(d *stateFile) error {
		if d.Owner != nil {
			return ErrOwnerExists
		}
		d.Owner = o
		return nil
	})
}

// SetPassword replaces the owner's password and ends every session except the one keyed keep; an empty keep ends them all.
func (s *State) SetPassword(password, keep string) error {
	return s.update(func(d *stateFile) error {
		if d.Owner == nil {
			return ErrNoOwner
		}
		o, err := newOwner(d.Owner.Username, password)
		if err != nil {
			return err
		}
		d.Owner = o
		maps.DeleteFunc(d.Sessions, func(k string, _ StoredSession) bool { return k != keep })
		return nil
	})
}

// Rename changes the owner's username, which has 1 to 64 characters.
func (s *State) Rename(username string) error {
	if err := checkUsername(username); err != nil {
		return err
	}
	return s.update(func(d *stateFile) error {
		if d.Owner == nil {
			return ErrNoOwner
		}
		d.Owner.Username = username
		return nil
	})
}

// CheckLogin reports whether username and password are the owner's, comparing in constant time.
func (s *State) CheckLogin(username, password string) (bool, error) {
	s.mu.Lock()
	o := s.data.Owner
	s.mu.Unlock()
	if o == nil {
		return false, ErrNoOwner
	}
	salt, err1 := base64.StdEncoding.DecodeString(o.Salt)
	want, err2 := base64.StdEncoding.DecodeString(o.Hash)
	if err := errors.Join(err1, err2); err != nil {
		return false, err
	}
	got, err := hashPassword(password, salt, o.Iterations)
	if err != nil {
		return false, err
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(o.Username)) == 1
	passOK := subtle.ConstantTimeCompare(got, want) == 1
	return userOK && passOK, nil
}

// OwnerName returns the owner's username, or "" before the account exists.
func (s *State) OwnerName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Owner == nil {
		return ""
	}
	return s.data.Owner.Username
}

func (s *State) session(key string) (StoredSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Sessions[key]
	return v, ok && time.Now().Before(v.Expires)
}

func (s *State) putSession(key string, v StoredSession) error {
	return s.update(func(d *stateFile) error {
		expire(d.Sessions, time.Now(), func(v StoredSession) time.Time { return v.Expires })
		put(&d.Sessions, key, v)
		return nil
	})
}

func (s *State) endSession(key string) error {
	return s.update(func(d *stateFile) error {
		delete(d.Sessions, key)
		return nil
	})
}

// Agents lists the connected agents.
func (s *State) Agents() []Agent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Agent, 0, len(s.data.Agents))
	for _, a := range s.data.Agents {
		out = append(out, a)
	}
	return out
}

// PutAgent records an agent and the expiry of its newest token, keeping how and when it was connected and clearing a sign-out.
func (s *State) PutAgent(a Agent) error {
	return s.update(func(d *stateFile) error {
		if old, ok := d.Agents[a.Name]; ok {
			a.ViaBrowser, a.ClientID, a.ConnectedAt, a.Seen = old.ViaBrowser, old.ClientID, old.ConnectedAt, old.Seen
		}
		put(&d.Agents, a.Name, a)
		return nil
	})
}

// MarkBrowserConnected records that the OAuth client connected the agent through the browser sign-in at at, unseen by the owner so far.
func (s *State) MarkBrowserConnected(name, clientID string, at time.Time) error {
	return s.update(func(d *stateFile) error {
		a, ok := d.Agents[name]
		if !ok {
			return notFound("no agent is named %s", name)
		}
		a.ViaBrowser, a.ClientID, a.ConnectedAt, a.Seen = true, clientID, at.UTC().Truncate(time.Second), false
		d.Agents[name] = a
		return nil
	})
}

// SeeAgents records that the owner has seen every agent an app connected through the browser sign-in.
func (s *State) SeeAgents() error {
	return s.update(func(d *stateFile) error {
		changed := errUnchanged
		for name, a := range d.Agents {
			if a.ViaBrowser && !a.Seen {
				a.Seen, changed = true, nil
				d.Agents[name] = a
			}
		}
		return changed
	})
}

// RevokeAgent forgets the agent and refuses every token minted for it up to at; it reports false when no such agent was ever connected.
func (s *State) RevokeAgent(name string, at time.Time) (bool, error) {
	found := false
	err := s.update(func(d *stateFile) error {
		_, known := d.Agents[name]
		_, revoked := d.Revoked[name]
		if found = known || revoked; !found {
			return errUnchanged
		}
		delete(d.Agents, name)
		put(&d.Revoked, name, at.UTC())
		return nil
	})
	return found && err == nil, err
}

// Revoked reports whether a token of agent issued at iat (Unix seconds) was minted before the agent was revoked.
func (s *State) Revoked(agent string, iat int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.data.Revoked[agent]
	return ok && iat <= at.Unix()
}

// OnboardingDismissed reports whether the first-run guide was closed or finished, for good.
func (s *State) OnboardingDismissed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.OnboardingDismissed
}

// DismissOnboarding closes the first-run guide for good, on every browser and across restarts.
func (s *State) DismissOnboarding() error {
	return s.update(func(d *stateFile) error {
		if d.OnboardingDismissed {
			return errUnchanged
		}
		d.OnboardingDismissed = true
		return nil
	})
}

// LastExport returns when the last full backup was exported, or the zero time when none was.
func (s *State) LastExport() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.LastExport
}

// MarkExported records at as the time of the last full backup.
func (s *State) MarkExported(at time.Time) error {
	return s.update(func(d *stateFile) error {
		d.LastExport = at.UTC().Truncate(time.Second)
		return nil
	})
}

// PutOrigin records where the connector or skill id was imported from.
func (s *State) PutOrigin(id string, o Origin) error {
	return s.update(func(d *stateFile) error {
		put(&d.Origins, id, o)
		return nil
	})
}

// OriginOf returns where the connector or skill id was imported from, if the console recorded it.
func (s *State) OriginOf(id string) (Origin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.data.Origins[id]
	return o, ok
}

// SetNewKeysOff records whether access keys made from now on start off on the connector instead of following its choices.
func (s *State) SetNewKeysOff(conn string, off bool) error {
	return s.update(func(d *stateFile) error {
		if !off {
			delete(d.NewKeysOff, conn)
			return nil
		}
		put(&d.NewKeysOff, conn, true)
		return nil
	})
}

// NewKeysOff reports whether access keys made from now on start off on the connector.
func (s *State) NewKeysOff(conn string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.NewKeysOff[conn]
}

// ForgetConnector drops the new-key default and the new-key limits of a removed connector and its profile from every permission set, and a set left empty.
func (s *State) ForgetConnector(conn string) error {
	return s.update(func(d *stateFile) error {
		delete(d.NewKeysOff, conn)
		delete(d.NewKeyLimits, conn)
		for name, slot := range d.Slots {
			delete(slot.Profiles, conn)
			if len(slot.Profiles) == 0 {
				delete(d.Slots, name)
			}
		}
		return nil
	})
}

// LimitNewKeys records that access keys made from now on get at most a on the connector's function or item, kept from an always allow given to one key.
func (s *State) LimitNewKeys(conn, name string, a Access) error {
	return s.update(func(d *stateFile) error {
		limits := maps.Clone(d.NewKeyLimits[conn])
		put(&limits, name, a)
		put(&d.NewKeyLimits, conn, limits)
		return nil
	})
}

// NewKeyLimits returns the most access keys made from now on get on the connector's functions and items, by name.
func (s *State) NewKeyLimits(conn string) map[string]Access {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.data.NewKeyLimits[conn])
}

// LiftNewKeyLimits forgets the new-key limits of the named functions and items once the owner sets their choice again.
func (s *State) LiftNewKeyLimits(conn string, names []string) error {
	return s.update(func(d *stateFile) error {
		limits := d.NewKeyLimits[conn]
		if !slices.ContainsFunc(names, func(n string) bool { _, ok := limits[n]; return ok }) {
			return errUnchanged
		}
		limits = maps.Clone(limits)
		for _, n := range names {
			delete(limits, n)
		}
		if len(limits) == 0 {
			delete(d.NewKeyLimits, conn)
		} else {
			d.NewKeyLimits[conn] = limits
		}
		return nil
	})
}
