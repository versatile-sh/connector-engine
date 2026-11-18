// Package reflection is the ListingSource that turns gRPC servers with reflection enabled into grpc_declarative listings.
package reflection

import (
	"context"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/agents-control/connector-engine/internal/digest"
	"github.com/agents-control/connector-engine/internal/grpcdesc"
	"github.com/agents-control/connector-engine/internal/importer"
	"github.com/agents-control/connector-engine/port"
)

// Registry is the name recorded as the provenance of a reflection import.
const Registry = "grpc-reflection"

var unsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// Config lists the targets to read; Insecure dials without TLS and Dial adds options such as credentials.
type Config struct {
	Targets  []string
	Insecure bool
	Dial     []grpc.DialOption
	Now      func() time.Time
}

// New returns a source with one listing per target; a target that offers no unary method is left out. The version is a digest of the descriptors, so a change of the server is a new version.
func New(cfg Config) *importer.Static {
	return &importer.Static{Registry: Registry, Now: cfg.Now, Load: func(ctx context.Context) ([]port.Manifest, error) {
		var out []port.Manifest
		for _, target := range cfg.Targets {
			m, ok, err := read(ctx, cfg, target)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, m)
			}
		}
		return out, nil
	}}
}

func read(ctx context.Context, cfg Config, target string) (port.Manifest, bool, error) {
	transport := credentials.NewTLS(nil)
	if cfg.Insecure {
		transport = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(target, append([]grpc.DialOption{grpc.WithTransportCredentials(transport)}, cfg.Dial...)...)
	if err != nil {
		return port.Manifest{}, false, port.Errorf(port.CodeInvalidArgument, "grpc target %q: %v", target, err)
	}
	defer conn.Close()
	files, err := grpcdesc.FromReflection(ctx, conn)
	if err != nil {
		return port.Manifest{}, false, err
	}
	sum, err := grpcdesc.Marshal(files)
	if err != nil {
		return port.Manifest{}, false, port.Errorf(port.CodeInternal, "descriptors of %s: %v", target, err)
	}
	g, ok := grpcdesc.Declare(target, port.Descriptor{From: port.DescriptorReflection}, files)
	if !ok {
		return port.Manifest{}, false, nil
	}
	return port.Manifest{
		Name:        "grpc.reflection/" + unsafe.ReplaceAllString(strings.ToLower(target), "-"),
		Version:     digest.Sum(sum)[:12],
		Title:       strings.Join(g.Services, ", "),
		Description: "gRPC services of " + target + " read through server reflection",
		Kind:        port.KindGRPCDeclarative,
		GRPC:        &g,
		Publisher:   port.Publisher{Name: target},
	}, true, nil
}
