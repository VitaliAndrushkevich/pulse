package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ProtoJSONToBytes converts Proto JSON to binary protobuf given a message descriptor.
// It creates a dynamic message from the descriptor, unmarshals the JSON into it,
// then marshals to binary wire format.
func (r *Registry) ProtoJSONToBytes(msgDesc protoreflect.MessageDescriptor, jsonPayload []byte) ([]byte, error) {
	if msgDesc == nil {
		return nil, fmt.Errorf("message descriptor is nil")
	}
	if len(jsonPayload) == 0 {
		return nil, fmt.Errorf("empty JSON payload")
	}

	msg := dynamicpb.NewMessage(msgDesc)

	unmarshalOpts := protojson.UnmarshalOptions{
		DiscardUnknown: false, // reject unknown fields
	}
	if err := unmarshalOpts.Unmarshal(jsonPayload, msg); err != nil {
		return nil, fmt.Errorf("proto JSON unmarshal failed: %w", err)
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("proto binary marshal failed: %w", err)
	}

	return data, nil
}

// BytesToProtoJSON converts binary protobuf to Proto JSON given a message descriptor.
// It creates a dynamic message from the descriptor, unmarshals the binary data into it,
// then marshals to Proto JSON format.
func (r *Registry) BytesToProtoJSON(msgDesc protoreflect.MessageDescriptor, data []byte) ([]byte, error) {
	if msgDesc == nil {
		return nil, fmt.Errorf("message descriptor is nil")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty binary data")
	}

	msg := dynamicpb.NewMessage(msgDesc)

	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("proto binary unmarshal failed: %w", err)
	}

	marshalOpts := protojson.MarshalOptions{
		EmitUnpopulated: false, // clean output, omit zero-value fields
	}
	jsonBytes, err := marshalOpts.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("proto JSON marshal failed: %w", err)
	}

	return jsonBytes, nil
}

// GenerateTemplate generates a Proto JSON template with zero-value defaults for all fields.
// For scalar fields: 0 for numerics, "" for string, false for bool.
// For enum fields: the first declared enum value name.
// For nested messages: {}.
// For repeated fields: [].
// For map fields: {}.
// For oneof: includes only the first option at its zero value.
func (r *Registry) GenerateTemplate(msgDesc protoreflect.MessageDescriptor) ([]byte, error) {
	if msgDesc == nil {
		return nil, fmt.Errorf("message descriptor is nil")
	}

	msg := dynamicpb.NewMessage(msgDesc)
	populateDefaults(msg, msgDesc)

	marshalOpts := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonBytes, err := marshalOpts.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("template generation failed: %w", err)
	}

	return jsonBytes, nil
}

// populateDefaults sets zero-value defaults on a dynamic message for template generation.
// It handles oneof fields by setting only the first option, and recursively
// populates nested message fields.
func populateDefaults(msg *dynamicpb.Message, msgDesc protoreflect.MessageDescriptor) {
	// Track which oneofs we've already handled so we only set the first option.
	handledOneofs := make(map[protoreflect.Name]bool)

	fields := msgDesc.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)

		// Handle oneof: only include the first field option.
		if oneofDesc := fd.ContainingOneof(); oneofDesc != nil && !oneofDesc.IsSynthetic() {
			if handledOneofs[oneofDesc.Name()] {
				continue
			}
			handledOneofs[oneofDesc.Name()] = true
			// Explicitly set the first oneof field to its zero value so it appears in output.
			if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
				nestedMsg := dynamicpb.NewMessage(fd.Message())
				msg.Set(fd, protoreflect.ValueOfMessage(nestedMsg))
			} else {
				msg.Set(fd, fd.Default())
			}
			continue
		}

		// For map fields, set an empty map (protojson EmitUnpopulated handles this).
		if fd.IsMap() {
			continue // EmitUnpopulated will emit {} for unset map fields
		}

		// For repeated fields, set an empty list (protojson EmitUnpopulated handles this).
		if fd.IsList() {
			continue // EmitUnpopulated will emit [] for unset repeated fields
		}

		// For message fields, create a nested message with defaults.
		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			nestedMsg := dynamicpb.NewMessage(fd.Message())
			msg.Set(fd, protoreflect.ValueOfMessage(nestedMsg))
			continue
		}

		// For scalar fields, the zero value is already the default.
		// EmitUnpopulated: true will emit them.
	}
}
