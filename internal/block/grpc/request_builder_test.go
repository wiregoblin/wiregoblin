package grpc

import (
	"encoding/base64"
	"math"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestRequestBuilderBuildsMessageFromJSONString(t *testing.T) {
	message := testRequestDescriptor(t)

	msg, err := NewRequestBuilder().Build(message, `{
		"user_id":"42",
		"active":"true",
		"profile":{"name":"demo"},
		"tags":["alpha","beta"],
		"labels":{"env":"dev","version":"v2"}
	}`)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	fields := msg.Descriptor().Fields()
	if got := msg.Get(fields.ByName("user_id")).Int(); got != 42 {
		t.Fatalf("user_id = %d, want 42", got)
	}
	if got := msg.Get(fields.ByName("active")).Bool(); !got {
		t.Fatalf("active = %v, want true", got)
	}

	profile := msg.Get(fields.ByName("profile")).Message()
	if got := profile.Get(profile.Descriptor().Fields().ByName("name")).String(); got != "demo" {
		t.Fatalf("profile.name = %q, want %q", got, "demo")
	}

	tags := msg.Get(fields.ByName("tags")).List()
	if tags.Len() != 2 || tags.Get(0).String() != "alpha" || tags.Get(1).String() != "beta" {
		t.Fatalf("tags = %#v", tags)
	}

	labels := msg.Get(fields.ByName("labels")).Map()
	if got := labels.Get(protoreflect.ValueOfString("env").MapKey()).String(); got != "dev" {
		t.Fatalf("labels[env] = %q, want %q", got, "dev")
	}
	if got := labels.Get(protoreflect.ValueOfString("version").MapKey()).String(); got != "v2" {
		t.Fatalf("labels[version] = %q, want %q", got, "v2")
	}
	msg, err = NewRequestBuilder().Build(message, `{"user_id":"7","profile":{"name":"json"}}`)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	fields = msg.Descriptor().Fields()
	if got := msg.Get(fields.ByName("user_id")).Int(); got != 7 {
		t.Fatalf("user_id = %d, want 7", got)
	}

	profile = msg.Get(fields.ByName("profile")).Message()
	if got := profile.Get(profile.Descriptor().Fields().ByName("name")).String(); got != "json" {
		t.Fatalf("profile.name = %q, want %q", got, "json")
	}
}

func TestRequestBuilderBytesStringIsNotDecodedImplicitly(t *testing.T) {
	message := bytesRequestDescriptor(t)

	msg, err := NewRequestBuilder().Build(message, `{"payload":"YWJj"}`)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	fields := msg.Descriptor().Fields()
	if got := string(msg.Get(fields.ByName("payload")).Bytes()); got != "YWJj" {
		t.Fatalf("payload = %q, want raw string bytes", got)
	}
}

func TestRequestBuilderBytesSupportsExplicitBase64Prefix(t *testing.T) {
	message := bytesRequestDescriptor(t)
	encoded := base64.StdEncoding.EncodeToString([]byte("abc"))

	msg, err := NewRequestBuilder().Build(message, `{"payload":"base64:`+encoded+`"}`)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	fields := msg.Descriptor().Fields()
	if got := string(msg.Get(fields.ByName("payload")).Bytes()); got != "abc" {
		t.Fatalf("payload = %q, want decoded base64 bytes", got)
	}
}

func TestAsInt64AcceptsUint(t *testing.T) {
	got, err := asInt64(uint(42), 64)
	if err != nil {
		t.Fatalf("asInt64() error = %v", err)
	}
	if got != 42 {
		t.Fatalf("asInt64() = %d, want 42", got)
	}
}

func TestAsInt64RejectsUint64Overflow(t *testing.T) {
	_, err := asInt64(uint64(math.MaxInt64)+1, 64)
	if err == nil {
		t.Fatal("asInt64() error = nil, want overflow error")
	}
	if !strings.Contains(err.Error(), "overflows int64") {
		t.Fatalf("asInt64() error = %q, want overflow error", err)
	}
}

func testRequestDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	file := &descriptorpb.FileDescriptorProto{
		Syntax:  proto.String("proto3"),
		Name:    proto.String("grpc_request_builder_test.proto"),
		Package: proto.String("grpcbuildertest"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Profile"),
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
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("LabelsEntry"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("key"),
								Number: proto.Int32(1),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
							{
								Name:   proto.String("value"),
								Number: proto.Int32(2),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
						Options: &descriptorpb.MessageOptions{
							MapEntry: proto.Bool(true),
						},
					},
				},
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("user_id"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
					{
						Name:   proto.String("active"),
						Number: proto.Int32(2),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
					},
					{
						Name:     proto.String("profile"),
						Number:   proto.Int32(3),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".grpcbuildertest.Profile"),
					},
					{
						Name:   proto.String("tags"),
						Number: proto.Int32(4),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:     proto.String("labels"),
						Number:   proto.Int32(5),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".grpcbuildertest.Request.LabelsEntry"),
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

func bytesRequestDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	file := &descriptorpb.FileDescriptorProto{
		Syntax:  proto.String("proto3"),
		Name:    proto.String("grpc_request_builder_bytes_test.proto"),
		Package: proto.String("grpcbuildertest"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("BytesRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("payload"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
					},
				},
			},
		},
	}

	descriptor, err := protodesc.NewFile(file, nil)
	if err != nil {
		t.Fatalf("protodesc.NewFile() error = %v", err)
	}

	return descriptor.Messages().ByName("BytesRequest")
}
