package console

import (
	"maps"
	"slices"
	"time"
)

const maxOAuthClients = 500

// OAuthClient is an app registered by dynamic client registration: its public id, its self-asserted name and the redirect URIs kept after filtering.
type OAuthClient struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	RedirectURIs []string  `json:"redirect_uris"`
	LastUsed     time.Time `json:"last_used"`
}

// RefreshGrant is what a refresh token stands for, stored under the SHA-256 of the token.
type RefreshGrant struct {
	ClientID string    `json:"client_id"`
	Agent    string    `json:"agent"`
	Resource string    `json:"resource,omitempty"`
	Family   string    `json:"family,omitempty"`
	Issued   time.Time `json:"issued"`
	Expires  time.Time `json:"expires"`
}

// OAuthClient returns a registered client.
func (s *State) OAuthClient(id string) (OAuthClient, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Clients[id]
	return c, ok
}

// PutOAuthClient stores a client, dropping the least recently used ones beyond the cap so registrations cannot grow the file without bound.
func (s *State) PutOAuthClient(c OAuthClient) error {
	return s.update(func(d *stateFile) error {
		put(&d.Clients, c.ID, c)
		for len(d.Clients) > maxOAuthClients {
			oldest := slices.MinFunc(slices.Collect(maps.Values(d.Clients)), func(a, b OAuthClient) int { return a.LastUsed.Compare(b.LastUsed) })
			delete(d.Clients, oldest.ID)
		}
		return nil
	})
}

// Agent returns a connected agent by name.
func (s *State) Agent(name string) (Agent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.data.Agents[name]
	return a, ok
}

// SignedIn reports whether a live refresh grant ties the agent to the OAuth client.
func (s *State) SignedIn(agent, clientID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, g := range s.data.Refresh {
		if g.Agent == agent && g.ClientID == clientID && now.Before(g.Expires) {
			return true
		}
	}
	return false
}

// RefreshGrant returns the live grant stored under the hash of a refresh token.
func (s *State) RefreshGrant(hash string) (RefreshGrant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.data.Refresh[hash]
	if !ok || time.Now().After(g.Expires) {
		return RefreshGrant{}, false
	}
	return g, true
}

// RotateRefresh drops the grant under old, when given, remembering it as spent for its family, and stores g under next, pruning expired grants on the way.
func (s *State) RotateRefresh(old, next string, g RefreshGrant) error {
	return s.update(func(d *stateFile) error {
		now := time.Now()
		expire(d.Refresh, now, func(v RefreshGrant) time.Time { return v.Expires })
		expire(d.Spent, now, func(v RefreshGrant) time.Time { return v.Expires })
		if old != "" {
			delete(d.Refresh, old)
			put(&d.Spent, old, g)
		}
		put(&d.Refresh, next, g)
		return nil
	})
}

// RevokeReusedRefresh ends every grant of the family of a refresh token already spent, refuses the agent's tokens issued so far and marks it signed out, reporting whether hash was one.
func (s *State) RevokeReusedRefresh(hash string) (bool, error) {
	reused := false
	err := s.update(func(d *stateFile) error {
		spent, ok := d.Spent[hash]
		if reused = ok && spent.Family != ""; !reused {
			return errUnchanged
		}
		maps.DeleteFunc(d.Refresh, func(_ string, g RefreshGrant) bool { return g.Family == spent.Family })
		now := time.Now().UTC()
		put(&d.Revoked, spent.Agent, now)
		if a, ok := d.Agents[spent.Agent]; ok {
			a.SignedOutAt = now.Truncate(time.Second)
			d.Agents[spent.Agent] = a
		}
		return nil
	})
	return reused && err == nil, err
}
