package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/core/names"
	"github.com/versatile-sh/connector-engine/core/pinning"
	"github.com/versatile-sh/connector-engine/port"
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

const handleIdle = 15 * time.Minute

type started struct {
	key     string
	ready   chan struct{}
	h       port.Handle
	err     error
	refs    int
	used    time.Time
	dropped bool
}

func upstream(err error, format string, args ...any) error {
	if pe := (*port.Error)(nil); errors.As(err, &pe) {
		return err
	}
	return port.Errorf(port.CodeUnavailable, format, append(args, err)...)
}

func (s *Service) handle(ctx context.Context, c port.Connector, m port.Manifest, cred port.CredentialID, owner port.OwnerKind) (*started, error) {
	key := s.handleKey(c, m.Version, cred)
	s.mu.Lock()
	e, ok := s.handles[key]
	if !ok {
		e = &started{key: key, ready: make(chan struct{})}
		s.handles[key] = e
	}
	e.refs++
	idle := s.takeIdle(key)
	s.mu.Unlock()
	s.stopAll(ctx, idle)
	if !ok {
		e.h, e.err = s.d.Runtime.Start(ctx, c, m, port.SecretsRef{Tenant: c.Tenant, Credential: cred, Owner: owner})
		close(e.ready)
	}
	select {
	case <-e.ready:
	case <-ctx.Done():
		s.release(ctx, e)
		return nil, ctx.Err()
	}
	if e.err != nil {
		s.drop(e)
		s.release(ctx, e)
		return nil, upstream(e.err, "connector %s did not start: %v", c.ID)
	}
	return e, nil
}

func (s *Service) takeIdle(keep string) []*started {
	var out []*started
	for k, e := range s.handles {
		if k != keep && e.refs == 0 && e.err == nil && s.now().Sub(e.used) > handleIdle {
			delete(s.handles, k)
			e.dropped = true
			out = append(out, e)
		}
	}
	return out
}

func (s *Service) release(ctx context.Context, e *started) {
	s.mu.Lock()
	e.refs--
	e.used = s.now()
	stop := e.dropped && e.refs == 0 && e.err == nil
	s.mu.Unlock()
	if stop {
		_ = s.d.Runtime.Stop(context.WithoutCancel(ctx), e.h)
	}
}

func (s *Service) drop(e *started) {
	s.mu.Lock()
	if s.handles[e.key] == e {
		delete(s.handles, e.key)
	}
	e.dropped = true
	s.mu.Unlock()
}

func (s *Service) stopAll(ctx context.Context, list []*started) {
	for _, e := range list {
		_ = s.d.Runtime.Stop(context.WithoutCancel(ctx), e.h)
	}
}

func (s *Service) dropHandles(ctx context.Context, match func(key string) bool) {
	s.mu.Lock()
	var stop []*started
	for k, e := range s.handles {
		if match(k) {
			delete(s.handles, k)
			e.dropped = true
			if e.refs == 0 && e.err == nil {
				stop = append(stop, e)
			}
		}
	}
	s.mu.Unlock()
	s.stopAll(ctx, stop)
}

func (s *Service) stopHandles(ctx context.Context, tenant port.TenantID) {
	s.dropHandles(ctx, func(k string) bool { return strings.HasPrefix(k, string(tenant)+"|") })
}

func (s *Service) stopConnector(ctx context.Context, c port.Connector) {
	prefix := strings.Join([]string{string(c.Tenant), string(c.ID)}, "|") + "|"
	s.dropHandles(ctx, func(k string) bool { return strings.HasPrefix(k, prefix) })
}

func (s *Service) functionOf(c port.Connector, m port.Manifest, def port.RawDefinition, meta port.ListMeta) (port.Function, error) {
	hash, err := pinning.DefinitionHash(def.Name, def.Raw)
	if err != nil {
		return port.Function{}, err
	}
	v := viewOf(def.Raw)
	f := port.Function{Tenant: c.Tenant, Connector: c.ID, Name: def.Name, Definition: slices.Clone(def.Raw), DefinitionHash: hash, Visibility: v.Meta.UI.Visibility,
		OpenWorld: port.ItemKind(def.Name) == "" && (v.Annotations == nil || v.Annotations.OpenWorldHint == nil || *v.Annotations.OpenWorldHint)}
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
	if port.ItemKind(f.Name) == "" && f.ClassSource == port.ClassSourceAdmin && f.Class != port.ClassNone {
		return f.Class, port.ClassSourceAdmin
	}
	return listing.Base(f, m)
}

var errSignInNeeded = errors.New("the connector lists its functions after somebody signs in")

func needsOAuth(m port.Manifest) bool {
	return slices.ContainsFunc(m.Auth, func(a port.AuthSpec) bool { return a.Kind == port.CredentialOAuth2User })
}

func (s *Service) signedInCredential(ctx context.Context, c port.Connector, m port.Manifest) (port.Credential, bool, error) {
	if !needsOAuth(m) {
		return port.Credential{}, false, nil
	}
	var creds []port.Credential
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: c.Tenant}, func(tx port.Tx) (err error) {
		creds, err = tx.ListCredentials(port.CredentialFilter{Connector: c.ID})
		return err
	}); err != nil {
		return port.Credential{}, false, err
	}
	creds = slices.DeleteFunc(creds, func(cr port.Credential) bool {
		return cr.Kind != port.CredentialOAuth2User || cr.Status != port.CredentialActive
	})
	if len(creds) == 0 {
		return port.Credential{}, false, nil
	}
	if i := slices.IndexFunc(creds, func(cr port.Credential) bool { return cr.OwnerKind == port.OwnerWorkspace }); i > 0 {
		return creds[i], true, nil
	}
	return creds[0], true, nil
}

func (s *Service) captureFunctions(ctx context.Context, c port.Connector, m port.Manifest) ([]port.Function, port.ListMeta, error) {
	cred, signedIn, err := s.signedInCredential(ctx, c, m)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	fns, meta, err := s.captureWith(ctx, c, m, cred)
	if err != nil && needsOAuth(m) && !signedIn {
		err = fmt.Errorf("%w: %w", errSignInNeeded, err)
	}
	return fns, meta, err
}

func (s *Service) captureWith(ctx context.Context, c port.Connector, m port.Manifest, cred port.Credential) ([]port.Function, port.ListMeta, error) {
	h, err := s.handle(ctx, c, m, cred.ID, cred.OwnerKind)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	defer s.release(ctx, h)
	defs, meta, err := s.d.Runtime.ListFunctions(ctx, h.h)
	if err != nil {
		if port.IsCode(err, port.CodeUnavailable) {
			s.drop(h)
		}
		return nil, meta, upstream(err, "connector %s did not list its functions: %v", c.ID)
	}
	for _, d := range defs {
		if port.ItemKind(d.Name) != "" {
			return nil, meta, port.Errorf(port.CodeInvalidManifest, "tool %q uses a name reserved for connector items", d.Name)
		}
		if _, err := names.Exposed(c.Alias, d.Name, s.cfg.Separator, s.cfg.NameMax); err != nil {
			return nil, meta, err
		}
	}
	items, err := s.itemDefinitions(ctx, h.h, meta)
	if err != nil {
		if port.IsCode(err, port.CodeUnavailable) {
			s.drop(h)
		}
		return nil, meta, upstream(err, "connector %s did not list its prompts and resources: %v", c.ID)
	}
	defs = append(defs, items...)
	out := make([]port.Function, 0, len(defs))
	for _, d := range defs {
		if err := names.ValidUpstream(d.Name); err != nil {
			return nil, meta, err
		}
		f, err := s.functionOf(c, m, d, meta)
		if err != nil {
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
