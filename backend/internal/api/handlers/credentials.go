package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// CredentialHandler manages monitor authentication credentials.
type CredentialHandler struct {
	queries *db.Queries
	key     []byte // AES-256-GCM encryption key
}

// NewCredentialHandler creates a handler with the given query layer and encryption key.
func NewCredentialHandler(queries *db.Queries, key []byte) *CredentialHandler {
	return &CredentialHandler{queries: queries, key: key}
}

// Create handles POST /monitors/:id/credentials.
func (h *CredentialHandler) Create(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	var req CreateCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "auth_type must be one of: bearer, basic, header")
		return
	}

	// Validate required fields per auth_type.
	if errMsg := validateCredentialFields(req); errMsg != "" {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", errMsg)
		return
	}

	// Verify the monitor exists.
	_, err = h.queries.GetMonitor(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
		return
	}

	// Build the credential payload for encryption.
	payload := buildCredentialPayload(req)

	// Encrypt the payload.
	encrypted, err := encryptCredentialPayload(h.key, payload)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to encrypt credential value")
		return
	}

	// Persist the credential.
	row, err := h.queries.CreateCredential(c.Request.Context(), db.CreateCredentialParams{
		MonitorID:      monitorID,
		AuthType:       req.AuthType,
		Name:           req.Name,
		EncryptedValue: encrypted,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create credential")
		return
	}

	c.JSON(http.StatusCreated, toCredentialResponse(row.ID, row.AuthType, row.Name, req, row.CreatedAt, row.UpdatedAt))
}

// Update handles PUT /monitors/:id/credentials/:credentialId.
func (h *CredentialHandler) Update(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	credentialID, err := uuid.Parse(c.Param("credentialId"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "credentialId must be a valid UUID")
		return
	}

	var req UpdateCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	// Load the existing credential.
	existing, err := h.queries.GetCredential(c.Request.Context(), db.GetCredentialParams{
		ID:        credentialID,
		MonitorID: monitorID,
	})
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "credential not found")
		return
	}

	// Decrypt the current payload.
	payload, err := decryptCredentialPayload(h.key, existing.EncryptedValue)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to decrypt credential value")
		return
	}

	// Merge update fields into the existing payload.
	if req.Token != "" {
		payload.Token = req.Token
	}
	if req.Username != "" {
		payload.Username = req.Username
	}
	if req.Password != "" {
		payload.Password = req.Password
	}
	if req.HeaderName != "" {
		payload.HeaderName = req.HeaderName
	}
	if req.HeaderValue != "" {
		payload.HeaderValue = req.HeaderValue
	}

	// Determine the updated name: use request name if provided, otherwise keep existing.
	name := existing.Name
	if req.Name != "" {
		name = req.Name
	}

	// Re-encrypt the updated payload.
	encrypted, err := encryptCredentialPayload(h.key, payload)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to encrypt credential value")
		return
	}

	// Persist the update.
	row, err := h.queries.UpdateCredential(c.Request.Context(), db.UpdateCredentialParams{
		ID:             credentialID,
		MonitorID:      monitorID,
		Name:           name,
		EncryptedValue: encrypted,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to update credential")
		return
	}

	c.JSON(http.StatusOK, toCredentialResponseFromUpdate(row, payload))
}

// toCredentialResponseFromUpdate builds a metadata-only CredentialResponse from an UpdateCredentialRow.
// It includes non-secret metadata (header_name for header type, username for basic type).
func toCredentialResponseFromUpdate(row db.UpdateCredentialRow, payload CredentialPayload) CredentialResponse {
	resp := CredentialResponse{
		ID:        row.ID,
		AuthType:  row.AuthType,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}

	switch row.AuthType {
	case "header":
		hn := payload.HeaderName
		resp.HeaderName = &hn
	case "basic":
		un := payload.Username
		resp.Username = &un
	}

	return resp
}

// validateCredentialFields checks that the required secret fields are present
// for the given auth_type. Returns an error message or empty string if valid.
func validateCredentialFields(req CreateCredentialRequest) string {
	switch req.AuthType {
	case "bearer":
		if req.Token == "" {
			return "token is required for bearer auth_type"
		}
	case "basic":
		if req.Username == "" {
			return "username is required for basic auth_type"
		}
		if req.Password == "" {
			return "password is required for basic auth_type"
		}
	case "header":
		if req.HeaderName == "" {
			return "header_name is required for header auth_type"
		}
		if req.HeaderValue == "" {
			return "header_value is required for header auth_type"
		}
	}
	return ""
}

// buildCredentialPayload constructs the CredentialPayload from a create request.
func buildCredentialPayload(req CreateCredentialRequest) CredentialPayload {
	switch req.AuthType {
	case "bearer":
		return CredentialPayload{Token: req.Token}
	case "basic":
		return CredentialPayload{Username: req.Username, Password: req.Password}
	case "header":
		return CredentialPayload{HeaderName: req.HeaderName, HeaderValue: req.HeaderValue}
	default:
		return CredentialPayload{}
	}
}

// Delete handles DELETE /monitors/:id/credentials/:credentialId.
func (h *CredentialHandler) Delete(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	credentialID, err := uuid.Parse(c.Param("credentialId"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "credentialId must be a valid UUID")
		return
	}

	// Verify the credential exists.
	_, err = h.queries.GetCredential(c.Request.Context(), db.GetCredentialParams{
		ID:        credentialID,
		MonitorID: monitorID,
	})
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "credential not found")
		return
	}

	// Delete the credential.
	err = h.queries.DeleteCredential(c.Request.Context(), db.DeleteCredentialParams{
		ID:        credentialID,
		MonitorID: monitorID,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete credential")
		return
	}

	c.Status(http.StatusNoContent)
}

// List handles GET /monitors/:id/credentials.
func (h *CredentialHandler) List(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	rows, err := h.queries.ListCredentialsByMonitorID(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list credentials")
		return
	}

	responses := make([]CredentialResponse, 0, len(rows))
	for _, row := range rows {
		resp, err := credentialRowToResponse(h.key, row)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to read credential metadata")
			return
		}
		responses = append(responses, resp)
	}

	c.JSON(http.StatusOK, responses)
}

// Get handles GET /monitors/:id/credentials/:credentialId.
func (h *CredentialHandler) Get(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	credentialID, err := uuid.Parse(c.Param("credentialId"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "credentialId must be a valid UUID")
		return
	}

	row, err := h.queries.GetCredential(c.Request.Context(), db.GetCredentialParams{
		ID:        credentialID,
		MonitorID: monitorID,
	})
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "credential not found")
		return
	}

	resp, err := credentialRowToResponse(h.key, row)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to read credential metadata")
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Register mounts credential routes on the given router group.
// Routes are nested under /monitors/:id/credentials.
func (h *CredentialHandler) Register(rg *gin.RouterGroup) {
	creds := rg.Group("/monitors/:id/credentials")
	creds.POST("", h.Create)
	creds.GET("", h.List)
	creds.GET("/:credentialId", h.Get)
	creds.PUT("/:credentialId", h.Update)
	creds.DELETE("/:credentialId", h.Delete)
}

// credentialRowToResponse converts a database MonitorCredential row into a
// metadata-only CredentialResponse by decrypting the payload to extract
// non-secret metadata (header_name for header type, username for basic type).
func credentialRowToResponse(key []byte, row db.MonitorCredential) (CredentialResponse, error) {
	resp := CredentialResponse{
		ID:        row.ID,
		AuthType:  row.AuthType,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}

	payload, err := decryptCredentialPayload(key, row.EncryptedValue)
	if err != nil {
		return resp, err
	}

	switch row.AuthType {
	case "header":
		hn := payload.HeaderName
		resp.HeaderName = &hn
	case "basic":
		un := payload.Username
		resp.Username = &un
	}

	return resp, nil
}

// toCredentialResponse builds a metadata-only CredentialResponse from a create request.
// It includes header_name for header type and username for basic type.
func toCredentialResponse(id uuid.UUID, authType, name string, req CreateCredentialRequest, createdAt, updatedAt time.Time) CredentialResponse {
	resp := CredentialResponse{
		ID:        id,
		AuthType:  authType,
		Name:      name,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	switch authType {
	case "header":
		hn := req.HeaderName
		resp.HeaderName = &hn
	case "basic":
		un := req.Username
		resp.Username = &un
	}

	return resp
}
