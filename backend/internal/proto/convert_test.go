package proto

import "testing"

func TestBytesToProtoJSON_EmptyData(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package empty;

message Msg {
  string str_field = 1;
  int32 int_field = 2;
  bool bool_field = 3;
  bytes bytes_field = 4;
}
`)
	msgDesc, err := ResolveMessageDescriptor(fds, "empty.Msg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}
	reg := NewRegistry()

	// Proto3 omits scalar fields with default values from the wire format.
	defaults, err := reg.ProtoJSONToBytes(msgDesc, []byte(`{"strField":"","intField":0,"boolField":false,"bytesField":""}`))
	if err != nil {
		t.Fatalf("ProtoJSONToBytes failed: %v", err)
	}
	if len(defaults) != 0 {
		t.Fatalf("expected empty encoding for default values, got %x", defaults)
	}

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "nil"},
		{name: "empty slice", data: []byte{}},
		{name: "default values round trip", data: defaults},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reg.BytesToProtoJSON(msgDesc, tc.data)
			if err != nil {
				t.Fatalf("BytesToProtoJSON failed: %v", err)
			}
			if string(got) != "{}" {
				t.Fatalf("expected empty JSON object, got %s", got)
			}
		})
	}

	t.Run("malformed wire data", func(t *testing.T) {
		if _, err := reg.BytesToProtoJSON(msgDesc, []byte{0x0a, 0x01}); err == nil {
			t.Fatal("expected error for truncated string field")
		}
	})
}
