package handlers

import (
	"time"

	"github.com/google/uuid"
)

// CreateCredentialRequest is the request body for creating a new monitor credential.
type CreateCredentialRequest struct {
	AuthType    string `json:"auth_type" binding:"required,oneof=bearer basic header"`
	Name        string `json:"name" binding:"required"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
}

// UpdateCredentialRequest is the request body for updating an existing credential.
type UpdateCredentialRequest struct {
	Name        string `json:"name,omitempty"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
}

// CredentialResponse is the metadata-only response returned by credential endpoints.
// Secret values are never included.
type CredentialResponse struct {
	ID         uuid.UUID `json:"id"`
	AuthType   string    `json:"auth_type"`
	Name       string    `json:"name"`
	HeaderName *string   `json:"header_name,omitempty"` // only for auth_type=header
	Username   *string   `json:"username,omitempty"`    // only for auth_type=basic
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CredentialPayload is the plaintext structure encrypted at rest.
type CredentialPayload struct {
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
}

// AuthCredential is a decrypted credential ready for injection into requests.
type AuthCredential struct {
	AuthType    string
	Token       string
	Username    string
	Password    string
	HeaderName  string
	HeaderValue string
}
