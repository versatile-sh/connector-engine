package console

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/skillcatalog"
)

type importRequest struct {
	MCP          string            `json:"mcp,omitempty"`
	Listing      string            `json:"listing,omitempty"`
	Local        bool              `json:"local,omitempty"`
	HeaderSecret bool              `json:"header_secret,omitempty"`
	Choices      map[string]Access `json:"choices,omitempty"`
	Access       Access            `json:"access,omitempty"`
	Digest       string            `json:"digest,omitempty"`
}

// Preview is what the confirm screen shows before an import: where it comes from, the listing and each function's proposed choice.
type Preview struct {
	Kind        string        `json:"kind"`
	URL         string        `json:"url"`
	Host        string        `json:"host"`
	Name        string        `json:"name"`
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Icon        string        `json:"icon,omitempty"`
	Publisher   string        `json:"publisher,omitempty"`
	Version     string        `json:"version"`
	RemoteURL   string        `json:"remote_url,omitempty"`
	WebsiteURL  string        `json:"website_url,omitempty"`
	Protocol    string        `json:"protocol,omitempty"`
	Auth        []string      `json:"auth,omitempty"`
	Tools       []ProbedTool  `json:"tools"`
	Items       []Item        `json:"items"`
	Warning     string        `json:"warning,omitempty"`
	Skill       *SkillPreview `json:"skill,omitempty"`
	manifest    port.Manifest
	entry       skillcatalog.Entry
}

func (c *Console) buildPreview(ctx context.Context, in importRequest) (Preview, error) {
	switch {
	case in.MCP != "" && in.Listing == "":
		p, err := Probe(ctx, strings.TrimSpace(in.MCP), Reach{Local: in.Local})
		if err != nil {
			return Preview{}, err
		}
		m, err := ManifestFor(p, in.HeaderSecret)
		if err != nil {
			return Preview{}, err
		}
		u, _ := url.Parse(p.URL)
		return Preview{Kind: "mcp", URL: p.URL, Host: u.Host, Name: m.Name, Title: m.Title, Description: m.Description, Icon: m.Icon, Publisher: m.Publisher.Name,
			Version: m.Version, RemoteURL: p.URL, WebsiteURL: p.WebsiteURL, Protocol: p.Protocol, Tools: nonNil(p.Tools), Items: p.Items, manifest: m}, nil
	case in.Listing != "" && in.MCP == "":
		src := strings.TrimSpace(in.Listing)
		body, err := c.get()(ctx, src, maxListingBytes)
		if err != nil {
			return Preview{}, err
		}
		u, _ := url.Parse(src)
		if skillcatalog.Kind(body) == string(port.KindSkill) {
			e, err := skillcatalog.Parse(body)
			if err != nil {
				return Preview{}, err
			}
			sp := c.skillPreview(e)
			return Preview{Kind: "skill", URL: src, Host: u.Host, Name: e.ID, Title: firstOf(e.Title, e.Name), Description: e.Description, Publisher: skillcatalog.Publisher(e.Source.Repository, u.Host),
				Version: e.Source.Commit, Tools: []ProbedTool{}, Items: []Item{}, Skill: &sp, entry: e}, nil
		}
		m, err := manifestFrom(src, body)
		if err != nil {
			return Preview{}, err
		}
		pv := Preview{Kind: "listing", URL: src, Host: u.Host, Name: m.Name, Title: firstOf(m.Title, m.Name), Description: m.Description, Icon: m.Icon, Publisher: m.Publisher.Name, Version: m.Version, Items: []Item{}}
		for _, a := range m.Auth {
			pv.Auth = append(pv.Auth, string(a.Kind))
		}
		for _, f := range m.Functions {
			ro := f.Annotations != nil && f.Annotations.ReadOnlyHint != nil && *f.Annotations.ReadOnlyHint
			de := !ro && (f.Annotations == nil || f.Annotations.DestructiveHint == nil || *f.Annotations.DestructiveHint)
			prop := AccessAsk
			if ro {
				prop = AccessOn
			}
			pv.Tools = append(pv.Tools, ProbedTool{Name: f.Name, Title: f.Title, Description: f.Description, ReadOnly: ro, Destructive: de, Kind: KindOf(ro, de), Proposed: prop})
		}
		if m.Remote != nil {
			pv.RemoteURL = m.Remote.URL
			p, err := Probe(ctx, m.Remote.URL, Reach{})
			if err != nil {
				pv.Warning = "the server could not be reached to list its functions: " + err.Error()
			} else {
				pv.Tools, pv.Items, pv.Protocol, pv.WebsiteURL = nonNil(p.Tools), p.Items, p.Protocol, p.WebsiteURL
				if pv.Icon == "" {
					pv.Icon = p.Icon
				}
			}
		}
		m.Icon = pv.Icon
		pv.Tools = nonNil(pv.Tools)
		pv.manifest = m
		return pv, nil
	}
	return Preview{}, invalid("give exactly one of mcp or listing")
}

func (c *Console) get() skillcatalog.Get {
	if c.Fetch != nil {
		return c.Fetch
	}
	return func(ctx context.Context, raw string, limit int64) ([]byte, error) {
		body, _, err := Reach{}.Get(ctx, raw, limit, "application/yaml, application/json, text/plain, */*")
		return body, err
	}
}

func (c *Console) importSkill(w http.ResponseWriter, r *http.Request, in importRequest, p Preview) {
	access := in.Access
	if access == "" {
		access = AccessOn
	}
	if !access.Valid() {
		writeError(w, http.StatusBadRequest, "access is off, on or ask")
		return
	}
	if in.Digest != p.Skill.Digest {
		writeError(w, http.StatusConflict, "the skill changed since the preview; review it again")
		return
	}
	m, err := skillcatalog.Manifest(r.Context(), c.get(), p.URL, p.entry)
	if err != nil {
		c.fail(w, err)
		return
	}
	id, err := c.InstallSkill(r.Context(), m, access)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"skill": id})
}

func (c *Console) preview(w http.ResponseWriter, r *http.Request) {
	var in importRequest
	if !readJSON(w, r, &in) {
		return
	}
	p, err := c.buildPreview(r.Context(), in)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (c *Console) doImport(w http.ResponseWriter, r *http.Request) {
	var in importRequest
	if !readJSON(w, r, &in) {
		return
	}
	for name, a := range in.Choices {
		if !a.Valid() {
			writeError(w, http.StatusBadRequest, "function "+name+": access is off, on or ask")
			return
		}
	}
	p, err := c.buildPreview(r.Context(), in)
	if err != nil {
		c.fail(w, err)
		return
	}
	if p.Kind == "skill" {
		c.importSkill(w, r, in, p)
		return
	}
	choices := map[string]Access{}
	for _, t := range p.Tools {
		choices[t.Name] = t.Proposed
		if a, ok := in.Choices[t.Name]; ok {
			choices[t.Name] = a
		}
	}
	for _, it := range p.Items {
		choices[it.Name] = it.Access
		if a, ok := in.Choices[it.Name]; ok {
			choices[it.Name] = a
		}
	}
	id, err := c.Install(r.Context(), p.manifest, choices)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"connector": id})
}
