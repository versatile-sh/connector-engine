package console

import (
	"crypto/rand"
	"encoding/hex"
	"maps"
	"slices"
	"time"
)

// StoredChannel is one configured approval channel as the state file keeps it, secrets included.
type StoredChannel struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Label     string            `json:"label"`
	Settings  map[string]string `json:"settings"`
	CreatedAt time.Time         `json:"created_at"`
}

// Device is one paired host helper; only the SHA-256 of its key is kept.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"key_hash"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen,omitzero"`
}

type notifyState struct {
	Channels map[string]StoredChannel `json:"channels,omitempty"`
	Devices  map[string]Device        `json:"devices,omitempty"`
	Elicit   map[string]bool          `json:"elicit,omitempty"`
	LinkKey  string                   `json:"link_key,omitempty"`
	Cursor   int64                    `json:"cursor,omitempty"`
}

// Channels returns a copy of the configured approval channels.
func (s *State) Channels() []StoredChannel {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StoredChannel, 0, len(s.data.Notify.Channels))
	for _, ch := range s.data.Notify.Channels {
		ch.Settings = maps.Clone(ch.Settings)
		out = append(out, ch)
	}
	return out
}

// PutChannel stores an approval channel.
func (s *State) PutChannel(ch StoredChannel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Notify.Channels == nil {
		s.data.Notify.Channels = map[string]StoredChannel{}
	}
	s.data.Notify.Channels[ch.ID] = ch
	return s.save()
}

// DeleteChannel removes an approval channel and its secrets, reporting whether it existed.
func (s *State) DeleteChannel(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Notify.Channels[id]; !ok {
		return false, nil
	}
	delete(s.data.Notify.Channels, id)
	return true, s.save()
}

// Devices returns the paired host helpers.
func (s *State) Devices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Values(s.data.Notify.Devices))
}

// PutDevice stores a paired host helper.
func (s *State) PutDevice(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Notify.Devices == nil {
		s.data.Notify.Devices = map[string]Device{}
	}
	s.data.Notify.Devices[d.ID] = d
	return s.save()
}

// DeleteDevice revokes a paired host helper, reporting whether it existed.
func (s *State) DeleteDevice(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Notify.Devices[id]; !ok {
		return false, nil
	}
	delete(s.data.Notify.Devices, id)
	return true, s.save()
}

// DeviceByKey returns the device whose key hashes to keyHash.
func (s *State) DeviceByKey(keyHash string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.data.Notify.Devices {
		if csrfOK(d.KeyHash, keyHash) {
			return d, true
		}
	}
	return Device{}, false
}

// ElicitApps returns the apps the owner lets ask in their own window.
func (s *State) ElicitApps() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.data.Notify.Elicit)
}

// Elicits reports whether the app may ask in its own window.
func (s *State) Elicits(app string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Notify.Elicit[app]
}

// SetElicit turns asking in the app's own window on or off.
func (s *State) SetElicit(app string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		if s.data.Notify.Elicit == nil {
			s.data.Notify.Elicit = map[string]bool{}
		}
		s.data.Notify.Elicit[app] = true
	} else {
		delete(s.data.Notify.Elicit, app)
	}
	return s.save()
}

// LinkKey returns the key that signs approval links and request states, made on first use.
func (s *State) LinkKey() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Notify.LinkKey == "" {
		b := make([]byte, 32)
		rand.Read(b)
		s.data.Notify.LinkKey = hex.EncodeToString(b)
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return hex.DecodeString(s.data.Notify.LinkKey)
}

// Cursor returns the last approval event sequence the relay handled.
func (s *State) Cursor() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Notify.Cursor
}

// SetCursor records the last approval event sequence the relay handled.
func (s *State) SetCursor(seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq <= s.data.Notify.Cursor {
		return nil
	}
	s.data.Notify.Cursor = seq
	return s.save()
}
