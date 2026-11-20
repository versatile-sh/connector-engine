// Package bsr is the ListingSource over the Buf Schema Registry API, pinned to buf.registry.module.v1: modules with a gRPC target become grpc_declarative listings.
package bsr

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/versatile-sh/connector-engine/internal/digest"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/fetch"
	"github.com/versatile-sh/connector-engine/internal/grpcdesc"
	"github.com/versatile-sh/connector-engine/internal/importer"
	"github.com/versatile-sh/connector-engine/port"
)

const (
	// DefaultURL is the public BSR.
	DefaultURL = "https://buf.build"

	moduleService        = "/buf.registry.module.v1.ModuleService/"
	descriptorSetService = "/buf.registry.module.v1.FileDescriptorSetService/"
	activeState          = "MODULE_STATE_ACTIVE"
)

// Config selects the owners to list and the gRPC target each module is served on.
type Config struct {
	Name     string
	BaseURL  string
	Token    string
	Owners   []string
	Targets  map[string]string
	PageSize int
	HTTP     *http.Client
	Guard    egress.Guard
	Now      func() time.Time
}

// Source lists modules per owner and fetches compiled descriptors; a module without a target is not callable and is left out.
type Source struct{ cfg Config }

// New returns a source; Targets maps "owner/module" to a gRPC target.
func New(cfg Config) *Source {
	if cfg.Name == "" {
		cfg.Name = "bsr"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultURL
	}
	if cfg.HTTP == nil {
		cfg.HTTP = cfg.Guard.Client()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Source{cfg: cfg}
}

// Name is the registry name recorded as provenance.
func (s *Source) Name() string { return s.cfg.Name }

func (s *Source) call(ctx context.Context, service string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return port.Errorf(port.CodeInternal, "bsr request: %v", err)
	}
	headers := map[string]string{"Content-Type": "application/json", "Connect-Protocol-Version": "1"}
	if s.cfg.Token != "" {
		headers["Authorization"] = "Bearer " + s.cfg.Token
	}
	res, err := fetch.Do(ctx, s.cfg.HTTP, http.MethodPost, s.cfg.BaseURL+service, bytes.NewReader(body), headers)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(res, out); err != nil {
		return port.Errorf(port.CodeUnavailable, "bsr response: %v", err)
	}
	return nil
}

type moduleJSON struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description"`
}

// List returns one page of the modules of one owner; the cursor is the owner's index and the BSR page token.
func (s *Source) List(ctx context.Context, cursor string) (port.ListingPage, error) {
	owner, token := 0, ""
	if cursor != "" {
		idx, tok, _ := strings.Cut(cursor, ":")
		var err error
		if owner, err = strconv.Atoi(idx); err != nil || owner < 0 {
			return port.ListingPage{}, port.Errorf(port.CodeInvalidArgument, "cursor %q is not an owner index and a page token", cursor)
		}
		token = tok
	}
	if owner >= len(s.cfg.Owners) {
		return port.ListingPage{}, nil
	}
	name := s.cfg.Owners[owner]
	req := map[string]any{"ownerRefs": []any{map[string]string{"name": name}}, "pageToken": token}
	if s.cfg.PageSize > 0 {
		req["pageSize"] = s.cfg.PageSize
	}
	var res struct {
		NextPageToken string       `json:"nextPageToken"`
		Modules       []moduleJSON `json:"modules"`
	}
	if err := s.call(ctx, moduleService+"ListModules", req, &res); err != nil {
		return port.ListingPage{}, err
	}
	var page port.ListingPage
	for _, mod := range res.Modules {
		target, ok := s.cfg.Targets[name+"/"+mod.Name]
		if !ok || mod.State != activeState {
			continue
		}
		m, err := s.manifest(ctx, name, mod, target, "")
		if err != nil {
			return port.ListingPage{}, err
		}
		page.Manifests = append(page.Manifests, m)
	}
	switch {
	case res.NextPageToken != "":
		page.Next = strconv.Itoa(owner) + ":" + res.NextPageToken
	case owner+1 < len(s.cfg.Owners):
		page.Next = strconv.Itoa(owner+1) + ":"
	}
	return page, nil
}

type descriptors struct {
	set    *descriptorpb.FileDescriptorSet
	commit string
}

func (s *Source) descriptors(ctx context.Context, owner, module, ref string) (descriptors, error) {
	name := map[string]string{"owner": owner, "module": module}
	if ref != "" {
		name["ref"] = ref
	}
	var res struct {
		Set    json.RawMessage `json:"fileDescriptorSet"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := s.call(ctx, descriptorSetService+"GetFileDescriptorSet", map[string]any{"resourceRef": map[string]any{"name": name}}, &res); err != nil {
		return descriptors{}, err
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(res.Set, set); err != nil {
		return descriptors{}, port.Errorf(port.CodeUnavailable, "bsr descriptor set of %s/%s: %v", owner, module, err)
	}
	return descriptors{set: set, commit: res.Commit.ID}, nil
}

func (s *Source) manifest(ctx context.Context, owner string, mod moduleJSON, target, ref string) (port.Manifest, error) {
	d, err := s.descriptors(ctx, owner, mod.Name, ref)
	if err != nil {
		return port.Manifest{}, err
	}
	files, err := grpcdesc.Resolve(d.set)
	if err != nil {
		return port.Manifest{}, err
	}
	b, err := grpcdesc.Marshal(files)
	if err != nil {
		return port.Manifest{}, port.Errorf(port.CodeInternal, "descriptors of %s/%s: %v", owner, mod.Name, err)
	}
	g, ok := grpcdesc.Declare(target, port.Descriptor{From: port.DescriptorBSR, Ref: owner + "/" + mod.Name, Digest: digest.Sum(b)}, files)
	if !ok {
		return port.Manifest{}, port.Errorf(port.CodeInvalidManifest, "%s/%s declares no unary method", owner, mod.Name)
	}
	m := port.Manifest{
		Name:        "build.buf/" + owner + "." + mod.Name,
		Version:     d.commit,
		Title:       owner + "/" + mod.Name,
		Description: mod.Description,
		Kind:        port.KindGRPCDeclarative,
		GRPC:        &g,
		Publisher:   port.Publisher{Name: owner},
	}
	if m.Version == "" {
		m.Version = g.Descriptor.Digest[:12]
	}
	return importer.Imported(m, s.cfg.Name, s.cfg.Now()), nil
}

func (s *Source) lookup(name string) (owner, module, target string, err error) {
	rest, ok := strings.CutPrefix(name, "build.buf/")
	owner, module, found := strings.Cut(rest, ".")
	if !ok || !found {
		return "", "", "", port.Errorf(port.CodeNotFound, "%q is not a BSR listing", name)
	}
	target, ok = s.cfg.Targets[owner+"/"+module]
	if !ok {
		return "", "", "", port.Errorf(port.CodeNotFound, "no gRPC target is configured for %s/%s", owner, module)
	}
	return owner, module, target, nil
}

// Fetch reads one module at a commit; an empty version or latest means the default label.
func (s *Source) Fetch(ctx context.Context, name, version string) (port.Manifest, error) {
	owner, module, target, err := s.lookup(name)
	if err != nil {
		return port.Manifest{}, err
	}
	if version == "latest" {
		version = ""
	}
	return s.manifest(ctx, owner, moduleJSON{Name: module}, target, version)
}

// FetchDescriptor returns the bytes whose digest a bsr descriptor carries, for the gRPC runtime's Fetch option.
func (s *Source) FetchDescriptor(ctx context.Context, d port.Descriptor) ([]byte, error) {
	if d.From != port.DescriptorBSR {
		return nil, port.Errorf(port.CodeInvalidManifest, "the BSR source serves bsr descriptors, not %s", d.From)
	}
	owner, module, ok := strings.Cut(d.Ref, "/")
	if !ok {
		return nil, port.Errorf(port.CodeInvalidManifest, "bsr descriptor ref %q is owner/module", d.Ref)
	}
	got, err := s.descriptors(ctx, owner, module, "")
	if err != nil {
		return nil, err
	}
	files, err := grpcdesc.Resolve(got.set)
	if err != nil {
		return nil, err
	}
	return grpcdesc.Marshal(files)
}
