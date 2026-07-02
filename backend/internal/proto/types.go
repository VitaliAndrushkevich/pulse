package proto

// ProtoSourceMetadata contains extracted information about all services and types in a FileDescriptorSet.
type ProtoSourceMetadata struct {
	Services     []ProtoService `json:"services"`
	MessageTypes []string       `json:"message_types"`
	Filenames    []string       `json:"filenames,omitempty"`
}

// ProtoService represents a gRPC service with its methods.
type ProtoService struct {
	FullName string        `json:"full_name"`
	Methods  []ProtoMethod `json:"methods"`
}

// ProtoMethod represents a single RPC method.
type ProtoMethod struct {
	Name       string `json:"name"`
	FullName   string `json:"full_name"`
	InputType  string `json:"input_type"`
	OutputType string `json:"output_type"`
}

// ProtoField represents a single field in a protobuf message, including nested structure.
type ProtoField struct {
	Name          string       `json:"name"`
	JSONName      string       `json:"json_name"`
	Type          string       `json:"type"`
	Repeated      bool         `json:"repeated"`
	MapKeyType    string       `json:"map_key_type,omitempty"`
	MapValueType  string       `json:"map_value_type,omitempty"`
	EnumValues    []string     `json:"enum_values,omitempty"`
	MessageFields []ProtoField `json:"message_fields,omitempty"`
	Comment       string       `json:"comment,omitempty"`
}
