package grpcdesc

import (
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/agents-control/connector-engine/port"
)

// FromSet resolves a serialized FileDescriptorSet.
func FromSet(b []byte) (*protoregistry.Files, error) {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &set); err != nil {
		return nil, port.Errorf(port.CodeInvalidManifest, "file descriptor set: %v", err)
	}
	return Resolve(&set)
}

// Resolve builds the registry of a FileDescriptorSet.
func Resolve(set *descriptorpb.FileDescriptorSet) (*protoregistry.Files, error) {
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidManifest, "file descriptor set: %v", err)
	}
	return files, nil
}

// Marshal serializes the files as a FileDescriptorSet in name order, so equal sets have equal bytes.
func Marshal(files *protoregistry.Files) ([]byte, error) {
	set := &descriptorpb.FileDescriptorSet{}
	files.RangeFiles(func(f protoreflect.FileDescriptor) bool {
		set.File = append(set.File, protodesc.ToFileDescriptorProto(f))
		return true
	})
	slices.SortFunc(set.File, func(a, b *descriptorpb.FileDescriptorProto) int { return strings.Compare(a.GetName(), b.GetName()) })
	return proto.MarshalOptions{Deterministic: true}.Marshal(set)
}

// Method is a unary method with the function definition derived from its descriptor.
type Method struct {
	Descriptor  protoreflect.MethodDescriptor
	Function    port.GRPCFunction
	Idempotency string
	HTTPVerb    string
}

// FullMethod is the wire name of a method: /package.Service/Method.
func FullMethod(md protoreflect.MethodDescriptor) string {
	return "/" + string(md.Parent().FullName()) + "/" + string(md.Name())
}

// FunctionName is <Service>_<Method> with the service name unqualified.
func FunctionName(md protoreflect.MethodDescriptor) string {
	return string(md.Parent().Name()) + "_" + string(md.Name())
}

// Describe derives the function of a method.
func Describe(md protoreflect.MethodDescriptor) Method {
	idempotency, verb := Hints(md)
	return Method{
		Descriptor: md,
		Function: port.GRPCFunction{
			Name:         FunctionName(md),
			FullMethod:   FullMethod(md),
			InputSchema:  Schema(md.Input()),
			OutputSchema: Schema(md.Output()),
		},
		Idempotency: idempotency,
		HTTPVerb:    verb,
	}
}

// Unary returns the method of a wire name, or an invalid_manifest error when it is missing or streams (grpc.unaryOnly).
func Unary(files *protoregistry.Files, fullMethod string) (protoreflect.MethodDescriptor, error) {
	service, method, _ := strings.Cut(strings.TrimPrefix(fullMethod, "/"), "/")
	d, err := files.FindDescriptorByName(protoreflect.FullName(service))
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if err != nil || !ok {
		return nil, port.Errorf(port.CodeInvalidManifest, "service %q is not in the descriptor set", service)
	}
	md := sd.Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return nil, port.Errorf(port.CodeInvalidManifest, "method %q is not in the descriptor set", fullMethod)
	}
	if md.IsStreamingClient() || md.IsStreamingServer() {
		return nil, port.Errorf(port.CodeInvalidManifest, "grpc.unaryOnly: %s streams", fullMethod)
	}
	return md, nil
}

// Methods lists the unary methods of the named services, or of every service but the reflection and health ones when none is named.
func Methods(files *protoregistry.Files, services []string) []Method {
	var out []Method
	files.RangeFiles(func(f protoreflect.FileDescriptor) bool {
		for i := range f.Services().Len() {
			sd := f.Services().Get(i)
			if len(services) > 0 && !slices.Contains(services, string(sd.FullName())) || len(services) == 0 && infrastructure(sd) {
				continue
			}
			for j := range sd.Methods().Len() {
				if md := sd.Methods().Get(j); !md.IsStreamingClient() && !md.IsStreamingServer() {
					out = append(out, Describe(md))
				}
			}
		}
		return true
	})
	slices.SortFunc(out, func(a, b Method) int { return strings.Compare(a.Function.FullMethod, b.Function.FullMethod) })
	return out
}

func infrastructure(sd protoreflect.ServiceDescriptor) bool {
	n := string(sd.FullName())
	return strings.HasPrefix(n, "grpc.reflection.") || strings.HasPrefix(n, "grpc.health.")
}

// ServiceNames lists the distinct services of the methods, sorted.
func ServiceNames(methods []Method) []string {
	var out []string
	for _, m := range methods {
		if s := string(m.Descriptor.Parent().FullName()); !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

// Declare builds the grpc block of a manifest from the unary methods of every service; it reports false when there are none.
func Declare(target string, d port.Descriptor, files *protoregistry.Files) (port.GRPC, bool) {
	methods := Methods(files, nil)
	g := port.GRPC{Target: target, Descriptor: d, Services: ServiceNames(methods)}
	for _, m := range methods {
		g.Methods = append(g.Methods, m.Function)
	}
	return g, len(methods) > 0
}
