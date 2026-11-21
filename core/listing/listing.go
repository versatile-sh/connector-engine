// Package listing validates listing manifests, resolves the class of a function in class_source order and computes trust_level.
package listing

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/port"
)

var (
	reverseDNS = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+/[a-zA-Z0-9._-]+$`)
	grpcName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*_[A-Za-z_][A-Za-z0-9_]*$`)
	kinds      = []port.Kind{port.KindMCPRemote, port.KindMCPPackage, port.KindRESTDeclarative, port.KindGRPCDeclarative, port.KindSkill}
)

// Lint returns every problem of a manifest; requireDigest is listing.requireDigest.
func Lint(m port.Manifest, requireDigest bool) []port.Finding {
	var out []port.Finding
	bad := func(format string, args ...any) {
		out = append(out, port.Finding{Kind: string(port.CodeInvalidManifest), Severity: "error", Detail: fmt.Sprintf(format, args...)})
	}
	if !reverseDNS.MatchString(m.Name) {
		bad("name %q is a reverse-DNS string with one slash", m.Name)
	}
	if m.Version == "" {
		bad("version is required")
	}
	if m.Publisher.Name == "" {
		bad("publisher.name is required")
	}
	if !slices.Contains(kinds, m.Kind) {
		bad("kind %q is not one of %v; piece and wasm_component are reserved", m.Kind, kinds)
	}
	switch m.Kind {
	case port.KindMCPRemote:
		if m.Remote == nil || !strings.HasPrefix(m.Remote.URL, "https://") && !strings.HasPrefix(m.Remote.URL, "http://") {
			bad("an mcp_remote listing carries remote.url")
		}
	case port.KindMCPPackage:
		if m.Artifact == nil || m.Artifact.Ref == "" || (requireDigest && m.Artifact.Digest == "") {
			bad("an mcp_package listing carries artifact.ref and artifact.digest")
		}
		if m.Egress == nil || len(m.Egress.AllowHosts) == 0 {
			bad("an mcp_package listing carries egress.allow_hosts")
		}
	case port.KindRESTDeclarative:
		if m.REST == nil || m.REST.BaseURL == "" || len(m.REST.Functions) == 0 {
			bad("a rest_declarative listing carries rest.base_url and rest.functions")
		}
		if m.Egress == nil || len(m.Egress.AllowHosts) == 0 {
			bad("a rest_declarative listing carries egress.allow_hosts")
		}
		if m.REST != nil {
			for _, f := range m.REST.Functions {
				if f.Name == "" || f.Method == "" || !strings.HasPrefix(f.Path, "/") {
					bad("rest function %q carries name, method and a path that starts with /", f.Name)
				}
			}
		}
	case port.KindGRPCDeclarative:
		lintGRPC(m.GRPC, requireDigest, bad)
	case port.KindSkill:
		lintSkill(m.Skill, bad)
	}
	return out
}

func lintGRPC(g *port.GRPC, requireDigest bool, bad func(string, ...any)) {
	if g == nil || g.Target == "" || len(g.Methods) == 0 {
		bad("a grpc_declarative listing carries grpc.target and grpc.methods")
		return
	}
	switch g.Descriptor.From {
	case port.DescriptorReflection:
	case port.DescriptorFileSet, port.DescriptorBSR:
		if g.Descriptor.Ref == "" || (requireDigest && g.Descriptor.Digest == "") {
			bad("a %s descriptor carries ref and digest", g.Descriptor.From)
		}
	default:
		bad("grpc.descriptor.from %q is reflection, file_descriptor_set or bsr", g.Descriptor.From)
	}
	for _, f := range g.Methods {
		service, method := splitMethod(f.FullMethod)
		if !grpcName.MatchString(f.Name) || f.Name != shortService(service)+"_"+method {
			bad("grpc function %q is named <Service>_<Method> after %q", f.Name, f.FullMethod)
		}
	}
}

func splitMethod(full string) (string, string) {
	full = strings.TrimPrefix(full, "/")
	s, m, _ := strings.Cut(full, "/")
	return s, m
}

func shortService(s string) string { return s[strings.LastIndexByte(s, '.')+1:] }

// Validate refuses a manifest with any lint finding, with code invalid_manifest.
func Validate(m port.Manifest, requireDigest bool) error {
	f := Lint(m, requireDigest)
	if len(f) == 0 {
		return nil
	}
	details := make([]string, len(f))
	for i, x := range f {
		details[i] = x.Detail
	}
	return port.Errorf(port.CodeInvalidManifest, "%s", strings.Join(details, "; "))
}

// Private sets what the engine owns on a private listing: owner, source private, the tenant as publisher.
func Private(m port.Manifest, tenant port.TenantID) port.Manifest {
	m.Owner = tenant
	m.Source = &port.SourceInfo{Registry: port.PrivateSource}
	m.Publisher = port.Publisher{Name: string(tenant)}
	if m.TrustLevel == port.TrustFirstParty {
		m.TrustLevel = port.TrustUnreviewed
	}
	return m
}

// ClassInput is what class resolution reads for one function at one definition_hash.
type ClassInput struct {
	Hash            port.Hash
	AdminClass      port.Class
	AdminHash       port.Hash
	ReviewedClass   port.Class
	Review          *port.Review
	Annotations     *port.Annotations
	HTTPMethod      string
	GRPCIdempotency string
}

// Class resolves the trusted class in class_source order and a suggested class that is shown, never applied.
func Class(in ClassInput) (port.Class, port.ClassSource, port.Class) {
	suggested := suggest(in)
	switch {
	case in.AdminClass != port.ClassNone && in.AdminHash == in.Hash:
		return in.AdminClass, port.ClassSourceAdmin, suggested
	case in.ReviewedClass != port.ClassNone && in.Review != nil && slices.Contains(in.Review.DefinitionHashes, in.Hash):
		return in.ReviewedClass, port.ClassSourceReviewed, suggested
	case in.Annotations != nil && in.Annotations.DestructiveHint != nil && *in.Annotations.DestructiveHint:
		return port.ClassDestructive, port.ClassSourceAnnotationRaise, suggested
	}
	return port.ClassNone, port.ClassSourceDefault, suggested
}

// Base is the class of f that the pinned manifest m gives without any admin class: an item reads, then the review, the declaration, the default.
func Base(f port.Function, m port.Manifest) (port.Class, port.ClassSource) {
	if port.ItemKind(f.Name) != "" {
		return port.ClassRead, port.ClassSourceItem
	}
	var d struct {
		Annotations *port.Annotations `json:"annotations"`
	}
	_ = json.Unmarshal(f.Definition, &d)
	in := ClassInput{Hash: f.DefinitionHash, Review: m.Review, Annotations: d.Annotations}
	if i := slices.IndexFunc(m.Functions, func(def port.FunctionDef) bool { return def.Name == f.Name }); i >= 0 {
		in.ReviewedClass = m.Functions[i].Class
	}
	class, source, _ := Class(in)
	return class, source
}

func suggest(in ClassInput) port.Class {
	if a := in.Annotations; a != nil && a.ReadOnlyHint != nil && *a.ReadOnlyHint {
		return port.ClassRead
	}
	switch strings.ToUpper(in.HTTPMethod) {
	case "GET", "HEAD":
		return port.ClassRead
	case "POST", "PUT", "PATCH":
		return port.ClassWrite
	case "DELETE":
		return port.ClassDestructive
	}
	switch in.GRPCIdempotency {
	case "NO_SIDE_EFFECTS":
		return port.ClassRead
	case "IDEMPOTENT":
		return port.ClassWrite
	}
	return port.ClassNone
}

// Evidence is what was established about a listing version besides its manifest.
type Evidence struct {
	NamespaceVerified bool
	Signed            bool
	FirstParty        bool
}

// Trust computes trust_level: first_party only for an installation listing the operator built; reviewed when the review covers every current hash.
func Trust(m port.Manifest, e Evidence) port.TrustLevel {
	switch {
	case e.FirstParty && m.Owner == port.Installation:
		return port.TrustFirstParty
	case covered(m):
		return port.TrustReviewed
	case e.Signed:
		return port.TrustSigned
	case e.NamespaceVerified || m.Publisher.VerifiedBy != "":
		return port.TrustNamespaceVerified
	}
	return port.TrustUnreviewed
}

func covered(m port.Manifest) bool {
	if m.Review == nil || len(m.Functions) == 0 {
		return false
	}
	for _, f := range m.Functions {
		if !slices.Contains(m.Review.DefinitionHashes, f.DefinitionHash) {
			return false
		}
	}
	return true
}

// InstallLevel is the workspace level a new install gets: write for reviewed and first_party, off for the rest.
func InstallLevel(t port.TrustLevel) port.Level {
	if t == port.TrustReviewed || t == port.TrustFirstParty {
		return port.LevelWrite
	}
	return port.LevelOff
}
