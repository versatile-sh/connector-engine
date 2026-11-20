package console

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/skillcatalog"
)

// SkillFileView is one file of a skill as the console shows it.
type SkillFileView struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// SkillPreview is what the confirm screen shows of a skill before it is imported.
type SkillPreview struct {
	Name           string              `json:"name"`
	Category       string              `json:"category,omitempty"`
	Source         port.SkillSource    `json:"source"`
	Licence        port.SkillLicence   `json:"licence"`
	Redistribution port.Redistribution `json:"redistribution"`
	HasScripts     bool                `json:"has_scripts"`
	Digest         string              `json:"digest"`
	Files          []SkillFileView     `json:"files"`
	Keys           []string            `json:"keys"`
}

// KeyAccess is the owner's choice for one access key on one skill.
type KeyAccess struct {
	Agent  string `json:"agent"`
	Access Access `json:"access"`
}

// SkillSummary is one installed skill with its provenance, files and the choice of every access key.
type SkillSummary struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	Version     string `json:"version"`
	State       string `json:"state"`
	SkillPreview
	Unbound []string    `json:"unbound,omitempty"`
	Access  []KeyAccess `json:"access"`
}

func fileViews(files []port.SkillFile) []SkillFileView {
	out := make([]SkillFileView, len(files))
	for i, f := range files {
		out[i] = SkillFileView{Path: f.Path, Bytes: f.Bytes}
	}
	return out
}

func (c *Console) liveAgents() []Agent {
	var out []Agent
	for _, a := range c.State.Agents() {
		if a.ExpiresAt.After(time.Now()) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b Agent) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func (c *Console) skillPreview(e skillcatalog.Entry) SkillPreview {
	keys := []string{}
	for _, a := range c.liveAgents() {
		keys = append(keys, a.Name)
	}
	return SkillPreview{Name: e.Name, Category: e.Category, Source: port.SkillSource{Repository: e.Source.Repository, Path: e.Source.Path, Commit: e.Source.Commit, URL: e.Source.URL},
		Licence: port.SkillLicence{SPDX: e.License.SPDX, From: e.License.From, Redistributable: e.License.Redistributable}, Redistribution: e.Redistribution(),
		HasScripts: e.HasScriptsDeclaredOrSeen(),
		Digest:     e.Digest(), Files: fileViews(e.SkillFiles()), Keys: keys}
}

func (c *Console) manifestOf(ctx context.Context, conn port.Connector) (port.Manifest, error) {
	var lv port.ListingVersion
	ref := conn.Listing
	err := c.Engine.Call(ctx, http.MethodGet, "/v1/listings/version"+q("workspace", c.Engine.Workspace, "owner", string(ref.Listing.Owner), "name", ref.Listing.Name, "version", ref.Version), nil, &lv)
	return lv.Manifest, err
}

func (c *Console) skillFunction(ctx context.Context, conn string) (port.Function, error) {
	var fns items[port.Function]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(conn)+"/functions", nil, &fns); err != nil {
		return port.Function{}, err
	}
	for _, f := range fns.Items {
		if f.State != port.FunctionRemoved {
			return f, nil
		}
	}
	return port.Function{}, &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "skill " + conn + " has no function"}
}

// InstallSkill installs a skill listing, accepts its pinned folder, opens it to the workspace and gives every current access key the owner's choice; later keys start off.
func (c *Console) InstallSkill(ctx context.Context, m port.Manifest, a Access) (string, error) {
	if m.Kind != port.KindSkill || m.Skill == nil {
		return "", fmt.Errorf("the listing is not a skill")
	}
	lv, err := c.createListing(ctx, m)
	if err != nil {
		return "", err
	}
	ref := lv.Ref.Listing
	if err := c.openToWorkspace(ctx, string(ref.Owner)+"/"+ref.Name); err != nil {
		return "", err
	}
	alias, err := c.freeAlias(ctx, m.Skill.Name)
	if err != nil {
		return "", err
	}
	var view connectorView
	if err := c.Engine.Call(ctx, http.MethodPost, "/v1/connectors", map[string]any{"workspace": c.Engine.Workspace, "alias": alias, "listing": ref, "version": lv.Ref.Version}, &view); err != nil {
		return "", err
	}
	conn := string(view.Connector.ID)
	f, err := c.skillFunction(ctx, conn)
	if err != nil {
		return conn, err
	}
	if f.State == port.FunctionQuarantined {
		if err := c.Engine.Call(ctx, http.MethodPost, "/v1/connectors/"+esc(conn)+"/functions:accept", map[string]any{"seen": map[string]port.Hash{f.Name: f.DefinitionHash}}, nil); err != nil {
			return conn, err
		}
	}
	if err := c.Engine.Call(ctx, http.MethodPut, "/v1/connectors/"+esc(conn)+"/level", map[string]any{"level": port.LevelCustom}, nil); err != nil {
		return conn, err
	}
	if err := c.Engine.Call(ctx, http.MethodPut, "/v1/connectors/"+esc(conn)+"/functions/"+esc(f.Name)+"/class", map[string]any{"class": port.ClassRead}, nil); err != nil {
		return conn, err
	}
	g := port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: c.Engine.Workspace, Connector: port.ConnectorID(conn), Selector: port.SelectorFunction + f.Name, Mode: port.ModeAllow, DefinitionHash: f.DefinitionHash}
	if err := c.Engine.Call(ctx, http.MethodPost, "/v1/grants", g, nil); err != nil {
		return conn, err
	}
	for _, ag := range c.liveAgents() {
		if err := c.bindSkill(ctx, conn, ag, a); err != nil {
			return conn, err
		}
	}
	return conn, nil
}

func (c *Console) bindSkill(ctx context.Context, conn string, a Agent, access Access) error {
	if err := c.Engine.Call(ctx, http.MethodPut, "/v1/connectors/"+esc(conn)+"/bindings/"+esc(a.Name), map[string]any{"credential_policy": port.CredentialShared, "ask_requester": true}, nil); err != nil {
		return err
	}
	return c.setKeyRow(ctx, conn, a, access)
}

func (c *Console) setKeyRow(ctx context.Context, conn string, a Agent, access Access) error {
	f, err := c.skillFunction(ctx, conn)
	if err != nil {
		return err
	}
	var rows items[port.Grant]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/grants"+q("scope_kind", string(port.ScopeAgent), "scope_id", a.Name, "connector", conn), nil, &rows); err != nil {
		return err
	}
	for _, g := range rows.Items {
		if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/grants/"+esc(string(g.ID)), nil, nil); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	mode := map[Access]port.Mode{AccessOn: port.ModeAllow, AccessAsk: port.ModeAskOnce}[access]
	if mode == "" {
		return nil
	}
	g := port.Grant{ScopeKind: port.ScopeAgent, ScopeID: a.Name, Agent: port.AgentID(a.Name), Connector: port.ConnectorID(conn), Selector: port.SelectorFunction + f.Name,
		Mode: mode, DefinitionHash: f.DefinitionHash, ExpiresAt: a.ExpiresAt}
	return c.Engine.Call(ctx, http.MethodPost, "/v1/grants", g, nil)
}

// SetSkillAccess writes the owner's choice for one access key on one skill: off removes its row, on allows, ask asks once per session.
func (c *Console) SetSkillAccess(ctx context.Context, conn, agent string, access Access) error {
	if !access.Valid() {
		return fmt.Errorf("access %q is not off, on or ask", access)
	}
	agents := c.liveAgents()
	i := slices.IndexFunc(agents, func(a Agent) bool { return a.Name == agent })
	if i < 0 {
		return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no access key is named " + agent}
	}
	var v connectorView
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(conn), nil, &v); err != nil {
		return err
	}
	if skill, err := c.isSkill(ctx, v.Connector); err != nil {
		return err
	} else if !skill || !v.Connector.RemovedAt.IsZero() {
		return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no skill is installed as " + conn}
	}
	return c.bindSkill(ctx, conn, agents[i], access)
}

// Skills lists the workspace's installed skills with each access key's choice and the required connectors not installed.
func (c *Console) Skills(ctx context.Context) ([]SkillSummary, error) {
	var list items[connectorView]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors"+q("workspace", c.Engine.Workspace), nil, &list); err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	var skills []SkillSummary
	var requires [][]string
	for _, v := range list.Items {
		conn := v.Connector
		if !conn.RemovedAt.IsZero() {
			continue
		}
		m, err := c.manifestOf(ctx, conn)
		if err != nil {
			return nil, err
		}
		installed[m.Name] = true
		if m.Kind != port.KindSkill || m.Skill == nil {
			continue
		}
		s := m.Skill
		sum := SkillSummary{ID: string(conn.ID), Alias: conn.Alias, Title: firstOf(m.Title, s.Name), Description: m.Description, Publisher: skillcatalog.Publisher(s.Source.Repository, m.Publisher.Name), Version: conn.Listing.Version,
			SkillPreview: SkillPreview{Name: s.Name, Category: s.Category, Source: s.Source, Licence: s.Licence, Redistribution: s.Redistribution, HasScripts: s.HasScripts,
				Files: fileViews(s.Files), Keys: []string{}}, Access: []KeyAccess{}}
		if f, err := c.skillFunction(ctx, string(conn.ID)); err == nil {
			sum.State = string(f.State)
		}
		for _, a := range c.liveAgents() {
			sum.Keys = append(sum.Keys, a.Name)
			access, err := c.keyAccess(ctx, string(conn.ID), a.Name)
			if err != nil {
				return nil, err
			}
			sum.Access = append(sum.Access, KeyAccess{Agent: a.Name, Access: access})
		}
		skills = append(skills, sum)
		requires = append(requires, s.Requires)
	}
	for i := range skills {
		for _, r := range requires[i] {
			if !installed[r] {
				skills[i].Unbound = append(skills[i].Unbound, r)
			}
		}
	}
	return skills, nil
}

func (c *Console) keyAccess(ctx context.Context, conn, agent string) (Access, error) {
	var rows items[port.Grant]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/grants"+q("scope_kind", string(port.ScopeAgent), "scope_id", agent, "connector", conn), nil, &rows); err != nil {
		return AccessOff, err
	}
	for _, g := range rows.Items {
		if strings.HasPrefix(g.Selector, port.SelectorFunction) {
			switch g.Mode {
			case port.ModeAllow:
				return AccessOn, nil
			case port.ModeAsk, port.ModeAskOnce:
				return AccessAsk, nil
			}
		}
	}
	return AccessOff, nil
}

func (c *Console) rebindSkill(ctx context.Context, conn string, a Agent) error {
	access, err := c.keyAccess(ctx, conn, a.Name)
	if err != nil {
		return err
	}
	return c.bindSkill(ctx, conn, a, access)
}

func (c *Console) isSkill(ctx context.Context, conn port.Connector) (bool, error) {
	m, err := c.manifestOf(ctx, conn)
	return m.Kind == port.KindSkill, err
}
