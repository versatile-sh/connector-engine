package listing_test

import (
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/port"
)

func yes() *bool { v := true; return &v }

func remote() port.Manifest {
	return port.Manifest{Name: "io.github.acme/issues", Version: "1.0.0", Kind: port.KindMCPRemote, Remote: &port.Remote{URL: "https://mcp.example/mcp"}, Publisher: port.Publisher{Name: "acme"}}
}

func TestLint(t *testing.T) {
	if err := listing.Validate(remote(), true); err != nil {
		t.Fatal(err)
	}
	m := remote()
	m.Name, m.Kind = "issues", port.KindPiece
	if f := listing.Lint(m, true); len(f) != 2 {
		t.Fatalf("a bad name and a reserved kind: %+v", f)
	}
	pkg := port.Manifest{Name: "io.x/p", Version: "1", Kind: port.KindMCPPackage, Publisher: port.Publisher{Name: "x"}, Artifact: &port.Artifact{Type: "oci", Ref: "r"}}
	if err := listing.Validate(pkg, true); err == nil || !strings.Contains(err.Error(), "digest") || !strings.Contains(err.Error(), "egress") {
		t.Fatalf("an mcp_package needs a digest and egress.allow_hosts: %v", err)
	}
	g := port.Manifest{Name: "io.x/g", Version: "1", Kind: port.KindGRPCDeclarative, Publisher: port.Publisher{Name: "x"}, GRPC: &port.GRPC{
		Target: "dns:///api:443", Descriptor: port.Descriptor{From: port.DescriptorReflection},
		Methods: []port.GRPCFunction{{Name: "Orders_Get", FullMethod: "/shop.v1.Orders/Get"}, {Name: "get", FullMethod: "/shop.v1.Orders/Get"}},
	}}
	if f := listing.Lint(g, true); len(f) != 1 || !strings.Contains(f[0].Detail, `"get"`) {
		t.Fatalf("a gRPC function is named <Service>_<Method>: %+v", f)
	}
}

func TestClassSourceOrder(t *testing.T) {
	review := &port.Review{DefinitionHashes: []port.Hash{"h1"}}
	for _, tc := range []struct {
		name   string
		in     listing.ClassInput
		class  port.Class
		source port.ClassSource
	}{
		{"admin override at this hash", listing.ClassInput{Hash: "h1", AdminClass: port.ClassRead, AdminHash: "h1", ReviewedClass: port.ClassWrite, Review: review}, port.ClassRead, port.ClassSourceAdmin},
		{"admin override at another hash falls to the review", listing.ClassInput{Hash: "h1", AdminClass: port.ClassRead, AdminHash: "h0", ReviewedClass: port.ClassWrite, Review: review}, port.ClassWrite, port.ClassSourceReviewed},
		{"a review that does not cover the hash", listing.ClassInput{Hash: "h2", ReviewedClass: port.ClassRead, Review: review}, port.ClassNone, port.ClassSourceDefault},
		{"an unreviewed destructiveHint raises", listing.ClassInput{Hash: "h2", Annotations: &port.Annotations{DestructiveHint: yes()}}, port.ClassDestructive, port.ClassSourceAnnotationRaise},
		{"an unreviewed readOnlyHint is a suggestion only", listing.ClassInput{Hash: "h2", Annotations: &port.Annotations{ReadOnlyHint: yes()}}, port.ClassNone, port.ClassSourceDefault},
	} {
		c, s, _ := listing.Class(tc.in)
		if c != tc.class || s != tc.source {
			t.Errorf("%s: %s %s", tc.name, c, s)
		}
	}
	if _, _, s := listing.Class(listing.ClassInput{HTTPMethod: "get"}); s != port.ClassRead {
		t.Fatal("an HTTP GET suggests read")
	}
}

func TestBaseClassIgnoresTheAdminClass(t *testing.T) {
	m := port.Manifest{Review: &port.Review{DefinitionHashes: []port.Hash{"h1"}}, Functions: []port.FunctionDef{{Name: "purge", Class: port.ClassDestructive}}}
	for _, tc := range []struct {
		name   string
		f      port.Function
		class  port.Class
		source port.ClassSource
	}{
		{"a reviewed class under an admin lowering", port.Function{Name: "purge", DefinitionHash: "h1", Class: port.ClassWrite, ClassSource: port.ClassSourceAdmin}, port.ClassDestructive, port.ClassSourceReviewed},
		{"a review of another hash", port.Function{Name: "purge", DefinitionHash: "h2", Class: port.ClassWrite, ClassSource: port.ClassSourceAdmin}, port.ClassNone, port.ClassSourceDefault},
		{"a declared destructive function", port.Function{Name: "wipe", Definition: []byte(`{"annotations":{"destructiveHint":true}}`), Class: port.ClassRead, ClassSource: port.ClassSourceAdmin}, port.ClassDestructive, port.ClassSourceAnnotationRaise},
		{"a read-only hint", port.Function{Name: "list", Definition: []byte(`{"annotations":{"readOnlyHint":true}}`), Class: port.ClassRead, ClassSource: port.ClassSourceAdmin}, port.ClassNone, port.ClassSourceDefault},
		{"an item", port.Function{Name: "prompt:hello", Class: port.ClassRead, ClassSource: port.ClassSourceItem}, port.ClassRead, port.ClassSourceItem},
	} {
		if c, s := listing.Base(tc.f, m); c != tc.class || s != tc.source {
			t.Errorf("%s: %s %s", tc.name, c, s)
		}
	}
}

func TestTrust(t *testing.T) {
	m := remote()
	m.Functions = []port.FunctionDef{{Name: "a", DefinitionHash: "h1"}, {Name: "b", DefinitionHash: "h2"}}
	if listing.Trust(m, listing.Evidence{}) != port.TrustUnreviewed {
		t.Fatal("no evidence")
	}
	m.Review = &port.Review{DefinitionHashes: []port.Hash{"h1"}}
	if listing.Trust(m, listing.Evidence{Signed: true}) != port.TrustSigned {
		t.Fatal("a review that misses a hash is not reviewed")
	}
	m.Review.DefinitionHashes = append(m.Review.DefinitionHashes, "h2")
	if listing.Trust(m, listing.Evidence{}) != port.TrustReviewed || listing.InstallLevel(port.TrustReviewed) != port.LevelWrite {
		t.Fatal("reviewed installs at level write")
	}
	p := listing.Private(m, "t1")
	if listing.Trust(p, listing.Evidence{FirstParty: true}) == port.TrustFirstParty || p.Source.Registry != port.PrivateSource || p.Publisher.Name != "t1" {
		t.Fatalf("a private listing is never first_party and carries the private source: %+v", p)
	}
	if listing.InstallLevel(port.TrustSigned) != port.LevelOff {
		t.Fatal("other listings install at off")
	}
}
