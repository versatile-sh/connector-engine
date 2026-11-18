package grpcapi

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func encode(src any, dst proto.Message) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return protojson.Unmarshal(data, dst)
}

func decode(src protoreflect.Message, dst any) error {
	data, err := json.Marshal(tree(src))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func tree(m protoreflect.Message) map[string]any {
	out := map[string]any{}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList():
			list := make([]any, v.List().Len())
			for i := range list {
				list[i] = leaf(fd, v.List().Get(i))
			}
			out[string(fd.Name())] = list
		case fd.IsMap():
			items := map[string]any{}
			v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
				items[k.String()] = leaf(fd.MapValue(), mv)
				return true
			})
			out[string(fd.Name())] = items
		default:
			out[string(fd.Name())] = leaf(fd, v)
		}
		return true
	})
	return out
}

func leaf(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	if fd.Message() == nil {
		return v.Interface()
	}
	switch msg := v.Message().Interface().(type) {
	case *timestamppb.Timestamp:
		return msg.AsTime().Format(time.RFC3339Nano)
	case *structpb.Struct:
		return msg.AsMap()
	case *structpb.Value:
		return msg.AsInterface()
	}
	return tree(v.Message())
}
