package console

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	"time"
)

// StoredChannel is one configured approval channel as the state file keeps it, without its secrets, with the person it answers for.
type StoredChannel struct {
	ID        string            `json:"id"`
	Owner     string            `json:"owner,omitempty"`
	Kind      string            `json:"kind"`
	Label     string            `json:"label"`
	Settings  map[string]string `json:"settings"`
	CreatedAt time.Time         `json:"created_at"`
}

// Device is one paired host helper and the person it answers for; only the SHA-256 of its key is kept.
type Device struct {
	ID        string    `json:"id"`
	Owner     string    `json:"owner,omitempty"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"key_hash"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen,omitzero"`
}

type notifyState struct {
	Channels map[string]StoredChannel `json:"channels,omitempty"`
	Devices  map[string]Device        `json:"devices,omitempty"`
	Elicit   map[string]bool          `json:"elicit,omitempty"`
	Sealed   map[string]port.Sealed   `json:"channel_secrets,omitempty"`
	LinkKey  string                   `json:"link_key,omitempty"`
	Cursor   int64                    `json:"cursor,omitempty"`

	SealedLinkKey *port.Sealed `json:"sealed_link_key,omitempty"`
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
	return s.update(func(d *stateFile) error {
		put(&d.Notify.Channels, ch.ID, ch)
		return nil
	})
}

// DeleteChannel removes owner's approval channel and its secrets, reporting whether owner had it.
func (s *State) DeleteChannel(owner, id string) (bool, error) {
	found := false
	err := s.update(func(d *stateFile) error {
		if found = d.Notify.Channels[id].Owner == owner && owner != ""; !found {
			return errUnchanged
		}
		delete(d.Notify.Channels, id)
		delete(d.Notify.Sealed, id)
		return nil
	})
	return found && err == nil, err
}

// Devices returns the paired host helpers.
func (s *State) Devices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Values(s.data.Notify.Devices))
}

// Device returns the paired host helper with this id.
func (s *State) Device(id string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.data.Notify.Devices[id]
	return dev, ok && id != ""
}

func ownDevice(d *stateFile, owner, id string) (Device, bool) {
	dev, ok := d.Notify.Devices[id]
	return dev, ok && owner != "" && dev.Owner == owner
}

// PutDevice stores a paired host helper.
func (s *State) PutDevice(dev Device) error {
	return s.update(func(d *stateFile) error {
		put(&d.Notify.Devices, dev.ID, dev)
		return nil
	})
}

// RenameDevice names owner's paired host helper again, reporting whether owner had it.
func (s *State) RenameDevice(owner, id, name string) (bool, error) {
	found := false
	err := s.update(func(d *stateFile) error {
		dev, ok := ownDevice(d, owner, id)
		if found = ok; !ok {
			return errUnchanged
		}
		dev.Name = name
		d.Notify.Devices[id] = dev
		return nil
	})
	return found && err == nil, err
}

// DeleteDevice revokes owner's paired host helper, reporting whether owner had it.
func (s *State) DeleteDevice(owner, id string) (bool, error) {
	found := false
	err := s.update(func(d *stateFile) error {
		if _, found = ownDevice(d, owner, id); !found {
			return errUnchanged
		}
		delete(d.Notify.Devices, id)
		return nil
	})
	return found && err == nil, err
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
	return s.update(func(d *stateFile) error {
		if on {
			put(&d.Notify.Elicit, app, true)
		} else {
			delete(d.Notify.Elicit, app)
		}
		return nil
	})
}

func (s *State) linkKeyRecords() (*port.Sealed, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Notify.SealedLinkKey, s.data.Notify.LinkKey
}

func (s *State) putSealedLinkKey(v port.Sealed) error {
	return s.update(func(d *stateFile) error {
		d.Notify.SealedLinkKey, d.Notify.LinkKey = &v, ""
		return nil
	})
}

// Cursor returns the last approval event sequence the relay handled.
func (s *State) Cursor() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Notify.Cursor
}

// SetCursor records the last approval event sequence the relay handled.
func (s *State) SetCursor(seq int64) error {
	return s.update(func(d *stateFile) error {
		if seq <= d.Notify.Cursor {
			return errUnchanged
		}
		d.Notify.Cursor = seq
		return nil
	})
}

type channelSecrets struct {
	state *State
	box   *sealed.Box
}

func (s channelSecrets) get(ctx context.Context, channel string) (map[string]string, error) {
	s.state.mu.Lock()
	v, ok := s.state.data.Notify.Sealed[channel]
	s.state.mu.Unlock()
	out := map[string]string{}
	if !ok {
		return out, nil
	}
	if s.box == nil {
		return nil, sealed.ErrLocked
	}
	plain, err := s.box.Open(ctx, v)
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(plain), &out)
}

func (s channelSecrets) put(ctx context.Context, channel string, secrets map[string]string) error {
	if s.box == nil {
		return sealed.ErrLocked
	}
	raw, err := json.Marshal(secrets)
	if err != nil {
		return err
	}
	v, err := s.box.Seal(ctx, string(raw))
	if err != nil {
		return err
	}
	return s.state.update(func(d *stateFile) error {
		put(&d.Notify.Sealed, channel, v)
		return nil
	})
}
