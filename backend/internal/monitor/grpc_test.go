package monitor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
	"pgregory.net/rapid"
)

// Feature: grpc-monitor, Property 2: Service method format validation
//
// For any string value of service_method, the checker SHALL accept it (return nil)
// if and only if it contains exactly one `/` separator, both the service segment
// (before `/`) and method segment (after `/`) are non-empty, and the combined
// length is ≤ 512 characters. All other non-whitespace-only strings SHALL cause
// a validation error.
//
// **Validates: Requirements 3.2, 3.3**
func TestProperty_ServiceMethodFormatValidation(t *testing.T) {
	// Sub-test: valid service methods should be accepted
	t.Run("valid_service_methods_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate service segment: 1-255 chars, no '/'
			service := rapid.StringMatching(`[a-zA-Z0-9._\-]{1,255}`).Draw(t, "service")
			// Generate method segment: 1-255 chars, no '/'
			method := rapid.StringMatching(`[a-zA-Z0-9._\-]{1,255}`).Draw(t, "method")

			input := service + "/" + method

			// Combined length must be ≤ 512 (service + "/" + method)
			if len(input) > 512 {
				t.Skip("generated input exceeds 512 chars, skipping")
			}

			err := validateServiceMethod(input)
			if err != nil {
				t.Fatalf("expected valid service method %q to be accepted, got error: %v", input, err)
			}
		})
	})

	// Sub-test: strings with zero slashes should be rejected
	t.Run("no_slash_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a non-whitespace string with no '/' characters
			s := rapid.StringMatching(`[a-zA-Z0-9._\-]{1,100}`).Draw(t, "no_slash")

			// Verify it has no slash
			if strings.Contains(s, "/") {
				t.Skip("generated string contains slash")
			}

			err := validateServiceMethod(s)
			if err == nil {
				t.Fatalf("expected string without '/' %q to be rejected, got nil error", s)
			}
		})
	})

	// Sub-test: strings with 2+ slashes should be rejected
	t.Run("multiple_slashes_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate segments and join with multiple slashes
			numSlashes := rapid.IntRange(2, 5).Draw(t, "numSlashes")
			segments := make([]string, numSlashes+1)
			for i := range segments {
				segments[i] = rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "segment")
			}
			input := strings.Join(segments, "/")

			err := validateServiceMethod(input)
			if err == nil {
				t.Fatalf("expected string with %d slashes %q to be rejected, got nil error", numSlashes, input)
			}
		})
	})

	// Sub-test: empty service segment ("/method") should be rejected
	t.Run("empty_service_segment_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			method := rapid.StringMatching(`[a-zA-Z0-9._\-]{1,50}`).Draw(t, "method")
			input := "/" + method

			err := validateServiceMethod(input)
			if err == nil {
				t.Fatalf("expected empty service segment %q to be rejected, got nil error", input)
			}
		})
	})

	// Sub-test: empty method segment ("service/") should be rejected
	t.Run("empty_method_segment_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			service := rapid.StringMatching(`[a-zA-Z0-9._\-]{1,50}`).Draw(t, "service")
			input := service + "/"

			err := validateServiceMethod(input)
			if err == nil {
				t.Fatalf("expected empty method segment %q to be rejected, got nil error", input)
			}
		})
	})

	// Sub-test: combined length > 512 should be rejected
	t.Run("exceeds_max_length_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate strings that ensure combined > 512
			serviceLen := rapid.IntRange(250, 300).Draw(t, "serviceLen")
			methodLen := rapid.IntRange(250, 300).Draw(t, "methodLen")

			service := strings.Repeat("a", serviceLen)
			method := strings.Repeat("b", methodLen)
			input := service + "/" + method

			// Verify it's actually > 512
			if len(input) <= 512 {
				t.Skip("generated input does not exceed 512 chars")
			}

			err := validateServiceMethod(input)
			if err == nil {
				t.Fatalf("expected service method with length %d > 512 to be rejected, got nil error", len(input))
			}
		})
	})
}

// Feature: grpc-monitor, Property 3: Whitespace-only service method falls back to default
//
// For any string composed entirely of whitespace characters (spaces, tabs, newlines),
// when provided as service_method, the checker SHALL treat it as unset and return nil
// from validation (no error), allowing fallback to default grpc.health.v1.Health/Check.
//
// **Validates: Requirements 3.5**
func TestProperty_WhitespaceServiceMethodFallback(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random whitespace-only string composed of space, tab, newline, carriage return.
		wsChars := []rune{' ', '\t', '\n', '\r'}
		length := rapid.IntRange(0, 100).Draw(t, "length")
		runes := make([]rune, length)
		for i := range runes {
			runes[i] = rapid.SampledFrom(wsChars).Draw(t, "char")
		}
		whitespaceStr := string(runes)

		// Whitespace-only strings should be treated as unset: validateServiceMethod returns nil.
		err := validateServiceMethod(whitespaceStr)
		if err != nil {
			t.Fatalf("expected nil error for whitespace-only string %q, got: %v", whitespaceStr, err)
		}
	})
}

// Feature: grpc-monitor, Property 4: Invalid TLS mode rejection
//
// For any string value of tls_mode that is not one of "plaintext", "tls",
// or "tls_skip_verify", the checker SHALL report a validation error.
//
// **Validates: Requirements 4.6**
func TestProperty_InvalidTLSModeRejection(t *testing.T) {
	validModes := map[string]bool{
		"plaintext":       true,
		"tls":             true,
		"tls_skip_verify": true,
	}

	rapid.Check(t, func(t *rapid.T) {
		// Generate a random string and filter out the three valid values.
		mode := rapid.String().Filter(func(s string) bool {
			return !validModes[s]
		}).Draw(t, "invalidTLSMode")

		err := validateTLSMode(mode)
		if err == nil {
			t.Fatalf("expected validation error for invalid TLS mode %q, got nil", mode)
		}
	})

	// Also verify that the three valid values pass validation.
	for _, validMode := range []string{"plaintext", "tls", "tls_skip_verify"} {
		if err := validateTLSMode(validMode); err != nil {
			t.Errorf("expected no error for valid TLS mode %q, got: %v", validMode, err)
		}
	}
}

// Feature: grpc-monitor, Property 10: Invalid base64 payload rejection
//
// For any string that is not valid standard base64 (RFC 4648 §4), when provided
// as request_payload, the checker SHALL report an error indicating payload decode failure.
//
// **Validates: Requirements 8.3**
func TestProperty_InvalidBase64PayloadRejection(t *testing.T) {
	// Characters that are definitively NOT in the standard base64 alphabet
	// and are rejected by Go's base64.StdEncoding.DecodeString.
	// Note: \n and \r are silently ignored by Go's decoder, so we exclude them.
	invalidChars := "!@#$%^&*()~`{}[]|\\:;\"'<>, \t?"

	// Sub-test: strings containing invalid base64 characters should be rejected
	t.Run("invalid_base64_chars_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a base string of valid-looking characters
			base := rapid.StringMatching(`[A-Za-z0-9+/=]{0,99}`).Draw(t, "base")

			// Ensure we have some content
			if len(base) == 0 {
				base = "AA"
			}

			// Inject at least one invalid character at a random position
			invalidIdx := rapid.IntRange(0, len(invalidChars)-1).Draw(t, "invalidCharIdx")
			invalidChar := string(invalidChars[invalidIdx])

			// Place the invalid char at a random position in the string
			pos := rapid.IntRange(0, len(base)).Draw(t, "insertPos")
			input := base[:pos] + invalidChar + base[pos:]

			_, err := validateRequestPayload(input)
			if err == nil {
				t.Fatalf("expected invalid base64 string %q to be rejected, got nil error", input)
			}
		})
	})

	// Sub-test: pure invalid character strings should be rejected
	t.Run("pure_invalid_chars_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate strings composed entirely of non-base64 characters
			length := rapid.IntRange(1, 50).Draw(t, "length")
			chars := make([]byte, length)
			for i := range chars {
				idx := rapid.IntRange(0, len(invalidChars)-1).Draw(t, "charIdx")
				chars[i] = invalidChars[idx]
			}
			input := string(chars)

			_, err := validateRequestPayload(input)
			if err == nil {
				t.Fatalf("expected pure invalid base64 string %q to be rejected, got nil error", input)
			}
		})
	})

	// Sanity check: valid base64 string should be accepted
	t.Run("valid_base64_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate random bytes and encode them as valid base64
			dataLen := rapid.IntRange(0, 1000).Draw(t, "dataLen")
			data := make([]byte, dataLen)
			for i := range data {
				data[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
			}

			encoded := base64.StdEncoding.EncodeToString(data)

			decoded, err := validateRequestPayload(encoded)
			if err != nil {
				t.Fatalf("expected valid base64 string to be accepted, got error: %v", err)
			}
			if len(data) > 0 && len(decoded) != len(data) {
				t.Fatalf("decoded length mismatch: expected %d, got %d", len(data), len(decoded))
			}
		})
	})
}

// Feature: grpc-monitor, Property 8: Expected statuses validation
//
// For any list provided as expected_statuses, the checker SHALL accept it if
// and only if every element is an integer in the range 0–16 inclusive and the
// list contains at most 17 entries. Any list containing a value outside 0–16
// or with more than 17 entries SHALL cause a validation error.
//
// **Validates: Requirements 7.3, 7.4**
func TestProperty_ExpectedStatusesValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random integer slice with values in [-10, 30] and length 0–25.
		length := rapid.IntRange(0, 25).Draw(t, "length")
		statuses := make([]int, length)
		for i := range statuses {
			statuses[i] = rapid.IntRange(-10, 30).Draw(t, fmt.Sprintf("status[%d]", i))
		}

		// Compute expected validity:
		// Valid iff all values in [0, 16] AND length ≤ 17.
		shouldBeValid := true
		if len(statuses) > 17 {
			shouldBeValid = false
		}
		for _, code := range statuses {
			if code < 0 || code > 16 {
				shouldBeValid = false
				break
			}
		}

		err := validateExpectedStatuses(statuses)

		if shouldBeValid && err != nil {
			t.Fatalf("expected valid statuses %v to be accepted, got error: %v", statuses, err)
		}
		if !shouldBeValid && err == nil {
			t.Fatalf("expected invalid statuses %v to be rejected, got nil error (len=%d)", statuses, len(statuses))
		}
	})
}

// Feature: grpc-monitor, Property 7: Metadata key validation
//
// For any metadata key string, the checker SHALL accept it if and only if:
// (a) it contains only lowercase ASCII letters, digits, hyphens, underscores, and dots;
// (b) it does not start with the prefix "grpc-";
// (c) its length is ≤ 128 characters;
// (d) if the key ends with "-bin", its corresponding value is valid base64.
// The total number of metadata entries must be ≤ 20 and each value must be ≤ 4096 characters.
//
// **Validates: Requirements 6.2, 6.3, 6.4**
func TestProperty_MetadataKeyValidation(t *testing.T) {
	// Sub-property 1: Valid metadata maps are accepted.
	t.Run("valid_metadata_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			numEntries := rapid.IntRange(0, 20).Draw(t, "numEntries")
			md := make(map[string]string, numEntries)

			for i := 0; i < numEntries; i++ {
				key := validMetadataKeyGen(true).Draw(t, "key")
				// Append index-based suffix to avoid duplicate keys in the map.
				key = key + strings.Repeat("a", i%5)
				// Truncate to 128 chars max.
				if len(key) > 128 {
					key = key[:128]
				}
				// Ensure key doesn't accidentally start with grpc-.
				if strings.HasPrefix(key, "grpc-") {
					key = "x" + key[5:]
				}
				// Ensure the key is non-empty and only valid chars after manipulation.
				if key == "" || !metadataKeyRegex.MatchString(key) {
					key = "valid-key"
				}

				var value string
				if strings.HasSuffix(key, "-bin") {
					// Generate valid base64 value.
					rawBytes := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "binValue")
					value = base64.StdEncoding.EncodeToString(rawBytes)
				} else {
					// Generate a value ≤ 4096 chars.
					valueLen := rapid.IntRange(0, 100).Draw(t, "valueLen")
					value = strings.Repeat("v", valueLen)
				}

				md[key] = value
			}

			err := validateMetadata(md)
			if err != nil {
				t.Fatalf("expected valid metadata to be accepted, got error: %v\nmetadata: %v", err, md)
			}
		})
	})

	// Sub-property 2: Too many entries are rejected.
	t.Run("too_many_entries_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			numEntries := rapid.IntRange(21, 30).Draw(t, "numEntries")
			md := make(map[string]string, numEntries)

			for i := 0; i < numEntries; i++ {
				// Use a unique key for each entry to avoid dedup.
				key := "key-" + strings.Repeat("a", i+1)
				if len(key) > 128 {
					key = key[:128]
				}
				md[key] = "value"
			}

			// Ensure we actually have > 20 entries.
			if len(md) <= 20 {
				return
			}

			err := validateMetadata(md)
			if err == nil {
				t.Fatalf("expected error for %d metadata entries, got nil", len(md))
			}
		})
	})

	// Sub-property 3: Key too long is rejected.
	t.Run("key_too_long_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			keyLen := rapid.IntRange(129, 256).Draw(t, "keyLen")
			// Build a key with only valid characters but exceeding 128 chars.
			key := strings.Repeat("a", keyLen)

			md := map[string]string{key: "value"}
			err := validateMetadata(md)
			if err == nil {
				t.Fatalf("expected error for key length %d, got nil", keyLen)
			}
		})
	})

	// Sub-property 4: Invalid key characters are rejected.
	t.Run("invalid_key_characters_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a key with at least one invalid character.
			invalidChar := rapid.SampledFrom([]rune{
				'A', 'B', 'Z', '!', '@', '#', '$', '%', '^', '&', '*',
				'(', ')', '+', '=', ' ', '/', '\\', '~', '`', '"', '\'',
			}).Draw(t, "invalidChar")

			prefix := rapid.StringMatching(`[a-z0-9]{0,5}`).Draw(t, "prefix")
			suffix := rapid.StringMatching(`[a-z0-9]{0,5}`).Draw(t, "suffix")
			key := prefix + string(invalidChar) + suffix

			md := map[string]string{key: "value"}
			err := validateMetadata(md)
			if err == nil {
				t.Fatalf("expected error for key %q with invalid characters, got nil", key)
			}
		})
	})

	// Sub-property 5: grpc- prefix is rejected.
	t.Run("grpc_prefix_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a key that starts with "grpc-" followed by valid characters.
			suffix := rapid.StringMatching(`[a-z0-9._-]{1,20}`).Draw(t, "suffix")
			key := "grpc-" + suffix

			md := map[string]string{key: "value"}
			err := validateMetadata(md)
			if err == nil {
				t.Fatalf("expected error for key %q with grpc- prefix, got nil", key)
			}
		})
	})

	// Sub-property 6: -bin key with invalid base64 is rejected.
	t.Run("bin_key_invalid_base64_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a valid key ending in "-bin".
			prefix := rapid.StringMatching(`[a-z][a-z0-9]{0,10}`).Draw(t, "prefix")
			key := prefix + "-bin"

			// Generate a value that is NOT valid base64.
			invalidValue := rapid.StringMatching(`[!@#$%^&*()]{1,20}`).Draw(t, "invalidValue")

			md := map[string]string{key: invalidValue}
			err := validateMetadata(md)
			if err == nil {
				t.Fatalf("expected error for -bin key %q with invalid base64 value %q, got nil", key, invalidValue)
			}
		})
	})

	// Sub-property 7: Value too long is rejected.
	t.Run("value_too_long_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			valueLen := rapid.IntRange(4097, 5000).Draw(t, "valueLen")
			value := strings.Repeat("x", valueLen)

			md := map[string]string{"valid-key": value}
			err := validateMetadata(md)
			if err == nil {
				t.Fatalf("expected error for value length %d, got nil", valueLen)
			}
		})
	})
}

// Feature: grpc-monitor, Property 9: Request payload round-trip
//
// For any byte sequence of length ≤ 1,048,576 bytes, base64-encoding it and
// providing the result as request_payload SHALL cause the checker to send those
// exact decoded bytes as the gRPC request body to the server.
//
// **Validates: Requirements 8.1**
func TestProperty_RequestPayloadRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random byte slice of length 0 to 10000 (practical for speed).
		dataLen := rapid.IntRange(0, 10000).Draw(t, "dataLen")
		originalBytes := make([]byte, dataLen)
		for i := range originalBytes {
			originalBytes[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}

		// Base64-encode the bytes.
		encoded := base64.StdEncoding.EncodeToString(originalBytes)

		// Set up a channel to capture the received payload.
		receivedCh := make(chan []byte, 1)

		// Start a mock gRPC server with raw codec that captures the request bytes.
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer lis.Close()

		// Register the raw codec globally so the server understands content-type application/grpc+raw.
		encoding.RegisterCodec(rawCodec{})

		srv := grpc.NewServer(
			grpc.ForceServerCodec(rawCodec{}),
			grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
				var msg []byte
				if err := stream.RecvMsg(&msg); err != nil {
					return err
				}
				receivedCh <- msg
				return stream.SendMsg([]byte{})
			}),
		)
		defer srv.Stop()

		go func() {
			_ = srv.Serve(lis)
		}()

		// Build settings with plaintext TLS mode and the request payload.
		settings := GRPCSettings{
			TLSMode:        "plaintext",
			ServiceMethod:  "test.Service/Method",
			RequestPayload: encoded,
			ExpectedStatuses: []int{0, 12}, // Accept OK or UNIMPLEMENTED
		}
		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			t.Fatalf("failed to marshal settings: %v", err)
		}

		// Run the checker.
		checker := &GRPCChecker{}
		ctx := context.Background()
		_ = checker.Check(ctx, lis.Addr().String(), settingsJSON)

		// Read the received payload from the channel.
		select {
		case received := <-receivedCh:
			// Assert that the received bytes match the original bytes.
			if len(originalBytes) == 0 && len(received) == 0 {
				// Both empty - pass.
				return
			}
			if len(received) != len(originalBytes) {
				t.Fatalf("payload length mismatch: sent %d bytes, server received %d bytes",
					len(originalBytes), len(received))
			}
			for i := range originalBytes {
				if received[i] != originalBytes[i] {
					t.Fatalf("payload mismatch at byte %d: sent 0x%02x, received 0x%02x",
						i, originalBytes[i], received[i])
				}
			}
		default:
			t.Fatalf("server did not receive any payload (channel empty)")
		}
	})
}

// validMetadataKeyGen generates valid metadata keys: lowercase alphanumeric + hyphens,
// underscores, and dots. Length 1-20 chars for practical test generation.
// If allowBinSuffix is true, may generate keys ending in "-bin".
func validMetadataKeyGen(allowBinSuffix bool) *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		if allowBinSuffix && rapid.Bool().Draw(t, "useBinSuffix") {
			prefix := rapid.StringMatching(`[a-z][a-z0-9._-]{0,15}`).Draw(t, "binPrefix")
			return prefix + "-bin"
		}
		// Generate key from valid character set, ensuring no grpc- prefix.
		key := rapid.StringMatching(`[a-z][a-z0-9._-]{0,19}`).Draw(t, "key")
		if strings.HasPrefix(key, "grpc-") {
			key = "x" + key[1:]
		}
		return key
	})
}


// startMockGRPCServer starts a mock gRPC server that returns a configurable status code
// for any incoming unary RPC. It properly receives the request and sends an empty response
// before returning the status code to avoid "cardinality violation" errors.
func startMockGRPCServer(t *testing.T, returnCode codes.Code) (addr string, stop func()) {
	t.Helper()
	encoding.RegisterCodec(rawCodec{})
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return status.Error(returnCode, "mock response")
		}),
	)
	go srv.Serve(lis)
	return lis.Addr().String(), srv.Stop
}

// Feature: grpc-monitor, Property 1: Status code determines up/down state
//
// For any gRPC status code returned by the server and for any non-empty
// expected_statuses list containing valid codes (0–16), the checker SHALL report
// state "up" if and only if the returned code is present in the expected list;
// otherwise it SHALL report state "down".
//
// **Validates: Requirements 2.4, 2.6, 7.1**
func TestProperty_StatusCodeDeterminesState(t *testing.T) {
	checker := &GRPCChecker{}

	rapid.Check(t, func(rt *rapid.T) {
		// Draw a random gRPC status code (0–16).
		code := rapid.IntRange(0, 16).Draw(rt, "statusCode")

		// Draw a random non-empty subset of [0..16] as expected statuses.
		// Generate by deciding inclusion of each code independently, ensuring at least one.
		allCodes := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		perm := rapid.Permutation(allCodes).Draw(rt, "perm")
		subsetSize := rapid.IntRange(1, 17).Draw(rt, "subsetSize")
		expectedStatuses := perm[:subsetSize]

		// Determine if the code is in the expected list.
		codeInList := false
		for _, ec := range expectedStatuses {
			if ec == code {
				codeInList = true
				break
			}
		}

		// Start a mock gRPC server that returns the drawn status code.
		addr, stop := startMockGRPCServer(t, codes.Code(code))
		defer stop()

		// Build settings JSON with plaintext TLS mode and the expected statuses.
		settings := GRPCSettings{
			TLSMode:          "plaintext",
			ExpectedStatuses: expectedStatuses,
		}
		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			rt.Fatalf("failed to marshal settings: %v", err)
		}

		// Execute the check.
		ctx := context.Background()
		result := checker.Check(ctx, addr, settingsJSON)

		// Verify: state is "up" iff code is in expected list, "down" otherwise.
		if codeInList {
			if result.State != "up" {
				rt.Fatalf("expected state 'up' for code %d in expected list %v, got %q (error: %s)",
					code, expectedStatuses, result.State, result.Error)
			}
		} else {
			if result.State != "down" {
				rt.Fatalf("expected state 'down' for code %d not in expected list %v, got %q",
					code, expectedStatuses, result.State)
			}
		}
	})
}

// generateSelfSignedCert creates a self-signed TLS certificate with the given NotAfter time.
// The certificate is valid for "localhost" DNS name.
func generateSelfSignedCert(t *testing.T, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"localhost"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to load key pair: %v", err)
	}
	return cert
}

// startTLSMockGRPCServer starts a TLS gRPC server with the given certificate
// that accepts any RPC and returns OK.
func startTLSMockGRPCServer(t *testing.T, cert tls.Certificate) (addr string, stop func()) {
	t.Helper()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return nil // return OK
		}),
	)
	go srv.Serve(lis)
	return lis.Addr().String(), srv.Stop
}

// startDelayedMockGRPCServer starts a mock gRPC server that delays for the given
// duration before responding with the specified status code.
func startDelayedMockGRPCServer(t *testing.T, delay time.Duration, returnCode codes.Code) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
		select {
		case <-time.After(delay):
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "context done")
		}
		return status.Error(returnCode, "mock response")
	}))
	go srv.Serve(lis)
	return lis.Addr().String(), srv.Stop
}

// startUnaryMockGRPCServer starts a plaintext mock gRPC server that properly handles
// unary RPCs by receiving the request and sending an empty response, then returning
// the given status code. This avoids the "cardinality violation" error.
func startUnaryMockGRPCServer(t *testing.T, returnCode codes.Code) (addr string, stop func()) {
	t.Helper()
	// Register the raw codec so the server understands content-type application/grpc+raw.
	encoding.RegisterCodec(rawCodec{})
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return status.Error(returnCode, "mock response")
		}),
	)
	go srv.Serve(lis)
	return lis.Addr().String(), srv.Stop
}

// startUnaryTLSMockGRPCServer starts a TLS-enabled mock gRPC server that properly
// handles unary RPCs with the given certificate and returns the specified status code.
func startUnaryTLSMockGRPCServer(t *testing.T, cert tls.Certificate, returnCode codes.Code) (addr string, stop func()) {
	t.Helper()
	// Register the raw codec so the server understands content-type application/grpc+raw.
	encoding.RegisterCodec(rawCodec{})
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return status.Error(returnCode, "mock response")
		}),
	)
	go srv.Serve(lis)
	return lis.Addr().String(), srv.Stop
}

// Feature: grpc-monitor, Property 5: SSL days remaining calculation
//
// For any certificate NotAfter timestamp in the future or past, the checker SHALL
// compute ssl_days_remaining as the number of hours between now and NotAfter divided
// by 24, truncated toward zero (producing negative values for already-expired certificates).
//
// **Validates: Requirements 5.1, 5.2**
func TestProperty_SSLDaysRemainingCalculation(t *testing.T) {
	checker := &GRPCChecker{}

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random day offset from -365 to +365.
		dayOffset := rapid.IntRange(-365, 365).Draw(rt, "dayOffset")

		// Compute the NotAfter time as now + dayOffset days.
		notAfter := time.Now().Add(time.Duration(dayOffset) * 24 * time.Hour)

		// Generate a self-signed certificate with this NotAfter.
		cert := generateSelfSignedCert(t, notAfter)

		// Start a TLS gRPC server with this certificate.
		addr, stop := startTLSMockGRPCServer(t, cert)
		defer stop()

		// Build settings with tls_skip_verify to bypass CA verification.
		settings := GRPCSettings{
			TLSMode:          "tls_skip_verify",
			ExpectedStatuses: []int{0},
		}
		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			rt.Fatalf("failed to marshal settings: %v", err)
		}

		// Execute the check.
		ctx := context.Background()
		result := checker.Check(ctx, addr, settingsJSON)

		// The check should succeed (status OK is in expected list).
		if result.State != "up" {
			rt.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
		}

		// SSLDaysRemaining must be set.
		if result.SSLDaysRemaining == nil {
			rt.Fatalf("expected ssl_days_remaining to be set, got nil")
		}

		// Compute expected value: hours until NotAfter / 24, truncated toward zero.
		// Allow ±1 day tolerance due to timing between cert creation and check execution.
		expectedDays := int32(time.Until(notAfter).Hours() / 24)
		actual := *result.SSLDaysRemaining

		diff := actual - expectedDays
		if diff < -1 || diff > 1 {
			rt.Fatalf("ssl_days_remaining mismatch: got %d, expected ~%d (dayOffset=%d, notAfter=%v, diff=%d)",
				actual, expectedDays, dayOffset, notAfter, diff)
		}
	})
}

// Feature: grpc-monitor, Property 6: SSL expiry threshold triggers down state
//
// For any ssl_expiry_threshold value in range 1–3650 and for any certificate
// with computed days_remaining, the checker SHALL report state "down" if and
// only if days_remaining ≤ ssl_expiry_threshold.
//
// **Validates: Requirements 5.3**
func TestProperty_SSLExpiryThresholdTriggersDown(t *testing.T) {
	checker := &GRPCChecker{}

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random daysOffset from -30 to 3650 (certificate's days until expiry).
		daysOffset := rapid.IntRange(-30, 3650).Draw(rt, "daysOffset")

		// Generate random threshold from 1 to 3650.
		threshold := rapid.IntRange(1, 3650).Draw(rt, "threshold")

		// Create a self-signed TLS cert with NotAfter = now + daysOffset days.
		notAfter := time.Now().Add(time.Duration(daysOffset) * 24 * time.Hour)
		cert := generateSelfSignedCert(t, notAfter)

		// Start a TLS gRPC server with that cert.
		addr, stop := startTLSMockGRPCServer(t, cert)
		defer stop()

		// Build settings with tls_skip_verify and the ssl_expiry_threshold.
		settings := GRPCSettings{
			TLSMode:            "tls_skip_verify",
			SSLExpiryThreshold: threshold,
			ExpectedStatuses:   []int{0}, // OK
		}
		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			rt.Fatalf("failed to marshal settings: %v", err)
		}

		// Execute the check.
		ctx := context.Background()
		result := checker.Check(ctx, addr, settingsJSON)

		// Skip boundary cases where abs(daysOffset - threshold) <= 1 due to timing.
		diff := daysOffset - threshold
		if diff >= -1 && diff <= 1 {
			rt.Skip("skipping boundary case where daysOffset ≈ threshold (timing-sensitive)")
		}

		// Verify: state is "down" iff daysOffset ≤ threshold.
		if daysOffset <= threshold {
			if result.State != "down" {
				rt.Fatalf("expected state 'down' for daysOffset=%d <= threshold=%d, got %q (error: %s)",
					daysOffset, threshold, result.State, result.Error)
			}
		} else {
			if result.State != "up" {
				rt.Fatalf("expected state 'up' for daysOffset=%d > threshold=%d, got %q (error: %s)",
					daysOffset, threshold, result.State, result.Error)
			}
		}
	})
}


// =============================================================================
// Unit Tests for GRPCChecker (Task 4.7)
// =============================================================================

// TestGRPCChecker_DefaultSettings verifies that empty/nil settings JSON results
// in the correct defaults: tls_mode="tls", ExpectedStatuses=[0], ServiceMethod="".
//
// Validates: Requirements 2.2, 4.4, 7.2
func TestGRPCChecker_DefaultSettings(t *testing.T) {
	t.Run("nil_settings", func(t *testing.T) {
		s := parseGRPCSettings(nil)
		if s.TLSMode != "tls" {
			t.Fatalf("expected tls_mode='tls', got %q", s.TLSMode)
		}
		if len(s.ExpectedStatuses) != 1 || s.ExpectedStatuses[0] != 0 {
			t.Fatalf("expected ExpectedStatuses=[0], got %v", s.ExpectedStatuses)
		}
		if s.ServiceMethod != "" {
			t.Fatalf("expected ServiceMethod='', got %q", s.ServiceMethod)
		}
	})

	t.Run("empty_json_object", func(t *testing.T) {
		s := parseGRPCSettings(json.RawMessage(`{}`))
		if s.TLSMode != "tls" {
			t.Fatalf("expected tls_mode='tls', got %q", s.TLSMode)
		}
		if len(s.ExpectedStatuses) != 1 || s.ExpectedStatuses[0] != 0 {
			t.Fatalf("expected ExpectedStatuses=[0], got %v", s.ExpectedStatuses)
		}
		if s.ServiceMethod != "" {
			t.Fatalf("expected ServiceMethod='', got %q", s.ServiceMethod)
		}
	})

	t.Run("empty_bytes", func(t *testing.T) {
		s := parseGRPCSettings(json.RawMessage(``))
		if s.TLSMode != "tls" {
			t.Fatalf("expected tls_mode='tls', got %q", s.TLSMode)
		}
		if len(s.ExpectedStatuses) != 1 || s.ExpectedStatuses[0] != 0 {
			t.Fatalf("expected ExpectedStatuses=[0], got %v", s.ExpectedStatuses)
		}
		if s.ServiceMethod != "" {
			t.Fatalf("expected ServiceMethod='', got %q", s.ServiceMethod)
		}
	})
}

// TestGRPCChecker_SettingsRoundTrip verifies that marshaling GRPCSettings to JSON
// and unmarshaling back preserves all fields.
//
// Validates: Requirements 5.5
func TestGRPCChecker_SettingsRoundTrip(t *testing.T) {
	original := GRPCSettings{
		ServiceMethod:      "my.package.Service/HealthCheck",
		TLSMode:            "tls_skip_verify",
		SSLExpiryThreshold: 30,
		Metadata: map[string]string{
			"authorization": "Bearer token123",
			"x-request-id":  "abc-def-123",
		},
		ExpectedStatuses: []int{0, 12, 14},
		RequestPayload:   base64.StdEncoding.EncodeToString([]byte("hello")),
	}

	// Marshal to JSON.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back.
	var restored GRPCSettings
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify all fields.
	if restored.ServiceMethod != original.ServiceMethod {
		t.Errorf("ServiceMethod: got %q, want %q", restored.ServiceMethod, original.ServiceMethod)
	}
	if restored.TLSMode != original.TLSMode {
		t.Errorf("TLSMode: got %q, want %q", restored.TLSMode, original.TLSMode)
	}
	if restored.SSLExpiryThreshold != original.SSLExpiryThreshold {
		t.Errorf("SSLExpiryThreshold: got %d, want %d", restored.SSLExpiryThreshold, original.SSLExpiryThreshold)
	}
	if len(restored.Metadata) != len(original.Metadata) {
		t.Fatalf("Metadata length: got %d, want %d", len(restored.Metadata), len(original.Metadata))
	}
	for k, v := range original.Metadata {
		if restored.Metadata[k] != v {
			t.Errorf("Metadata[%q]: got %q, want %q", k, restored.Metadata[k], v)
		}
	}
	if len(restored.ExpectedStatuses) != len(original.ExpectedStatuses) {
		t.Fatalf("ExpectedStatuses length: got %d, want %d", len(restored.ExpectedStatuses), len(original.ExpectedStatuses))
	}
	for i, v := range original.ExpectedStatuses {
		if restored.ExpectedStatuses[i] != v {
			t.Errorf("ExpectedStatuses[%d]: got %d, want %d", i, restored.ExpectedStatuses[i], v)
		}
	}
	if restored.RequestPayload != original.RequestPayload {
		t.Errorf("RequestPayload: got %q, want %q", restored.RequestPayload, original.RequestPayload)
	}
}

// TestGRPCChecker_PlaintextSkipsSSL verifies that plaintext mode does not attempt
// TLS cert extraction and leaves SSLDaysRemaining nil.
//
// Validates: Requirements 5.5
func TestGRPCChecker_PlaintextSkipsSSL(t *testing.T) {
	// Use a mock server that properly sends a unary response.
	addr, stop := startUnaryMockGRPCServer(t, codes.OK)
	defer stop()

	checker := &GRPCChecker{}
	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	result := checker.Check(context.Background(), addr, settingsJSON)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.SSLDaysRemaining != nil {
		t.Fatalf("expected SSLDaysRemaining=nil for plaintext, got %d", *result.SSLDaysRemaining)
	}
}

// TestGRPCChecker_NoPeerCertsGraceful verifies that when TLS is used but the peer
// provides no certificates (e.g., AuthInfo is nil or has no peer certs), the checker
// leaves SSLDaysRemaining nil and does not error.
//
// We test this by:
// 1. Confirming that a TLS connection WITH certs does populate SSLDaysRemaining.
// 2. Confirming that a plaintext connection (AuthInfo is nil) gracefully leaves it nil.
// This exercises the `if p.AuthInfo != nil` guard and `if len(tlsInfo.State.PeerCertificates) > 0` guard.
//
// Validates: Requirements 5.6
func TestGRPCChecker_NoPeerCertsGraceful(t *testing.T) {
	// Generate a self-signed cert valid for 1 year.
	cert := generateSelfSignedCert(t, time.Now().Add(365*24*time.Hour))

	// Start a TLS server with that cert — use a handler that sends a proper unary response.
	addr, stop := startUnaryTLSMockGRPCServer(t, cert, codes.OK)
	defer stop()

	checker := &GRPCChecker{}

	// With tls_skip_verify, the checker connects and extracts certs.
	settings := GRPCSettings{
		TLSMode:          "tls_skip_verify",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	result := checker.Check(context.Background(), addr, settingsJSON)
	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	// With a self-signed cert, ssl_days_remaining should be set.
	if result.SSLDaysRemaining == nil {
		t.Fatalf("expected SSLDaysRemaining to be set for TLS connection with certs")
	}

	// Now test the graceful nil path: plaintext mode means no TLS at all,
	// so the `p.AuthInfo` will be nil → ssl_days_remaining stays nil.
	plaintextSettings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	plaintextJSON, _ := json.Marshal(plaintextSettings)

	// Use the non-TLS mock server for this.
	plainAddr, plainStop := startUnaryMockGRPCServer(t, codes.OK)
	defer plainStop()

	result2 := checker.Check(context.Background(), plainAddr, plaintextJSON)
	if result2.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result2.State, result2.Error)
	}
	if result2.SSLDaysRemaining != nil {
		t.Fatalf("expected SSLDaysRemaining=nil when no TLS, got %d", *result2.SSLDaysRemaining)
	}
}

// TestGRPCChecker_ContextTimeout verifies that when the context times out before
// the server responds, the checker returns state "down" with a timeout-related error
// and reports the elapsed latency.
//
// Validates: Requirements 11.3, 12.4
func TestGRPCChecker_ContextTimeout(t *testing.T) {
	// Start a mock server that delays for 5 seconds before responding.
	addr, stop := startDelayedMockGRPCServer(t, 5*time.Second, codes.OK)
	defer stop()

	checker := &GRPCChecker{}
	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, _ := json.Marshal(settings)

	// Use a very short timeout (50ms) so it expires before the server responds.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := checker.Check(ctx, addr, settingsJSON)
	elapsed := time.Since(start)

	if result.State != "down" {
		t.Fatalf("expected state 'down', got %q", result.State)
	}

	// Error should mention deadline or timeout.
	errLower := strings.ToLower(result.Error)
	if !strings.Contains(errLower, "deadline") && !strings.Contains(errLower, "timeout") && !strings.Contains(errLower, "context deadline exceeded") {
		t.Fatalf("expected timeout-related error, got: %s", result.Error)
	}

	// Latency should be reported (at least some ms).
	if result.LatencyMs <= 0 {
		t.Fatalf("expected positive latency, got %d", result.LatencyMs)
	}

	// Sanity check: elapsed time should be roughly around the timeout (50ms), not the full 5s delay.
	if elapsed > 2*time.Second {
		t.Fatalf("expected check to return quickly after timeout, but took %v", elapsed)
	}
}

// TestGRPCChecker_ContextCancellation verifies that when the context is cancelled
// mid-flight, the checker returns state "down" with a cancellation error and reports latency.
//
// Validates: Requirements 12.2, 12.3
func TestGRPCChecker_ContextCancellation(t *testing.T) {
	// Start a mock server that delays for 5 seconds.
	addr, stop := startDelayedMockGRPCServer(t, 5*time.Second, codes.OK)
	defer stop()

	checker := &GRPCChecker{}
	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, _ := json.Marshal(settings)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 50ms.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result := checker.Check(ctx, addr, settingsJSON)
	elapsed := time.Since(start)

	if result.State != "down" {
		t.Fatalf("expected state 'down', got %q", result.State)
	}

	// Error should mention cancel.
	errLower := strings.ToLower(result.Error)
	if !strings.Contains(errLower, "cancel") && !strings.Contains(errLower, "context canceled") {
		t.Fatalf("expected cancellation error, got: %s", result.Error)
	}

	// Latency should be reported.
	if result.LatencyMs <= 0 {
		t.Fatalf("expected positive latency, got %d", result.LatencyMs)
	}

	// Sanity check: should return quickly, not wait the full 5s.
	if elapsed > 2*time.Second {
		t.Fatalf("expected check to return quickly after cancellation, but took %v", elapsed)
	}
}
