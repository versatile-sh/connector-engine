package grpcdesc

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const httpRuleField = 72295728

var httpVerbs = map[protowire.Number]string{2: "GET", 3: "PUT", 4: "POST", 5: "DELETE", 6: "PATCH"}

// Hints returns the idempotency_level name and the google.api.http verb of a method, each empty when absent; they only suggest a class.
func Hints(md protoreflect.MethodDescriptor) (idempotency, verb string) {
	opts, ok := md.Options().(*descriptorpb.MethodOptions)
	if !ok || opts == nil {
		return "", ""
	}
	if level := opts.GetIdempotencyLevel(); level != descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN {
		idempotency = level.String()
	}
	wire, err := proto.Marshal(opts)
	if err != nil {
		return idempotency, ""
	}
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			break
		}
		wire = wire[n:]
		size := protowire.ConsumeFieldValue(num, typ, wire)
		if size < 0 {
			break
		}
		if num == httpRuleField && typ == protowire.BytesType {
			rule, _ := protowire.ConsumeBytes(wire)
			return idempotency, ruleVerb(rule)
		}
		wire = wire[size:]
	}
	return idempotency, ""
}

func ruleVerb(rule []byte) string {
	for len(rule) > 0 {
		num, typ, n := protowire.ConsumeTag(rule)
		if n < 0 {
			return ""
		}
		rule = rule[n:]
		size := protowire.ConsumeFieldValue(num, typ, rule)
		if size < 0 {
			return ""
		}
		if verb, ok := httpVerbs[num]; ok && typ == protowire.BytesType {
			return verb
		}
		rule = rule[size:]
	}
	return ""
}
