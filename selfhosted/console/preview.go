package console

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/skillcatalog"
)

type importRequest struct {
	MCP          string            `json:"mcp,omitempty"`
	Listing      string            `json:"listing,omitempty"`
	Local        bool              `json:"local,omitempty"`
	HeaderSecret bool              `json:"header_secret,omitempty"`
	Choices      map[string]Access `json:"choices,omitempty"`
	Access       Access            `json:"access,omitempty"`
	Digest       string            `json:"digest,omitempty"`
	capped       bool
	keysOff      bool
}

// Preview is what the confirm screen shows before an import: where it comes from, the listing and each function's proposed choice.
type Preview struct {
	Kind         string        `json:"kind"`
	URL          string        `json:"url"`
	Host         string        `json:"host"`
	Name         string        `json:"name"`
	Title        string        `json:"title"`
	Description  string        `json:"description,omitempty"`
	Icon         string        `json:"icon,omitempty"`
	Publisher    string        `json:"publisher,omitempty"`
	Version      string        `json:"version"`
	RemoteURL    string        `json:"remote_url,omitempty"`
	WebsiteURL   string        `json:"website_url,omitempty"`
	Protocol     string        `json:"protocol,omitempty"`
	Auth         []string      `json:"auth,omitempty"`
	Tools        []ProbedTool  `json:"tools"`
	Items        []Item        `json:"items"`
	Warning      string        `json:"warning,omitempty"`
	HostLoopback bool          `json:"host_loopback,omitempty"`
	Skill        *SkillPreview `json:"skill,omitempty"`
	manifest     port.Manifest
	entry        skillcatalog.Entry
}

func (c *Console) buildPreview(ctx context.Context, in importRequest) (Preview, error) {
	switch {
	case in.MCP != "" && in.Listing == "":
		return previewMCP(ctx, strings.TrimSpace(in.MCP), in, Reach{Local: in.Local, LoopbackHost: c.LoopbackHost})
	case in.Listing != "" && in.MCP == "":
		src := strings.TrimSpace(in.Listing)
		body, err := c.get()(ctx, src, maxListingBytes)
		if err != nil {
			return Preview{}, err
		}
		u, _ := url.Parse(src)
		if skillcatalog.Kind(body) == string(port.KindSkill) {
			return c.previewSkill(src, u.Host, body)
		}
		return c.previewListing(ctx, src, u.Host, body)
	}
	return Preview{}, invalid("give exactly one of mcp or listing")
}

func previewMCP(ctx context.Context, raw string, in importRequest, reach Reach) (Preview, error) {
	p, err := Probe(ctx, raw, reach)
	if err != nil {
		return Preview{}, err
	}
	m, err := ManifestFor(p, in.HeaderSecret)
	if err != nil {
		return Preview{}, err
	}
	u, _ := url.Parse(p.URL)
	return Preview{Kind: "mcp", URL: p.URL, Host: u.Host, Name: m.Name, Title: m.Title, Description: m.Description, Icon: m.Icon, Publisher: m.Publisher.Name,
		Version: m.Version, RemoteURL: p.URL, WebsiteURL: p.WebsiteURL, Protocol: p.Protocol, Tools: nonNil(p.Tools), Items: p.Items, HostLoopback: reach.LoopbackHost != "" && egress.IsLoopback(u.Hostname()), manifest: m}, nil
}

func (c *Console) previewSkill(src, host string, body []byte) (Preview, error) {
	e, err := skillcatalog.Parse(body)
	if err != nil {
		return Preview{}, err
	}
	sp := c.skillPreview(e)
	return Preview{Kind: "skill", URL: src, Host: host, Name: e.ID, Title: firstOf(e.Title, e.Name), Description: e.Description, Publisher: skillcatalog.Publisher(e.Source.Repository, host),
		Version: e.Source.Commit, Tools: []ProbedTool{}, Items: []Item{}, Skill: &sp, entry: e}, nil
}

func (c *Console) previewListing(ctx context.Context, src, host string, body []byte) (Preview, error) {
	m, err := manifestFrom(src, body)
	if err != nil {
		return Preview{}, err
	}
	pv := Preview{Kind: "listing", URL: src, Host: host, Name: m.Name, Title: firstOf(m.Title, m.Name), Description: m.Description, Icon: m.Icon, Publisher: m.Publisher.Name, Version: m.Version, Tools: []ProbedTool{}, Items: []Item{}}
	for _, a := range m.Auth {
		pv.Auth = append(pv.Auth, string(a.Kind))
	}
	for _, f := range m.Functions {
		pv.Tools = append(pv.Tools, probedTool(f.Name, f.Title, f.Description, hintsOf(f.Annotations)))
	}
	if m.Remote != nil {
		if u, err := url.Parse(m.Remote.URL); err == nil && (egress.IsLoopback(u.Hostname()) || c.LoopbackHost != "" && strings.EqualFold(u.Hostname(), c.LoopbackHost)) {
			return Preview{}, invalid("the listing's server is on this computer; import it by its URL and tick \"local server\"")
		}
		pv.RemoteURL = m.Remote.URL
		if p, err := Probe(ctx, m.Remote.URL, Reach{}); err != nil {
			pv.Warning = "the server could not be reached to list its functions: " + err.Error()
		} else {
			pv.Tools, pv.Items, pv.Protocol, pv.WebsiteURL, pv.Icon = nonNil(p.Tools), p.Items, p.Protocol, p.WebsiteURL, firstOf(pv.Icon, p.Icon)
		}
	}
	m.Icon = pv.Icon
	pv.manifest = m
	return pv, nil
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

func (c *Console) importSkill(ctx context.Context, in importRequest, p Preview) (string, error) {
	access := in.Access
	if access == "" {
		access = AccessOn
	}
	if !access.Valid() {
		return "", invalid("access is off, on or ask")
	}
	if in.Digest != p.Skill.Digest {
		return "", &statusError{status: http.StatusConflict, err: errors.New("the skill changed since the preview; review it again")}
	}
	m, err := skillcatalog.Manifest(ctx, c.get(), p.URL, p.entry)
	if err != nil {
		return "", err
	}
	return c.InstallSkill(ctx, m, access)
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
	id, p, err := c.importOne(r.Context(), in)
	if err != nil {
		c.fail(w, err)
		return
	}
	kind := p.Kind
	if kind != "skill" {
		kind = "connector"
	}
	writeJSON(w, http.StatusCreated, map[string]string{kind: id})
}

func proposedChoices(p Preview) map[string]Access {
	out := map[string]Access{}
	for _, t := range p.Tools {
		out[t.Name] = t.Proposed
	}
	for _, it := range p.Items {
		out[it.Name] = it.Access
	}
	return out
}

func above(a, base Access) bool { return a.mode().Rank() > base.mode().Rank() }

func lowered(want, proposed map[string]Access) map[string]Access {
	var out map[string]Access
	for name, a := range want {
		if base, ok := proposed[name]; ok && above(a, base) {
			put(&out, name, base)
		}
	}
	return out
}

func (c *Console) importOne(ctx context.Context, in importRequest) (string, Preview, error) {
	for name, a := range in.Choices {
		if !a.Valid() {
			return "", Preview{}, invalid("function %s: access is off, on or ask", name)
		}
	}
	p, err := c.buildPreview(ctx, in)
	if err != nil {
		return "", Preview{}, err
	}
	var id string
	if p.Kind == "skill" {
		id, err = c.importSkill(ctx, in, p)
	} else {
		choices := proposedChoices(p)
		for name, a := range in.Choices {
			if base, ok := choices[name]; ok && !(in.capped && above(a, base)) {
				choices[name] = a
			}
		}
		id, err = c.Install(ctx, p.manifest, choices, !in.keysOff)
	}
	if err != nil {
		return "", Preview{}, err
	}
	o := Origin{MCP: strings.TrimSpace(in.MCP), Listing: strings.TrimSpace(in.Listing), Local: in.Local, HeaderSecret: in.HeaderSecret, Digest: in.Digest}
	return id, p, c.State.PutOrigin(id, o)
}
