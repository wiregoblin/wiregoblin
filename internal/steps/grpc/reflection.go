package grpc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jhump/protoreflect/v2/grpcreflect"
	"github.com/jhump/protoreflect/v2/protoresolve"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ConnectResponse contains the discovered services for a connected gRPC server.
type ConnectResponse struct {
	Address  string        `json:"address"`
	Services []ServiceInfo `json:"services"`
}

// ReflectionService loads gRPC descriptors from server reflection.
type ReflectionService struct {
	client *Client
}

// ServiceInfo describes a reflected gRPC service and its unary methods.
type ServiceInfo struct {
	Name    string       `json:"name"`
	Methods []MethodInfo `json:"methods"`
}

// MethodInfo describes a reflected unary method.
type MethodInfo struct {
	Name       string `json:"name"`
	FullMethod string `json:"fullMethod"`
}

// MethodSchema describes the request or response fields for a unary method.
type MethodSchema struct {
	Service    string      `json:"service"`
	Method     string      `json:"method"`
	FullMethod string      `json:"fullMethod"`
	Fields     []FieldInfo `json:"fields"`
}

// FieldInfo describes a request field exposed in the generated form.
type FieldInfo struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	TypeName   string `json:"typeName,omitempty"`
	Path       string `json:"path"`
	Repeated   bool   `json:"repeated"`
	IsMessage  bool   `json:"isMessage"`
	IsEnum     bool   `json:"isEnum"`
	IsMap      bool   `json:"isMap"`
	IsRequired bool   `json:"isRequired"`
}

// NewReflectionService creates a reflection service backed by the active gRPC connection.
func NewReflectionService(client *Client) *ReflectionService {
	return &ReflectionService{client: client}
}

// ListServices returns reflected services with unary methods only.
func (s *ReflectionService) ListServices(ctx context.Context) ([]ServiceInfo, error) {
	reflectClient, err := s.newClient(ctx)
	if err != nil {
		return nil, err
	}
	defer reflectClient.Reset()

	names, err := reflectClient.ListServices()
	if err != nil {
		return nil, fmt.Errorf("list services via reflection: %w", err)
	}

	var services []ServiceInfo
	for _, name := range names {
		serviceName := string(name)
		if strings.HasPrefix(serviceName, "grpc.reflection.") {
			continue
		}

		serviceDescriptor, err := resolveServiceDescriptor(reflectClient, name)
		if err != nil {
			return nil, fmt.Errorf("resolve service %s: %w", serviceName, err)
		}

		methods := make([]MethodInfo, 0, serviceDescriptor.Methods().Len())
		for i := 0; i < serviceDescriptor.Methods().Len(); i++ {
			method := serviceDescriptor.Methods().Get(i)
			if method.IsStreamingClient() || method.IsStreamingServer() {
				continue
			}

			methods = append(methods, MethodInfo{
				Name:       string(method.Name()),
				FullMethod: fmt.Sprintf("/%s/%s", serviceDescriptor.FullName(), method.Name()),
			})
		}

		if len(methods) == 0 {
			continue
		}

		sort.Slice(methods, func(i, j int) bool {
			return methods[i].Name < methods[j].Name
		})

		services = append(services, ServiceInfo{
			Name:    string(serviceDescriptor.FullName()),
			Methods: methods,
		})
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	return services, nil
}

// GetMethodSchema resolves the method and describes its request fields.
func (s *ReflectionService) GetMethodSchema(ctx context.Context, fullMethod string) (*MethodSchema, error) {
	method, err := s.ResolveMethod(ctx, fullMethod)
	if err != nil {
		return nil, err
	}

	fields := describeFields(method.Input(), "")

	return &MethodSchema{
		Service:    string(method.Parent().FullName()),
		Method:     string(method.Name()),
		FullMethod: fullMethod,
		Fields:     fields,
	}, nil
}

// GetResponseSchema resolves the method and describes its response fields.
func (s *ReflectionService) GetResponseSchema(ctx context.Context, fullMethod string) (*MethodSchema, error) {
	method, err := s.ResolveMethod(ctx, fullMethod)
	if err != nil {
		return nil, err
	}

	fields := describeFields(method.Output(), "")

	return &MethodSchema{
		Service:    string(method.Parent().FullName()),
		Method:     string(method.Name()),
		FullMethod: fullMethod,
		Fields:     fields,
	}, nil
}

// ResolveMethod resolves a unary method descriptor by its full RPC path.
func (s *ReflectionService) ResolveMethod(
	ctx context.Context,
	fullMethod string,
) (protoreflect.MethodDescriptor, error) {
	serviceName, methodName, err := splitFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	reflectClient, err := s.newClient(ctx)
	if err != nil {
		return nil, err
	}
	defer reflectClient.Reset()

	serviceDescriptor, err := resolveServiceDescriptor(reflectClient, protoreflect.FullName(serviceName))
	if err != nil {
		return nil, fmt.Errorf("resolve service %s: %w", serviceName, err)
	}

	method := serviceDescriptor.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		return nil, fmt.Errorf("method %s not found", fullMethod)
	}
	if method.IsStreamingClient() || method.IsStreamingServer() {
		return nil, fmt.Errorf("streaming methods are not supported")
	}

	return method, nil
}

func (s *ReflectionService) newClient(ctx context.Context) (*grpcreflect.Client, error) {
	conn, err := s.client.Conn()
	if err != nil {
		return nil, err
	}

	return grpcreflect.NewClientAuto(ctx, conn), nil
}

func describeFields(message protoreflect.MessageDescriptor, prefix string) []FieldInfo {
	return describeFieldsWithVisited(message, prefix, map[protoreflect.FullName]struct{}{})
}

func describeFieldsWithVisited(
	message protoreflect.MessageDescriptor,
	prefix string,
	visited map[protoreflect.FullName]struct{},
) []FieldInfo {
	if message == nil {
		return nil
	}

	_, seen := visited[message.FullName()]
	if !seen {
		visited[message.FullName()] = struct{}{}
		defer delete(visited, message.FullName())
	}

	fields := make([]FieldInfo, 0, message.Fields().Len())

	for i := 0; i < message.Fields().Len(); i++ {
		field := message.Fields().Get(i)
		path := string(field.Name())
		if prefix != "" {
			path = prefix + "." + path
		}

		info := FieldInfo{
			Name:       string(field.Name()),
			Label:      strings.ReplaceAll(path, ".", " > "),
			Kind:       field.Kind().String(),
			Path:       path,
			Repeated:   field.Cardinality() == protoreflect.Repeated,
			IsMessage:  field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind,
			IsEnum:     field.Kind() == protoreflect.EnumKind,
			IsMap:      field.IsMap(),
			IsRequired: field.Cardinality() == protoreflect.Required,
		}

		switch {
		case info.IsMessage && field.Message() != nil:
			info.TypeName = string(field.Message().FullName())
		case info.IsEnum && field.Enum() != nil:
			info.TypeName = string(field.Enum().FullName())
		}
		if info.IsMap {
			info.Kind = "map"
			info.TypeName = ""
			info.IsMessage = false
		}

		fields = append(fields, info)

		if info.IsMessage && !field.IsMap() && !seen {
			fields = append(fields, describeFieldsWithVisited(field.Message(), path, visited)...)
		}
	}

	return fields
}

func splitFullMethod(fullMethod string) (string, string, error) {
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	parts := strings.Split(fullMethod, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid method name: %s", fullMethod)
	}

	return parts[0], parts[1], nil
}

func resolveServiceDescriptor(
	client *grpcreflect.Client,
	serviceName protoreflect.FullName,
) (protoreflect.ServiceDescriptor, error) {
	fileDescriptor, err := client.FileContainingSymbol(serviceName)
	if err != nil {
		return nil, err
	}

	descriptor := protoresolve.FindDescriptorByNameInFile(fileDescriptor, serviceName)
	if descriptor == nil {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	serviceDescriptor, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a service", serviceName)
	}

	return serviceDescriptor, nil
}
