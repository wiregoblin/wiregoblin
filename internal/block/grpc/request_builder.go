package grpc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Builder creates dynamic protobuf request messages from JSON-compatible input.
type Builder struct{}

// NewRequestBuilder creates a request builder for dynamic protobuf messages.
func NewRequestBuilder() *Builder {
	return &Builder{}
}

// Build fills a dynamic protobuf message using a JSON-compatible request payload.
func (b *Builder) Build(message protoreflect.MessageDescriptor, request any) (*dynamicpb.Message, error) {
	msg := dynamicpb.NewMessage(message)
	if request == nil {
		return msg, nil
	}

	requestObject, err := normalizeRequestObject(request)
	if err != nil {
		return nil, err
	}

	if err := b.fillMessage(msg, requestObject); err != nil {
		return nil, err
	}

	return msg, nil
}

func normalizeRequestObject(request any) (map[string]any, error) {
	switch typed := request.(type) {
	case map[string]any:
		return typed, nil
	case string:
		if typed == "" {
			return map[string]any{}, nil
		}

		var requestObject map[string]any
		if err := json.Unmarshal([]byte(typed), &requestObject); err != nil {
			return nil, fmt.Errorf("request must be a JSON object: %w", err)
		}
		return requestObject, nil
	default:
		return nil, fmt.Errorf("request must be a JSON object, got %T", request)
	}
}

func (b *Builder) fillMessage(msg *dynamicpb.Message, request map[string]any) error {
	for key, raw := range request {
		field := fieldByJSONName(msg.Descriptor(), key)
		if field == nil {
			return fmt.Errorf("%s: field not found", key)
		}

		value, err := buildFieldValue(msg, field, raw)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		msg.Set(field, value)
	}

	return nil
}

func fieldByJSONName(message protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	fields := message.Fields()
	if field := fields.ByName(protoreflect.Name(name)); field != nil {
		return field
	}

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.JSONName() == name {
			return field
		}
	}

	return nil
}

func buildFieldValue(
	parent protoreflect.Message,
	field protoreflect.FieldDescriptor,
	raw any,
) (protoreflect.Value, error) {
	switch {
	case field.IsMap():
		return buildMapValue(parent, field, raw)
	case field.Cardinality() == protoreflect.Repeated:
		return buildListValue(parent, field, raw)
	case field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind:
		nested, ok := raw.(map[string]any)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected object")
		}

		msg := dynamicpb.NewMessage(field.Message())
		builder := NewRequestBuilder()
		if err := builder.fillMessage(msg, nested); err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfMessage(msg), nil
	default:
		return parseScalarValue(field, raw)
	}
}

func buildListValue(
	parent protoreflect.Message,
	field protoreflect.FieldDescriptor,
	raw any,
) (protoreflect.Value, error) {
	items, ok := raw.([]any)
	if !ok {
		return protoreflect.Value{}, fmt.Errorf("expected array")
	}

	list := parent.NewField(field).List()
	for _, item := range items {
		var (
			value protoreflect.Value
			err   error
		)

		if field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind {
			nested, ok := item.(map[string]any)
			if !ok {
				return protoreflect.Value{}, fmt.Errorf("expected array of objects")
			}

			msg := dynamicpb.NewMessage(field.Message())
			builder := NewRequestBuilder()
			if err = builder.fillMessage(msg, nested); err != nil {
				return protoreflect.Value{}, err
			}
			value = protoreflect.ValueOfMessage(msg)
		} else {
			value, err = parseScalarValue(field, item)
			if err != nil {
				return protoreflect.Value{}, err
			}
		}

		list.Append(value)
	}

	return protoreflect.ValueOfList(list), nil
}

func buildMapValue(
	parent protoreflect.Message,
	field protoreflect.FieldDescriptor,
	raw any,
) (protoreflect.Value, error) {
	body, ok := raw.(map[string]any)
	if !ok {
		return protoreflect.Value{}, fmt.Errorf("expected object")
	}

	entryDescriptor := field.Message()
	if entryDescriptor == nil {
		return protoreflect.Value{}, fmt.Errorf("map field has no entry descriptor")
	}

	keyDescriptor := entryDescriptor.Fields().ByName("key")
	valueDescriptor := entryDescriptor.Fields().ByName("value")
	if keyDescriptor == nil || valueDescriptor == nil {
		return protoreflect.Value{}, fmt.Errorf("map entry descriptor is invalid")
	}

	mapValue := parent.NewField(field).Map()
	for key, value := range body {
		keyValue, err := parseScalarValue(keyDescriptor, key)
		if err != nil {
			return protoreflect.Value{}, err
		}

		entryValue, err := buildMapEntryValue(valueDescriptor, value)
		if err != nil {
			return protoreflect.Value{}, err
		}

		mapValue.Set(keyValue.MapKey(), entryValue)
	}

	return protoreflect.ValueOfMap(mapValue), nil
}

func buildMapEntryValue(field protoreflect.FieldDescriptor, raw any) (protoreflect.Value, error) {
	if field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind {
		nested, ok := raw.(map[string]any)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected object")
		}

		msg := dynamicpb.NewMessage(field.Message())
		builder := NewRequestBuilder()
		if err := builder.fillMessage(msg, nested); err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfMessage(msg), nil
	}

	return parseScalarValue(field, raw)
}

func parseScalarValue(field protoreflect.FieldDescriptor, raw any) (protoreflect.Value, error) {
	switch field.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(fmt.Sprint(raw)), nil
	case protoreflect.BoolKind:
		value, err := asBool(raw)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBool(value), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		value, err := asInt64(raw, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		if value < math.MinInt32 || value > math.MaxInt32 {
			return protoreflect.Value{}, fmt.Errorf("value %d overflows int32", value)
		}
		return protoreflect.ValueOfInt32(int32(value)), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		value, err := asInt64(raw, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfInt64(value), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		value, err := asUint64(raw, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		if value > math.MaxUint32 {
			return protoreflect.Value{}, fmt.Errorf("value %d overflows uint32", value)
		}
		return protoreflect.ValueOfUint32(uint32(value)), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		value, err := asUint64(raw, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfUint64(value), nil
	case protoreflect.FloatKind:
		value, err := asFloat64(raw, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat32(float32(value)), nil
	case protoreflect.DoubleKind:
		value, err := asFloat64(raw, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat64(value), nil
	case protoreflect.BytesKind:
		value, err := asBytes(raw)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBytes(value), nil
	case protoreflect.EnumKind:
		enumValue, err := parseEnumValue(field.Enum(), raw)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfEnum(enumValue), nil
	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind %s", field.Kind())
	}
}

func parseEnumValue(enum protoreflect.EnumDescriptor, raw any) (protoreflect.EnumNumber, error) {
	switch typed := raw.(type) {
	case string:
		if value := enum.Values().ByName(protoreflect.Name(typed)); value != nil {
			return value.Number(), nil
		}

		number, err := strconv.ParseInt(typed, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid enum value %q", typed)
		}

		if number < math.MinInt32 || number > math.MaxInt32 {
			return 0, fmt.Errorf("enum value %d overflows int32", number)
		}
		return protoreflect.EnumNumber(int32(number)), nil
	default:
		number, err := asInt64(raw, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid enum value %v", raw)
		}
		if number < math.MinInt32 || number > math.MaxInt32 {
			return 0, fmt.Errorf("enum value %d overflows int32", number)
		}
		return protoreflect.EnumNumber(int32(number)), nil
	}
}

func asBool(raw any) (bool, error) {
	switch typed := raw.(type) {
	case bool:
		return typed, nil
	case string:
		return strconv.ParseBool(typed)
	default:
		return false, fmt.Errorf("expected bool, got %T", raw)
	}
}

func asInt64(raw any, bits int) (int64, error) {
	switch typed := raw.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, fmt.Errorf("value %d overflows int64", typed)
		}
		return int64(typed), nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		if typed > math.MaxInt64 {
			return 0, fmt.Errorf("value %d overflows int64", typed)
		}
		return int64(typed), nil
	case float32:
		return floatToInt64(float64(typed), bits)
	case float64:
		return floatToInt64(typed, bits)
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(typed, 10, bits)
	default:
		return 0, fmt.Errorf("expected integer, got %T", raw)
	}
}

func asUint64(raw any, bits int) (uint64, error) {
	switch typed := raw.(type) {
	case int:
		if typed < 0 {
			return 0, fmt.Errorf("value %d must be non-negative", typed)
		}
		return uint64(typed), nil
	case int8:
		if typed < 0 {
			return 0, fmt.Errorf("value %d must be non-negative", typed)
		}
		return uint64(typed), nil
	case int16:
		if typed < 0 {
			return 0, fmt.Errorf("value %d must be non-negative", typed)
		}
		return uint64(typed), nil
	case int32:
		if typed < 0 {
			return 0, fmt.Errorf("value %d must be non-negative", typed)
		}
		return uint64(typed), nil
	case int64:
		if typed < 0 {
			return 0, fmt.Errorf("value %d must be non-negative", typed)
		}
		return uint64(typed), nil
	case uint:
		return uint64(typed), nil
	case uint8:
		return uint64(typed), nil
	case uint16:
		return uint64(typed), nil
	case uint32:
		return uint64(typed), nil
	case uint64:
		return typed, nil
	case float32:
		return floatToUint64(float64(typed), bits)
	case float64:
		return floatToUint64(typed, bits)
	case json.Number:
		value, err := strconv.ParseUint(string(typed), 10, bits)
		if err != nil {
			return 0, err
		}
		return value, nil
	case string:
		return strconv.ParseUint(typed, 10, bits)
	default:
		return 0, fmt.Errorf("expected unsigned integer, got %T", raw)
	}
}

func asBytes(raw any) ([]byte, error) {
	switch typed := raw.(type) {
	case []byte:
		return append([]byte(nil), typed...), nil
	case string:
		if strings.HasPrefix(typed, "base64:") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(typed, "base64:"))
			if err != nil {
				return nil, fmt.Errorf("decode base64 bytes: %w", err)
			}
			return decoded, nil
		}
		return []byte(typed), nil
	default:
		return nil, fmt.Errorf("expected bytes or string, got %T", raw)
	}
}

func asFloat64(raw any, bits int) (float64, error) {
	switch typed := raw.(type) {
	case float32:
		return float64(typed), nil
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int8:
		return float64(typed), nil
	case int16:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case uint:
		return float64(typed), nil
	case uint8:
		return float64(typed), nil
	case uint16:
		return float64(typed), nil
	case uint32:
		return float64(typed), nil
	case uint64:
		return float64(typed), nil
	case json.Number:
		return typed.Float64()
	case string:
		return strconv.ParseFloat(typed, bits)
	default:
		return 0, fmt.Errorf("expected number, got %T", raw)
	}
}

func floatToInt64(value float64, bits int) (int64, error) {
	if math.Trunc(value) != value {
		return 0, fmt.Errorf("expected integer, got %v", value)
	}
	return strconv.ParseInt(strconv.FormatFloat(value, 'f', 0, 64), 10, bits)
}

func floatToUint64(value float64, bits int) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("value %v must be non-negative", value)
	}
	if math.Trunc(value) != value {
		return 0, fmt.Errorf("expected integer, got %v", value)
	}
	return strconv.ParseUint(strconv.FormatFloat(value, 'f', 0, 64), 10, bits)
}
