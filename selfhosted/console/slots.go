package console

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
)

// KeySlot is a named permission set a shared setup brought: a profile for each connector it installed, waiting until the owner gives it to an access key, never a credential.
type KeySlot struct {
	Name     string             `json:"name"`
	Profiles map[string]Profile `json:"profiles"`
}

// SlotProfile is what one permission set holds for one connector: its mode and overrides, with the connector's alias.
type SlotProfile struct {
	Slot      string            `json:"slot"`
	Connector string            `json:"connector"`
	Alias     string            `json:"alias"`
	Mode      ProfileMode       `json:"mode"`
	Overrides map[string]Access `json:"overrides,omitempty"`
}

// SlotView is one permission set as the console lists it, with its profile on each live connector.
type SlotView struct {
	Name       string        `json:"name"`
	Connectors []SlotProfile `json:"connectors"`
}

func migrateSlots(d *stateFile) {
	for conn, list := range d.Pending {
		for _, p := range list {
			slot := d.Slots[p.Agent]
			slot.Name = p.Agent
			put(&slot.Profiles, conn, p)
			put(&d.Slots, p.Agent, slot)
		}
	}
	d.Pending = nil
}

func freeSlotName(taken map[string]KeySlot, base string) string {
	name := base
	for i := 2; ; i++ {
		if _, ok := taken[name]; !ok {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// PutSlot keeps a permission set under its name, or the name with the first free number appended, and returns the name it took.
func (s *State) PutSlot(slot KeySlot) (string, error) {
	err := s.update(func(d *stateFile) error {
		slot.Name = freeSlotName(d.Slots, slot.Name)
		put(&d.Slots, slot.Name, slot)
		return nil
	})
	return slot.Name, err
}

// Slot returns the permission set of this name.
func (s *State) Slot(name string) (KeySlot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.data.Slots[name]
	slot.Profiles = maps.Clone(slot.Profiles)
	return slot, ok
}

// Slots returns every permission set waiting to be given to a key, by name.
func (s *State) Slots() []KeySlot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Collect(maps.Values(s.data.Slots))
	slices.SortFunc(out, func(a, b KeySlot) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// DropSlot forgets the permission set of this name.
func (s *State) DropSlot(name string) error {
	return s.update(func(d *stateFile) error {
		if _, ok := d.Slots[name]; !ok {
			return errUnchanged
		}
		delete(d.Slots, name)
		return nil
	})
}

func noSlot(name string) error {
	return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no permission set is named " + name}
}

// Slots lists the permission sets waiting to be given to a key, each with its profile on the connectors still installed.
func (c *Console) Slots(ctx context.Context) ([]SlotView, error) {
	live, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return nil, err
	}
	out := []SlotView{}
	for _, slot := range c.State.Slots() {
		v := SlotView{Name: slot.Name, Connectors: []SlotProfile{}}
		for _, conn := range live {
			if p, ok := slot.Profiles[string(conn.ID)]; ok {
				v.Connectors = append(v.Connectors, SlotProfile{Slot: slot.Name, Connector: string(conn.ID), Alias: conn.Alias, Mode: p.Mode, Overrides: p.Overrides})
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (c *Console) slotsOn(conn, alias string) []SlotProfile {
	out := []SlotProfile{}
	for _, slot := range c.State.Slots() {
		if p, ok := slot.Profiles[conn]; ok {
			out = append(out, SlotProfile{Slot: slot.Name, Connector: conn, Alias: alias, Mode: p.Mode, Overrides: p.Overrides})
		}
	}
	return out
}

// ClaimSlot gives an access key made already the permission set's profile on each of its connectors, within their new-key limits, and then forgets the set.
func (c *Console) ClaimSlot(ctx context.Context, name, key string) error {
	slot, ok := c.State.Slot(name)
	if !ok {
		return noSlot(name)
	}
	a, err := agentNamed(c.knownAgents(), key)
	if err != nil {
		return err
	}
	live, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return err
	}
	for _, conn := range live {
		if p, ok := slot.Profiles[string(conn.ID)]; ok {
			if err := c.claimOn(ctx, string(conn.ID), a, p); err != nil {
				return fmt.Errorf("connector %s: %w", conn.Alias, err)
			}
		}
	}
	return c.State.DropSlot(name)
}

func (c *Console) claimOn(ctx context.Context, conn string, a Agent, p Profile) error {
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return err
	}
	defer unlock()
	p.Agent = a.Name
	return c.applyProfile(ctx, conn, a, limited(p, c.State.NewKeyLimits(conn)))
}

// DiscardSlot forgets a permission set without giving it to any key.
func (c *Console) DiscardSlot(name string) error {
	if _, ok := c.State.Slot(name); !ok {
		return noSlot(name)
	}
	return c.State.DropSlot(name)
}

func (c *Console) listSlots(w http.ResponseWriter, r *http.Request) {
	slots, err := c.Slots(r.Context())
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

func (c *Console) claimSlot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.ClaimSlot(r.Context(), r.PathValue("name"), in.Key); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) discardSlot(w http.ResponseWriter, r *http.Request) {
	if err := c.DiscardSlot(r.PathValue("name")); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
