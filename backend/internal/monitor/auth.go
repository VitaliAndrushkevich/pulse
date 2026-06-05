package monitor

import (
	"context"
	"encoding/json"
)

// AuthCredential is a decrypted credential ready for injection into requests.
// This is defined in the monitor package (separate from handlers.AuthCredential)
// to avoid circular dependencies — the scheduler maps between the two.
type AuthCredential struct {
	AuthType    string // "bearer", "basic", "header"
	Token       string // bearer token value
	Username    string // basic auth username
	Password    string // basic auth password
	HeaderName  string // custom header name
	HeaderValue string // custom header value
}

// AuthenticatedChecker extends Checker with credential injection support.
// The scheduler calls CheckWithAuth when credentials exist for a monitor,
// falling back to Check when none are configured.
type AuthenticatedChecker interface {
	Checker
	CheckWithAuth(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result
}
