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
	"os"
	"path/filepath"
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

// Agent is an agent the owner connected, with the expiry of the last token minted for it.
type Agent struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StoredSession is a console session as the state file keeps it, keyed by the SHA-256 of its cookie value.
type StoredSession struct {
	CSRF    string    `json:"csrf"`
	Expires time.Time `json:"expires"`
}

type stateFile struct {
	Owner               *Owner                   `json:"owner,omitempty"`
	Agents              map[string]Agent         `json:"agents,omitempty"`
	Sessions            map[string]StoredSession `json:"sessions,omitempty"`
	Revoked             map[string]time.Time     `json:"revoked,omitempty"`
	OnboardingDismissed bool                     `json:"onboarding_dismissed,omitempty"`
	Clients             map[string]OAuthClient   `json:"oauth_clients,omitempty"`
	Refresh             map[string]RefreshGrant  `json:"oauth_refresh,omitempty"`
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
	return s, nil
}

func (s *State) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
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

// CreateOwner writes the owner account once; the password has at least ten characters.
func (s *State) CreateOwner(username, password string) error {
	if username == "" || len(username) > 64 {
		return fmt.Errorf("the username has 1 to 64 characters")
	}
	if len(password) < minPassword {
		return fmt.Errorf("the password has at least %d characters", minPassword)
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	hash, err := hashPassword(password, salt, pbkdf2Iterations)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Owner != nil {
		return ErrOwnerExists
	}
	s.data.Owner = &Owner{Username: username, Salt: base64.StdEncoding.EncodeToString(salt), Hash: base64.StdEncoding.EncodeToString(hash), Iterations: pbkdf2Iterations}
	if err := s.save(); err != nil {
		s.data.Owner = nil
		return err
	}
	return nil
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

// Sessions returns the stored sessions.
func (s *State) Sessions() map[string]StoredSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]StoredSession, len(s.data.Sessions))
	for k, v := range s.data.Sessions {
		out[k] = v
	}
	return out
}

// PutSessions replaces the stored sessions.
func (s *State) PutSessions(sessions map[string]StoredSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Sessions = sessions
	return s.save()
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

// PutAgent records an agent and the expiry of its newest token.
func (s *State) PutAgent(a Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Agents == nil {
		s.data.Agents = map[string]Agent{}
	}
	s.data.Agents[a.Name] = a
	return s.save()
}

// RevokeAgent forgets the agent and refuses every token minted for it up to at; it reports false when no such agent was ever connected.
func (s *State) RevokeAgent(name string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agent, known := s.data.Agents[name]
	before, revoked := s.data.Revoked[name]
	if !known && !revoked {
		return false, nil
	}
	if s.data.Revoked == nil {
		s.data.Revoked = map[string]time.Time{}
	}
	delete(s.data.Agents, name)
	s.data.Revoked[name] = at.UTC()
	if err := s.save(); err != nil {
		if known {
			s.data.Agents[name] = agent
		}
		if revoked {
			s.data.Revoked[name] = before
		} else {
			delete(s.data.Revoked, name)
		}
		return false, err
	}
	return true, nil
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.OnboardingDismissed {
		return nil
	}
	s.data.OnboardingDismissed = true
	if err := s.save(); err != nil {
		s.data.OnboardingDismissed = false
		return err
	}
	return nil
}
