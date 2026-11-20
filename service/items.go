package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/yosida95/uritemplate/v3"

	"github.com/versatile-sh/connector-engine/core/names"
	"github.com/versatile-sh/connector-engine/port"
)

// ServedItem is one connector item the agent may use: a prompt under <alias>__<name>, a resource or template under <alias>+<uri>, or accepted instructions under the alias.
type ServedItem struct {
	Name       string
	Alias      string
	Connector  port.ConnectorID
	Item       string
	Definition json.RawMessage
	Mode       port.Mode
}

func (s *Service) relays(kind string) bool {
	switch kind {
	case port.ItemInstructions:
		return s.cfg.RelayInstructions
	case port.ItemPrompt, port.ItemResource:
		return s.cfg.UpstreamContent
	}
	return false
}

func (s *Service) itemDefinitions(ctx context.Context, h port.Handle, meta port.ListMeta) ([]port.RawDefinition, error) {
	var out []port.RawDefinition
	if content, ok := s.d.Runtime.(port.ContentRuntime); ok && s.cfg.UpstreamContent {
		defs, err := content.ListContent(ctx, h)
		if err != nil {
			return nil, err
		}
		out = defs
	}
	if s.cfg.RelayInstructions && meta.Instructions != "" {
		raw, _ := json.Marshal(map[string]string{"instructions": meta.Instructions})
		out = append(out, port.RawDefinition{Name: port.InstructionsItem, Raw: raw})
	}
	return out, nil
}

func (s *Service) itemName(alias string, f port.Function) (string, json.RawMessage, bool) {
	kind := port.ItemKind(f.Name)
	upstream := strings.TrimPrefix(f.Name, kind)
	var def map[string]json.RawMessage
	if json.Unmarshal(f.Definition, &def) != nil {
		return "", nil, false
	}
	var name string
	switch kind {
	case port.ItemInstructions:
		return alias, f.Definition, true
	case port.ItemPrompt:
		var err error
		if name, err = names.Exposed(alias, upstream, s.cfg.Separator, s.cfg.NameMax); err != nil {
			return "", nil, false
		}
		var desc string
		_ = json.Unmarshal(def["description"], &desc)
		def["name"], _ = json.Marshal(name)
		def["description"], _ = json.Marshal(strings.TrimSpace("From connector " + alias + ". " + desc))
	case port.ItemResource:
		var ok bool
		if name, ok = names.ResourceURI(alias, upstream); !ok {
			return "", nil, false
		}
		key := "uri"
		if _, template := def["uriTemplate"]; template {
			key = "uriTemplate"
		}
		def[key], _ = json.Marshal(name)
	default:
		return "", nil, false
	}
	out, _ := json.Marshal(def)
	return name, out, true
}

func (s *Service) serveItem(out *FunctionList, c port.Connector, f port.Function, mode port.Mode) {
	kind := port.ItemKind(f.Name)
	if !s.relays(kind) || kind == port.ItemInstructions && mode != port.ModeAllow {
		return
	}
	name, def, ok := s.itemName(c.Alias, f)
	if !ok {
		return
	}
	out.Items = append(out.Items, ServedItem{Name: name, Alias: c.Alias, Connector: c.ID, Item: f.Name, Definition: def, Mode: mode})
	out.ApprovalTools = out.ApprovalTools || mode == port.ModeAsk || mode == port.ModeAskOnce
}

func templateMatches(f port.Function, upstream string) bool {
	var def struct {
		Template string `json:"uriTemplate"`
	}
	if json.Unmarshal(f.Definition, &def) != nil || def.Template == "" {
		return false
	}
	t, err := uritemplate.New(def.Template)
	return err == nil && t.Regexp().MatchString(upstream)
}

func readArgs(uri string) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"uri": uri})
	return raw
}

func (s *Service) resolveItem(tx port.Tx, conns []port.Connector, kind, name string) (target, bool, error) {
	if !s.relays(kind) || kind == port.ItemInstructions {
		return target{}, false, nil
	}
	alias, upstream, _ := names.SplitResourceURI(name)
	for _, c := range conns {
		if kind == port.ItemResource && c.Alias != alias {
			continue
		}
		fns, err := tx.ListFunctions(c.ID)
		if err != nil {
			return target{}, false, err
		}
		var template *port.Function
		for i, f := range fns {
			switch {
			case port.ItemKind(f.Name) != kind:
			case kind == port.ItemPrompt:
				if n, _, ok := s.itemName(c.Alias, f); ok && n == name {
					return target{Conn: c, Fn: f}, true, nil
				}
			case f.Name == kind+upstream:
				return target{Conn: c, Fn: f, Args: readArgs(upstream)}, true, nil
			case template == nil && templateMatches(f, upstream):
				template = &fns[i]
			}
		}
		if template != nil {
			return target{Conn: c, Fn: *template, Args: readArgs(upstream)}, true, nil
		}
	}
	return target{}, false, nil
}

func prefixURIs(v any, alias string) bool {
	changed := false
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			changed = prefixURIs(e, alias) || changed
		}
	case map[string]any:
		for k, e := range x {
			if uri, ok := e.(string); ok && k == "uri" {
				if p, ok := names.ResourceURI(alias, uri); ok {
					x[k], changed = p, true
				}
				continue
			}
			changed = prefixURIs(e, alias) || changed
		}
	}
	return changed
}

func (s *Service) agentURIs(alias string, raw json.RawMessage) json.RawMessage {
	if !s.cfg.UpstreamContent || len(raw) == 0 {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil || !prefixURIs(v, alias) {
		return raw
	}
	if out, err := json.Marshal(v); err == nil {
		return out
	}
	return raw
}

// Complete answers completion/complete for a prompt or resource template the agent may use without approval; it opens no approval and writes no audit row, and found is false otherwise.
func (s *Service) Complete(ctx context.Context, p port.Principal, req CallRequest) (completion json.RawMessage, found bool, err error) {
	sub, err := s.agentSubject(ctx, p)
	if err != nil {
		return nil, false, err
	}
	t, found, err := s.resolve(ctx, sub, req)
	if err != nil || !found {
		return nil, false, err
	}
	dec, err := s.d.Authorizer.Evaluate(ctx, queryOf(sub, t, nil))
	if err != nil || dec.Mode != port.ModeAllow {
		return nil, false, err
	}
	var res resolution
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) (err error) {
		res, err = s.resolveCredential(tx, t.Conn, t.Bind, sub.OnBehalfOf, t.Fn.Name, t.Manifest)
		return err
	}); err != nil || res.Code != "" {
		return nil, false, err
	}
	var cred port.CredentialID
	var owner port.OwnerKind
	if res.Credential != nil {
		cred, owner = res.Credential.ID, res.Credential.OwnerKind
	}
	h, err := s.handle(ctx, t.Conn, t.Manifest, cred, owner)
	if err != nil {
		return nil, false, err
	}
	defer s.release(ctx, h)
	content, ok := s.d.Runtime.(port.ContentRuntime)
	if !ok {
		return nil, false, nil
	}
	out, err := content.Complete(ctx, h.h, t.Fn.Name, req.Arguments)
	return out, err == nil, err
}
