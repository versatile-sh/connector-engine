package console

import (
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Clients == nil {
		s.data.Clients = map[string]OAuthClient{}
	}
	s.data.Clients[c.ID] = c
	for len(s.data.Clients) > maxOAuthClients {
		oldest := slices.MinFunc(mapValues(s.data.Clients), func(a, b OAuthClient) int { return a.LastUsed.Compare(b.LastUsed) })
		delete(s.data.Clients, oldest.ID)
	}
	return s.save()
}

func mapValues[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// Agent returns a connected agent by name.
func (s *State) Agent(name string) (Agent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.data.Agents[name]
	return a, ok
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

// RotateRefresh drops the grant under old, when given, and stores g under next, pruning expired grants on the way.
func (s *State) RotateRefresh(old, next string, g RefreshGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Refresh == nil {
		s.data.Refresh = map[string]RefreshGrant{}
	}
	now := time.Now()
	expire(s.data.Refresh, now, func(v RefreshGrant) time.Time { return v.Expires })
	delete(s.data.Refresh, old)
	s.data.Refresh[next] = g
	return s.save()
}
