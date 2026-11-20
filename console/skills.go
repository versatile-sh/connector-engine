package console

import (
	"context"
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

func (c *Console) skillFunction(ctx context.Context, conn string) (port.Function, error) {
	fns, err := c.Engine.functions(ctx, conn)
	if err != nil {
		return port.Function{}, err
	}
	return liveFunction(conn, fns)
}

func liveFunction(conn string, fns []port.Function) (port.Function, error) {
	if i := slices.IndexFunc(fns, func(f port.Function) bool { return f.State != port.FunctionRemoved }); i >= 0 {
		return fns[i], nil
	}
	return port.Function{}, &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "skill " + conn + " has no function"}
}

// InstallSkill installs a skill listing, accepts its pinned folder, opens it to the workspace and gives every current access key the owner's choice; later keys start off.
func (c *Console) InstallSkill(ctx context.Context, m port.Manifest, a Access) (string, error) {
	if m.Kind != port.KindSkill || m.Skill == nil {
		return "", invalid("the listing is not a skill")
	}
	conn, fns, err := c.installListing(ctx, m, func(port.ListingRef) string { return m.Skill.Name })
	if err != nil {
		return conn, err
	}
	f, err := liveFunction(conn, fns)
	if err != nil {
		return conn, err
	}
	if _, err := c.Engine.setClass(ctx, conn, f.Name, port.ClassRead); err != nil {
		return conn, err
	}
	g := port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: c.Engine.Workspace, Connector: port.ConnectorID(conn), Selector: port.SelectorFunction + f.Name, Mode: port.ModeAllow, DefinitionHash: f.DefinitionHash}
	if err := c.Engine.createGrant(ctx, g); err != nil {
		return conn, err
	}
	return conn, c.bindAll(ctx, conn, func(ctx context.Context, conn string, ag Agent) error { return c.bindSkill(ctx, conn, ag, a) })
}

func (c *Console) bindSkill(ctx context.Context, conn string, a Agent, access Access) error {
	if err := c.Engine.bind(ctx, conn, a.Name); err != nil {
		return err
	}
	return c.setKeyRow(ctx, conn, a, access)
}

func (c *Console) setKeyRow(ctx context.Context, conn string, a Agent, access Access) error {
	f, err := c.skillFunction(ctx, conn)
	if err != nil {
		return err
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, a.Name, conn)
	if err != nil {
		return err
	}
	if err := c.Engine.deleteGrants(ctx, rows); err != nil {
		return err
	}
	if access == AccessOff {
		return nil
	}
	return c.Engine.createGrant(ctx, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: a.Name, Agent: port.AgentID(a.Name), Connector: port.ConnectorID(conn), Selector: port.SelectorFunction + f.Name,
		Mode: access.mode(), DefinitionHash: f.DefinitionHash, ExpiresAt: a.ExpiresAt})
}

// SetSkillAccess writes the owner's choice for one access key on one skill: off removes its row, on allows, ask asks once per session.
func (c *Console) SetSkillAccess(ctx context.Context, conn, agent string, access Access) error {
	if !access.Valid() {
		return invalid("access %q is not off, on or ask", access)
	}
	agents := c.liveAgents()
	i := slices.IndexFunc(agents, func(a Agent) bool { return a.Name == agent })
	if i < 0 {
		return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no access key is named " + agent}
	}
	v, err := c.Engine.connector(ctx, conn)
	if err != nil {
		return err
	}
	if skill, err := c.isSkill(ctx, v); err != nil {
		return err
	} else if !skill || !v.RemovedAt.IsZero() {
		return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no skill is installed as " + conn}
	}
	return c.bindSkill(ctx, conn, agents[i], access)
}

// Skills lists the workspace's installed skills with each access key's choice and the required connectors not installed.
func (c *Console) Skills(ctx context.Context) ([]SkillSummary, error) {
	live, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, "", "")
	if err != nil {
		return nil, err
	}
	agents := c.liveAgents()
	installed := map[string]bool{}
	var skills []SkillSummary
	var requires [][]string
	for _, conn := range live {
		m, err := c.Engine.manifest(ctx, conn)
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
		for _, a := range agents {
			sum.Keys = append(sum.Keys, a.Name)
			sum.Access = append(sum.Access, KeyAccess{Agent: a.Name, Access: skillAccess(rows, string(conn.ID), a.Name)})
		}
		skills = append(skills, sum)
		requires = append(requires, s.Requires)
	}
	for i := range skills {
		skills[i].Unbound = port.Missing(requires[i], installed)
	}
	return skills, nil
}

func skillAccess(rows []port.Grant, conn, agent string) Access {
	return accessOf(rows, func(g port.Grant) bool {
		return string(g.Connector) == conn && g.ScopeID == agent && strings.HasPrefix(g.Selector, port.SelectorFunction)
	})
}

func (c *Console) keyAccess(ctx context.Context, conn, agent string) (Access, error) {
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, agent, conn)
	return skillAccess(rows, conn, agent), err
}

func (c *Console) rebindSkill(ctx context.Context, conn string, a Agent) error {
	access, err := c.keyAccess(ctx, conn, a.Name)
	if err != nil {
		return err
	}
	return c.bindSkill(ctx, conn, a, access)
}

func (c *Console) isSkill(ctx context.Context, conn port.Connector) (bool, error) {
	m, err := c.Engine.manifest(ctx, conn)
	return m.Kind == port.KindSkill, err
}
