package grpc

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestDescribeFieldsRecursesIntoRepeatedMessages(t *testing.T) {
	message := repeatedMessageDescriptor(t)

	fields := describeFields(message, "")

	if !hasFieldPath(fields, "items") {
		t.Fatalf("describeFields() missing repeated message field: %#v", fields)
	}
	if !hasFieldPath(fields, "items.name") {
		t.Fatalf("describeFields() missing nested repeated message field: %#v", fields)
	}
}

func TestDescribeFieldsStopsOnRecursiveMessages(t *testing.T) {
	message := recursiveMessageDescriptor(t)

	fields := describeFields(message, "")

	if !hasFieldPath(fields, "child") {
		t.Fatalf("describeFields() missing recursive field: %#v", fields)
	}
	if !hasFieldPath(fields, "child.value") {
		t.Fatalf("describeFields() missing one recursive nesting level: %#v", fields)
	}
	if hasFieldPath(fields, "child.child.child") {
		t.Fatalf("describeFields() recursed indefinitely: %#v", fields)
	}
}

func repeatedMessageDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	file := &descriptorpb.FileDescriptorProto{
		Syntax:  proto.String("proto3"),
		Name:    proto.String("grpc_reflection_test.proto"),
		Package: proto.String("grpcreflectiontest"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Item"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
			{
				Name: proto.String("Request"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("items"),
						Number:   proto.Int32(1),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".grpcreflectiontest.Item"),
					},
				},
			},
		},
	}

	descriptor, err := protodesc.NewFile(file, nil)
	if err != nil {
		t.Fatalf("protodesc.NewFile() error = %v", err)
	}

	return descriptor.Messages().ByName("Request")
}

func recursiveMessageDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	file := &descriptorpb.FileDescriptorProto{
		Syntax:  proto.String("proto3"),
		Name:    proto.String("grpc_reflection_recursive_test.proto"),
		Package: proto.String("grpcreflectiontest"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Node"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("value"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:     proto.String("child"),
						Number:   proto.Int32(2),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".grpcreflectiontest.Node"),
					},
				},
			},
		},
	}

	descriptor, err := protodesc.NewFile(file, nil)
	if err != nil {
		t.Fatalf("protodesc.NewFile() error = %v", err)
	}

	return descriptor.Messages().ByName("Node")
}

func hasFieldPath(fields []FieldInfo, want string) bool {
	for _, field := range fields {
		if field.Path == want {
			return true
		}
	}
	return false
}
