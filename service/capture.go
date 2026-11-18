package service

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/port"
)

type definitionView struct {
	Name        string            `json:"name"`
	Annotations *port.Annotations `json:"annotations"`
	Meta        struct {
		UI struct {
			ResourceURI string   `json:"resourceUri"`
			Visibility  []string `json:"visibility"`
		} `json:"ui"`
	} `json:"_meta"`
}

func viewOf(raw json.RawMessage) definitionView {
	var v definitionView
	_ = json.Unmarshal(raw, &v)
	return v
}

func (s *Service) handleKey(c port.Connector, version string, cred port.CredentialID) string {
	return strings.Join([]string{string(c.Tenant), string(c.ID), version, string(cred)}, "|")
}

func (s *Service) handle(ctx context.Context, c port.Connector, m port.Manifest, cred port.CredentialID, owner port.OwnerKind) (port.Handle, error) {
	key := s.handleKey(c, m.Version, cred)
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.handles[key]; ok {
		return h, nil
	}
	h, err := s.d.Runtime.Start(ctx, c, m, port.SecretsRef{Credential: cred, Owner: owner})
	if err != nil {
		return "", port.Errorf(port.CodeUnavailable, "connector %s did not start: %v", c.ID, err)
	}
	s.handles[key] = h
	return h, nil
}

func (s *Service) dropHandles(ctx context.Context, match func(key string) bool) {
	s.mu.Lock()
	var stop []port.Handle
	for k, h := range s.handles {
		if match(k) {
			stop = append(stop, h)
			delete(s.handles, k)
		}
	}
	s.mu.Unlock()
	for _, h := range stop {
		_ = s.d.Runtime.Stop(ctx, h)
	}
}

func (s *Service) stopHandles(ctx context.Context, tenant port.TenantID) {
	s.dropHandles(ctx, func(k string) bool { return strings.HasPrefix(k, string(tenant)+"|") })
}

func (s *Service) stopConnector(ctx context.Context, c port.Connector) {
	prefix := strings.Join([]string{string(c.Tenant), string(c.ID)}, "|") + "|"
	s.dropHandles(ctx, func(k string) bool { return strings.HasPrefix(k, prefix) })
}

func (s *Service) functionOf(c port.Connector, m port.Manifest, def port.RawDefinition, meta port.ListMeta) (port.Function, error) {
	hash, err := pinning.DefinitionHash(def.Raw)
	if err != nil {
		return port.Function{}, err
	}
	v := viewOf(def.Raw)
	f := port.Function{Tenant: c.Tenant, Connector: c.ID, Name: def.Name, Definition: slices.Clone(def.Raw), DefinitionHash: hash, Visibility: v.Meta.UI.Visibility,
		OpenWorld: v.Annotations == nil || v.Annotations.OpenWorldHint == nil || *v.Annotations.OpenWorldHint}
	if content, ok := meta.UIResources[v.Meta.UI.ResourceURI]; ok {
		f.UIResourceHash = pinning.UIResourceHash(content)
	}
	if i := slices.IndexFunc(m.Functions, func(d port.FunctionDef) bool { return d.Name == def.Name }); i >= 0 {
		f.ReadsPrivate, f.ReturnsThirdParty = m.Functions[i].ReadsPrivate, m.Functions[i].ReturnsThirdParty
	}
	f.Class, f.ClassSource = classOf(f, m)
	return f, nil
}

func classOf(f port.Function, m port.Manifest) (port.Class, port.ClassSource) {
	if f.ClassSource == port.ClassSourceAdmin && f.Class != port.ClassNone {
		return f.Class, port.ClassSourceAdmin
	}
	in := listing.ClassInput{Hash: f.DefinitionHash, Review: m.Review, Annotations: viewOf(f.Definition).Annotations}
	if i := slices.IndexFunc(m.Functions, func(d port.FunctionDef) bool { return d.Name == f.Name }); i >= 0 {
		in.ReviewedClass = m.Functions[i].Class
	}
	class, source, _ := listing.Class(in)
	return class, source
}

func (s *Service) captureFunctions(ctx context.Context, c port.Connector, m port.Manifest) ([]port.Function, port.ListMeta, error) {
	h, err := s.handle(ctx, c, m, "", "")
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	defs, meta, err := s.d.Runtime.ListFunctions(ctx, h)
	if err != nil {
		return nil, meta, port.Errorf(port.CodeUnavailable, "connector %s did not list its functions: %v", c.ID, err)
	}
	out := make([]port.Function, 0, len(defs))
	for _, d := range defs {
		f, err := s.functionOf(c, m, d, meta)
		if err != nil {
			return nil, meta, err
		}
		if _, err := names.Exposed(c.Alias, f.Name, s.cfg.Separator, s.cfg.NameMax); err != nil {
			return nil, meta, err
		}
		out = append(out, f)
	}
	return out, meta, nil
}

func outputSchemaOf(f port.Function) json.RawMessage {
	var d struct {
		Output json.RawMessage `json:"outputSchema"`
	}
	_ = json.Unmarshal(f.Definition, &d)
	return d.Output
}
