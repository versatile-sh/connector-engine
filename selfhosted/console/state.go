package console

import (
	"cmp"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
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

// ErrNoOwner is returned by a login before any user exists.
var ErrNoOwner = errors.New("no owner account yet")

// ErrOwnerExists is returned by a second setup.
var ErrOwnerExists = errors.New("the owner account already exists")

// Owner is the single account of a console from before multi-user, read once to migrate it to a User and never written again.
type Owner struct {
	Username   string `json:"username"`
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}

// Agent is an access key: its owner's person, the expiry of the last token minted for it and, for an app that connected itself through the browser sign-in, the OAuth client, when it connected, whether it was seen since and when a replayed refresh token signed it out.
type Agent struct {
	Name        string    `json:"name"`
	Owner       string    `json:"owner,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	ViaBrowser  bool      `json:"via_browser,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
	ConnectedAt time.Time `json:"connected_at,omitzero"`
	Seen        bool      `json:"seen,omitempty"`
	SignedOutAt time.Time `json:"signed_out_at,omitzero"`
	App         string    `json:"app,omitempty"`
}

// StoredSession is a console session as the state file keeps it, keyed by the SHA-256 of its cookie value, and the person it belongs to.
type StoredSession struct {
	Person  string    `json:"person,omitempty"`
	CSRF    string    `json:"csrf"`
	Expires time.Time `json:"expires"`
}

type stateFile struct {
	Format              int                          `json:"format"`
	Owner               *Owner                       `json:"owner,omitempty"`
	Users               map[string]User              `json:"users,omitempty"`
	LegacyPerson        string                       `json:"legacy_person,omitempty"`
	Invites             map[string]Invite            `json:"invites,omitempty"`
	Audit               []AuditEntry                 `json:"audit,omitempty"`
	Agents              map[string]Agent             `json:"agents,omitempty"`
	Sessions            map[string]StoredSession     `json:"sessions,omitempty"`
	Revoked             map[string]time.Time         `json:"revoked,omitempty"`
	OnboardingDismissed bool                         `json:"onboarding_dismissed,omitempty"`
	ClassesRepaired     bool                         `json:"classes_repaired,omitempty"`
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
	Credentials         map[string]CredentialMode    `json:"credential_modes,omitempty"`
	Signup              SignupSettings               `json:"signup,omitzero"`
}

// Origin is where a connector or a skill was imported from, as the import asked for it, so a setup file can import it again.
type Origin struct {
	MCP          string `json:"mcp,omitempty"`
	Listing      string `json:"listing,omitempty"`
	Local        bool   `json:"local,omitempty"`
	HeaderSecret bool   `json:"header_secret,omitempty"`
	Digest       string `json:"digest,omitempty"`
}

// State is the console's file in its volume: the users and their sessions, the connected agents and the revoked ones, written atomically with mode 0600.
type State struct {
	path   string
	person string
	mu     sync.Mutex
	data   stateFile
}

// OpenState reads path, or starts empty when it does not exist; person, owner when empty, is the first Owner's person id and the owner of a legacy account.
func OpenState(path, person string) (*State, error) {
	if person == "" {
		person = defaultPerson
	}
	s := &State{path: path, person: person}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.data.Format = stateFormat
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := upgradeState(&s.data, person); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

const stateFormat = 2

var stateSteps = []func(d *stateFile, person string){
	func(d *stateFile, person string) {
		migrateSlots(d)
		migrateUsers(d, person)
	},
	func(d *stateFile, _ string) { d.Signup = SignupSettings{Role: RoleMember} },
}

func upgradeState(d *stateFile, person string) error {
	if d.Format > stateFormat {
		return fmt.Errorf("the state file has format %d and this console reads up to format %d: run a newer console, or restore a backup made by this one", d.Format, stateFormat)
	}
	if d.Format < 0 {
		return fmt.Errorf("the state file has format %d", d.Format)
	}
	for _, step := range stateSteps[d.Format:] {
		step(d, person)
	}
	d.Format = stateFormat
	return nil
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
	if len(s.data.Users) > 0 && owners(&next) == 0 {
		return ErrLastOwner
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

func hashPassword(password string, salt []byte, iterations int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iterations, pbkdf2KeyLen)
}

func checkUsername(username string) error {
	if username == "" || len(username) > 64 {
		return invalid("the username has 1 to 64 characters")
	}
	return nil
}

func (s *State) session(key string) (StoredSession, User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Sessions[key]
	u, known := s.data.Users[v.Person]
	return v, u, ok && known && time.Now().Before(v.Expires)
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

// ErrNameTaken refuses an access key name that another person's key already has.
var ErrNameTaken = &statusError{status: http.StatusConflict, err: errors.New("this name is taken")}

// PutAgent records an agent and the expiry of its newest token, keeping its owner and how and when it was connected and clearing a sign-out; a key needs an owner, and another owner than the key's is refused.
func (s *State) PutAgent(a Agent) error {
	return s.update(func(d *stateFile) error {
		old, ok := d.Agents[a.Name]
		switch {
		case ok && a.Owner != "" && a.Owner != old.Owner:
			return ErrNameTaken
		case ok:
			a.Owner, a.ViaBrowser, a.ClientID, a.ConnectedAt, a.Seen, a.App = old.Owner, old.ViaBrowser, old.ClientID, old.ConnectedAt, old.Seen, old.App
		case a.Owner == "":
			return invalid("the access key %s has no owner", a.Name)
		}
		put(&d.Agents, a.Name, a)
		return nil
	})
}

// SetAgentApp records which AI app the owner picked for the access key name; an empty id clears it.
func (s *State) SetAgentApp(name, app string) error {
	return s.update(func(d *stateFile) error {
		a, ok := d.Agents[name]
		if !ok {
			return notFound("no agent is named %s", name)
		}
		a.App = app
		d.Agents[name] = a
		return nil
	})
}

// KeyOwner returns the person owning the access key name, and false for a key the console does not know.
func (s *State) KeyOwner(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.data.Agents[name]
	return a.Owner, ok && a.Owner != ""
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

// ClassesRepaired reports whether the one-time raise of lowered function classes has run.
func (s *State) ClassesRepaired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.ClassesRepaired
}

// MarkClassesRepaired records that the one-time raise of lowered function classes has run.
func (s *State) MarkClassesRepaired() error {
	return s.update(func(d *stateFile) error {
		if d.ClassesRepaired {
			return errUnchanged
		}
		d.ClassesRepaired = true
		return nil
	})
}

// RecordClassRaised records in the audit that the console raised a function of a connector back to the class its review or server declares.
func (s *State) RecordClassRaised(actor, connector, function, from, to string) error {
	return s.update(func(d *stateFile) error {
		record(d, actor, "function.class_raised", connector+"/"+function, map[string]string{"connector": connector, "function": function, "from": from, "to": to})
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

// SetCredentialMode records whose credential the connector's calls use; shared, the default, is not stored.
func (s *State) SetCredentialMode(conn string, mode CredentialMode) error {
	return s.update(func(d *stateFile) error {
		if mode == CredentialShared {
			delete(d.Credentials, conn)
			return nil
		}
		put(&d.Credentials, conn, mode)
		return nil
	})
}

// CredentialMode returns whose credential the connector's calls use, shared when nobody chose otherwise.
func (s *State) CredentialMode(conn string) CredentialMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cmp.Or(s.data.Credentials[conn], CredentialShared)
}

// ForgetConnector drops the new-key default, the new-key limits and the credential mode of a removed connector and its profile from every permission set, and a set left empty.
func (s *State) ForgetConnector(conn string) error {
	return s.update(func(d *stateFile) error {
		delete(d.NewKeysOff, conn)
		delete(d.NewKeyLimits, conn)
		delete(d.Credentials, conn)
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
