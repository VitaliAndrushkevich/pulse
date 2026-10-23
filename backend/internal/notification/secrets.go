package notification

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
)

// RotateChannelConfig re-encrypts every secret in a stored channel config.
// Unknown channel types fail closed so key rotation cannot strand their secrets.
func RotateChannelConfig(kind string, raw json.RawMessage, oldKey, newKey []byte) (json.RawMessage, error) {
	if kind == "email" {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, fmt.Errorf("invalid channel configuration")
	}
	rotate := func(value string) (string, error) {
		cipher, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return "", fmt.Errorf("invalid channel ciphertext")
		}
		plain, err := crypto.Decrypt(oldKey, cipher)
		if err != nil {
			return "", fmt.Errorf("cannot decrypt channel secret")
		}
		cipher, err = crypto.Encrypt(newKey, plain)
		if err != nil {
			return "", fmt.Errorf("cannot encrypt channel secret")
		}
		return base64.StdEncoding.EncodeToString(cipher), nil
	}
	if kind == "webhook" {
		var headers []map[string]json.RawMessage
		if len(fields["headers"]) > 0 && json.Unmarshal(fields["headers"], &headers) != nil {
			return nil, fmt.Errorf("invalid webhook headers")
		}
		for _, header := range headers {
			var value string
			if json.Unmarshal(header["value"], &value) != nil {
				return nil, fmt.Errorf("invalid webhook header secret")
			}
			// Older versions stored empty headers without encryption.
			if value == "" {
				cipher, err := crypto.Encrypt(newKey, nil)
				if err != nil {
					return nil, fmt.Errorf("cannot encrypt header secret")
				}
				value = base64.StdEncoding.EncodeToString(cipher)
			} else {
				var err error
				value, err = rotate(value)
				if err != nil {
					return nil, err
				}
			}
			header["value"], _ = json.Marshal(value)
		}
		fields["headers"], _ = json.Marshal(headers)
	} else {
		name := map[string]string{"slack": "webhook_url", "telegram": "bot_token", "matrix": "access_token"}[kind]
		if name == "" {
			return nil, fmt.Errorf("unsupported notification type for rotation")
		}
		var value string
		if json.Unmarshal(fields[name], &value) != nil {
			return nil, fmt.Errorf("invalid channel secret")
		}
		value, err := rotate(value)
		if err != nil {
			return nil, err
		}
		fields[name], _ = json.Marshal(value)
	}
	return json.Marshal(fields)
}
