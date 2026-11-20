package console

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	setupAPIVersion = "connector-engine/setup/v1"
	setupKind       = "Setup"
	setupExtension  = ".setup.yaml"
	maxSetupBytes   = 1 << 20
)

var tokenLike = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Setup is a shareable install: the connectors and skills to add with their choices, each access key's profiles as a permission set named after the key, and the kinds of notification channel, never a secret.
type Setup struct {
	APIVersion    string           `yaml:"apiVersion" json:"api_version"`
	Kind          string           `yaml:"kind" json:"kind"`
	Connectors    []SetupConnector `yaml:"connectors,omitempty" json:"connectors"`
	Skills        []SetupSkill     `yaml:"skills,omitempty" json:"skills"`
	Notifications []string         `yaml:"notifications,omitempty" json:"notifications"`
	Stripped      []string         `yaml:"stripped,omitempty" json:"stripped"`
}

// SetupConnector is one connector of a setup: its source, MCP address or listing address, the choice of each function and item, whether new keys start off and each access key's profile there, and in a review whether it is installed and the proposal of each choice this console would lower.
type SetupConnector struct {
	Name         string            `yaml:"name" json:"name"`
	MCP          string            `yaml:"mcp,omitempty" json:"mcp,omitempty"`
	Listing      string            `yaml:"listing,omitempty" json:"listing,omitempty"`
	Transport    string            `yaml:"transport,omitempty" json:"transport,omitempty"`
	Local        bool              `yaml:"local,omitempty" json:"local,omitempty"`
	HeaderSecret bool              `yaml:"headerSecret,omitempty" json:"header_secret,omitempty"`
	Functions    map[string]Access `yaml:"functions,omitempty" json:"functions"`
	Items        map[string]Access `yaml:"items,omitempty" json:"items"`
	NewKeysOff   bool              `yaml:"newKeysOff,omitempty" json:"new_keys_off,omitempty"`
	Profiles     []Profile         `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Exists       bool              `yaml:"-" json:"exists"`
	Above        map[string]Access `yaml:"-" json:"above,omitempty"`
}

// SetupOptions is what the owner chose in a setup's review: what becomes of each permission set by name, and whether the keys made already get the connectors it adds; the zero value keeps every set and leaves those keys off.
type SetupOptions struct {
	Slots        map[string]SlotChoice `json:"slots,omitempty"`
	ExistingKeys bool                  `json:"existing_keys,omitempty"`
}

// SlotChoice is the review's answer for one permission set: keep it to give to a key later, apply it to a key made already, or discard it.
type SlotChoice struct {
	Action SlotAction `json:"action"`
	Key    string     `json:"key,omitempty"`
}

// SlotAction is what a setup's import does with one permission set.
type SlotAction string

// The answers a review gives for a permission set.
const (
	SlotKeep    SlotAction = "keep"
	SlotApply   SlotAction = "apply"
	SlotDiscard SlotAction = "discard"
)

// SetupSkill is one skill of a setup: its catalog address, the digest it is pinned to and its default access.
type SetupSkill struct {
	Name    string `yaml:"name" json:"name"`
	Listing string `yaml:"listing" json:"listing"`
	Digest  string `yaml:"digest,omitempty" json:"digest,omitempty"`
	Access  Access `yaml:"access" json:"access"`
	Exists  bool   `yaml:"-" json:"exists"`
}

// SetupResult is what an import of a setup did: the entries it added, the ones it kept because they were installed, the ones that failed with why, the permission sets it kept to give to keys later, and by connector the choices it lowered to this console's proposal.
type SetupResult struct {
	Added   []string                     `json:"added"`
	Kept    []string                     `json:"kept"`
	Failed  map[string]string            `json:"failed"`
	SignIns []string                     `json:"sign_ins"`
	Slots   []string                     `json:"slots"`
	Capped  map[string]map[string]Access `json:"capped"`
}

func setupName(at time.Time) string {
	return "connector-engine-setup-" + at.UTC().Format("20060102T150405Z") + setupExtension
}

func secretSegment(seg string) bool {
	return len(seg) >= 20 && tokenLike.MatchString(seg) && strings.ContainsAny(seg, "0123456789")
}

func stripURL(raw string, stripped *[]string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if raw != "" {
			*stripped = append(*stripped, "left out an address that does not parse")
		}
		return ""
	}
	site := u.Scheme + "://" + u.Host
	if slices.ContainsFunc(strings.Split(u.Path, "/"), secretSegment) {
		*stripped = append(*stripped, fmt.Sprintf("left out an address of %s: its path looks like it carries a secret", site))
		return ""
	}
	for _, part := range []struct {
		what    string
		present bool
	}{{"the user name and password", u.User != nil}, {"the query", u.RawQuery != ""}, {"the fragment", u.Fragment != ""}} {
		if part.present {
			*stripped = append(*stripped, fmt.Sprintf("removed %s from an address of %s", part.what, site))
		}
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
	return u.String()
}

func commonAccess(rows []KeyAccess) Access {
	if len(rows) == 0 {
		return AccessOn
	}
	for _, r := range rows[1:] {
		if r.Access != rows[0].Access {
			return AccessAsk
		}
	}
	return rows[0].Access
}

// ExportSetup writes the shareable setup of this install as YAML, with every secret left out and each removal listed.
func (c *Console) ExportSetup(ctx context.Context) ([]byte, string, error) {
	connectors, err := c.Connectors(ctx)
	if err != nil {
		return nil, "", err
	}
	skills, err := c.Skills(ctx)
	if err != nil {
		return nil, "", err
	}
	s := Setup{APIVersion: setupAPIVersion, Kind: setupKind}
	for _, cs := range connectors {
		o, _ := c.State.OriginOf(cs.ID)
		sc := SetupConnector{Name: cs.Alias, Transport: cs.Transport, Local: o.Local, HeaderSecret: o.HeaderSecret, Functions: map[string]Access{}, Items: map[string]Access{}, NewKeysOff: cs.NewKeysOff}
		for _, p := range cs.Profiles {
			sc.Profiles = append(sc.Profiles, Profile{Agent: p.Agent, Mode: p.Mode, Overrides: p.Overrides})
		}
		switch {
		case o.Listing != "":
			sc.Listing = stripURL(o.Listing, &s.Stripped)
		default:
			sc.MCP = stripURL(firstOf(o.MCP, cs.RemoteURL), &s.Stripped)
		}
		if sc.MCP == "" && sc.Listing == "" {
			s.Stripped = append(s.Stripped, fmt.Sprintf("left out %s: it has no address to import it from", cs.Alias))
			continue
		}
		for _, f := range cs.Functions {
			sc.Functions[f.Name] = f.Access
		}
		for _, it := range cs.Items {
			sc.Items[it.Name] = it.Access
		}
		s.Connectors = append(s.Connectors, sc)
	}
	for _, sk := range skills {
		o, ok := c.State.OriginOf(sk.ID)
		if !ok || o.Listing == "" {
			s.Stripped = append(s.Stripped, fmt.Sprintf("left out the skill %s: it was imported before the console recorded catalog addresses", sk.Name))
			continue
		}
		s.Skills = append(s.Skills, SetupSkill{Name: sk.Name, Listing: stripURL(o.Listing, &s.Stripped), Digest: o.Digest, Access: commonAccess(sk.Access)})
	}
	for _, ch := range c.State.Channels() {
		if !slices.Contains(s.Notifications, ch.Kind) {
			s.Notifications = append(s.Notifications, ch.Kind)
		}
	}
	slices.Sort(s.Notifications)
	var out bytes.Buffer
	out.WriteString("# A connector-engine setup: what to install and how, without any credential. Import it from the console or with make import FILE=...\n")
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(s); err != nil {
		return nil, "", err
	}
	return out.Bytes(), setupName(time.Now()), nil
}

func parseSetup(raw []byte) (Setup, error) {
	if len(raw) > maxSetupBytes {
		return Setup{}, invalid("a setup file is at most %d bytes", maxSetupBytes)
	}
	var s Setup
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return Setup{}, invalid("this is neither a backup nor a setup file: %v", err)
	}
	if s.APIVersion != setupAPIVersion || s.Kind != setupKind {
		return Setup{}, invalid("this setup file is %s %s; this console reads %s %s", s.APIVersion, s.Kind, setupAPIVersion, setupKind)
	}
	for _, sc := range s.Connectors {
		if (sc.MCP == "") == (sc.Listing == "") {
			return Setup{}, invalid("connector %s gives exactly one of mcp or listing", sc.Name)
		}
		for name, a := range mergeChoices(sc) {
			if !validChoice(name, a) {
				return Setup{}, invalid("connector %s, %s: access is off, on or ask, and instructions are off or on", sc.Name, name)
			}
		}
		for _, p := range sc.Profiles {
			if Slug(p.Agent) == "" || !slices.Contains([]ProfileMode{ProfileOff, ProfileFollow, ProfileCustom}, p.Mode) {
				return Setup{}, invalid("connector %s, key %q: a profile names a key and is off, follow or custom", sc.Name, p.Agent)
			}
			for name, a := range p.Overrides {
				if !validChoice(name, a) {
					return Setup{}, invalid("connector %s, key %s, %s: access is off, on or ask, and instructions are off or on", sc.Name, p.Agent, name)
				}
			}
		}
	}
	for _, sk := range s.Skills {
		if sk.Listing == "" || (sk.Access != "" && !sk.Access.Valid()) {
			return Setup{}, invalid("skill %s needs a listing address and an access of off, on or ask", sk.Name)
		}
	}
	return s, nil
}

func mergeChoices(sc SetupConnector) map[string]Access {
	out := map[string]Access{}
	for k, v := range sc.Functions {
		out[k] = v
	}
	for k, v := range sc.Items {
		out[k] = v
	}
	return out
}

func (c *Console) markInstalled(ctx context.Context, s *Setup) error {
	connectors, err := c.Connectors(ctx)
	if err != nil {
		return err
	}
	skills, err := c.Skills(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, cs := range connectors {
		o, _ := c.State.OriginOf(cs.ID)
		for _, src := range []string{o.MCP, o.Listing, cs.RemoteURL} {
			have[src] = src != ""
		}
	}
	for _, sk := range skills {
		have["skill:"+sk.Name] = true
		if o, ok := c.State.OriginOf(sk.ID); ok && o.Listing != "" {
			have[o.Listing] = true
		}
	}
	for i, sc := range s.Connectors {
		s.Connectors[i].Exists = have[sc.MCP] || have[sc.Listing]
	}
	for i, sk := range s.Skills {
		s.Skills[i].Exists = have[sk.Listing] || have["skill:"+sk.Name]
	}
	return nil
}

func (c *Console) inspectSetup(ctx context.Context, raw []byte) (BackupSummary, error) {
	s, err := parseSetup(raw)
	if err != nil {
		return BackupSummary{}, err
	}
	if err := c.markInstalled(ctx, &s); err != nil {
		return BackupSummary{}, err
	}
	for i, sc := range s.Connectors {
		if sc.Exists || sc.Local {
			continue
		}
		p, err := c.buildPreview(ctx, importRequest{MCP: sc.MCP, Listing: sc.Listing, HeaderSecret: sc.HeaderSecret})
		if err != nil {
			continue
		}
		s.Connectors[i].Above = lowered(mergeChoices(sc), proposedChoices(p))
	}
	return BackupSummary{Kind: "setup", Counts: map[string]int{"connectors": len(s.Connectors), "skills": len(s.Skills)}, Setup: &s}, nil
}

func (c *Console) checkOptions(o SetupOptions) error {
	for name, ch := range o.Slots {
		switch ch.Action {
		case SlotKeep, SlotDiscard:
		case SlotApply:
			if _, err := agentNamed(c.knownAgents(), ch.Key); err != nil {
				return err
			}
		default:
			return invalid("permission set %s: the answer is keep, apply or discard", name)
		}
	}
	return nil
}

// ImportSetup adds what a setup holds through the console's own import, so pinning and the probe still apply, with each choice capped at what this console proposes and the keys made already left off its connectors unless the options opt in; an entry already installed keeps its choices, and each key's profiles on the connectors it added become a permission set the options keep, apply to a key made already or discard, never bound by name.
func (c *Console) ImportSetup(ctx context.Context, raw []byte, opts SetupOptions) (SetupResult, error) {
	s, err := parseSetup(raw)
	if err != nil {
		return SetupResult{}, err
	}
	if err := c.checkOptions(opts); err != nil {
		return SetupResult{}, err
	}
	if err := c.markInstalled(ctx, &s); err != nil {
		return SetupResult{}, err
	}
	res := SetupResult{Added: []string{}, Kept: []string{}, Failed: map[string]string{}, SignIns: []string{}, Slots: []string{}, Capped: map[string]map[string]Access{}}
	add := func(name string, in importRequest) (string, bool) {
		id, p, err := c.importOne(ctx, in)
		if err != nil {
			res.Failed[name] = err.Error()
		}
		if low := lowered(in.Choices, proposedChoices(p)); in.capped && len(low) > 0 {
			res.Capped[name] = low
		}
		return id, err == nil
	}
	slots := map[string]KeySlot{}
	for _, sc := range s.Connectors {
		if sc.Exists {
			res.Kept = append(res.Kept, sc.Name)
			continue
		}
		id, ok := add(sc.Name, importRequest{MCP: sc.MCP, Listing: sc.Listing, Local: sc.Local, HeaderSecret: sc.HeaderSecret, Choices: mergeChoices(sc), capped: true, keysOff: !opts.ExistingKeys})
		if !ok {
			continue
		}
		res.SignIns = append(res.SignIns, sc.Name)
		if err := c.State.SetNewKeysOff(id, sc.NewKeysOff); err != nil {
			res.Failed[sc.Name] = "installed without its new-key default: " + err.Error()
			continue
		}
		res.Added = append(res.Added, sc.Name)
		for _, p := range sc.Profiles {
			name := Slug(p.Agent)
			slot := slots[name]
			slot.Name = name
			put(&slot.Profiles, id, Profile{Mode: p.Mode, Overrides: p.Overrides})
			slots[name] = slot
		}
	}
	for _, name := range slices.Sorted(maps.Keys(slots)) {
		ch := opts.Slots[name]
		switch {
		case ch.Action == SlotDiscard:
			continue
		case ch.Action == SlotApply:
			err := c.applySlot(ctx, slots[name], ch.Key)
			if err == nil {
				continue
			}
			res.Failed["permission set "+name] = fmt.Sprintf("not applied to %s, kept to apply again: %v", ch.Key, err)
		}
		kept, err := c.State.PutSlot(slots[name])
		if err != nil {
			res.Failed["permission set "+name] = "not kept: " + err.Error()
			continue
		}
		res.Slots = append(res.Slots, kept)
	}
	for _, sk := range s.Skills {
		if sk.Exists {
			res.Kept = append(res.Kept, sk.Name)
			continue
		}
		if _, ok := add(sk.Name, importRequest{Listing: sk.Listing, Access: sk.Access, Digest: sk.Digest}); ok {
			res.Added = append(res.Added, sk.Name)
		}
	}
	return res, nil
}

func validChoice(name string, a Access) bool {
	kind, item := itemKinds[port.ItemKind(name)]
	return a.Valid() && (!item || validFor(kind, a))
}

func (c *Console) applySlot(ctx context.Context, slot KeySlot, key string) error {
	a, err := agentNamed(c.knownAgents(), key)
	if err != nil {
		return err
	}
	for conn, p := range slot.Profiles {
		if err := c.claimOn(ctx, conn, a, p); err != nil {
			return err
		}
	}
	return nil
}
