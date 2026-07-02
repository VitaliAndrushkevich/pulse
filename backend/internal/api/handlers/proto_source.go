package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	protolib "github.com/VitaliAndrushkevich/pulse/internal/proto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

const maxProtoUploadSize = 5 * 1024 * 1024 // 5MB

// ProtoSourceHandler manages proto source uploads and retrieval for monitors.
type ProtoSourceHandler struct {
	queries  *db.Queries
	pool     *pgxpool.Pool
	registry *protolib.Registry
}

// NewProtoSourceHandler creates a new handler for proto source operations.
func NewProtoSourceHandler(queries *db.Queries, pool *pgxpool.Pool) *ProtoSourceHandler {
	return &ProtoSourceHandler{
		queries:  queries,
		pool:     pool,
		registry: protolib.NewRegistry(),
	}
}

// Register mounts proto source routes on the given router group.
func (h *ProtoSourceHandler) Register(rg *gin.RouterGroup) {
	ps := rg.Group("/monitors/:id/proto-source")
	ps.POST("", h.Upload)
	ps.POST("/reflect", h.Reflect)
	ps.GET("", h.Get)
	ps.DELETE("", h.Delete)
}

// Upload handles POST /monitors/:id/proto-source.
// Accepts multipart/form-data with .proto or .desc/.bin files.
func (h *ProtoSourceHandler) Upload(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Verify the monitor exists.
	_, err = h.queries.GetMonitor(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusNotFound, "MONITOR_NOT_FOUND", "monitor not found")
		return
	}

	// Parse multipart form with 5MB max memory.
	if err := c.Request.ParseMultipartForm(maxProtoUploadSize); err != nil {
		apiError(c, http.StatusBadRequest, "PROTO_SIZE_EXCEEDED", "upload exceeds 5MB limit")
		return
	}

	form := c.Request.MultipartForm
	if form == nil || len(form.File) == 0 {
		apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", "no files uploaded")
		return
	}

	// Collect all uploaded files, detect type by extension.
	var totalSize int64
	protoFiles := make(map[string][]byte)
	var descriptorData []byte
	var descriptorFilename string
	var filenames []string
	isDescriptor := false

	for _, fileHeaders := range form.File {
		for _, fh := range fileHeaders {
			totalSize += fh.Size
			if totalSize > maxProtoUploadSize {
				apiError(c, http.StatusBadRequest, "PROTO_SIZE_EXCEEDED", "total upload size exceeds 5MB limit")
				return
			}

			f, err := fh.Open()
			if err != nil {
				apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to read file %q", fh.Filename))
				return
			}

			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to read file %q", fh.Filename))
				return
			}

			ext := strings.ToLower(filepath.Ext(fh.Filename))
			switch ext {
			case ".proto":
				protoFiles[fh.Filename] = data
				filenames = append(filenames, fh.Filename)
			case ".desc", ".bin":
				descriptorData = data
				descriptorFilename = fh.Filename
				isDescriptor = true
				filenames = append(filenames, fh.Filename)
			default:
				apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("unsupported file type %q; expected .proto, .desc, or .bin", ext))
				return
			}
		}
	}

	if len(protoFiles) == 0 && descriptorData == nil {
		apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", "no valid proto files uploaded")
		return
	}

	// Parse based on detected file type.
	var fds *descriptorpb.FileDescriptorSet
	if isDescriptor {
		fds, err = h.registry.ParseFileDescriptorSet(descriptorData)
		if err != nil {
			apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to parse %q: %s", descriptorFilename, err.Error()))
			return
		}
	} else {
		fds, err = h.registry.ParseProtoFiles(protoFiles)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "unresolved imports") {
				apiError(c, http.StatusBadRequest, "PROTO_UNRESOLVED_IMPORTS", errMsg)
				return
			}
			apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", errMsg)
			return
		}
	}

	// Extract metadata from the parsed FileDescriptorSet.
	metadata, err := protolib.ExtractMetadata(fds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to extract metadata: %s", err.Error()))
		return
	}

	// Override filenames in metadata with the uploaded filenames.
	metadata.Filenames = filenames

	// Serialize the FileDescriptorSet to bytes.
	descriptorBytes, err := proto.Marshal(fds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", "failed to serialize FileDescriptorSet")
		return
	}

	// Marshal metadata to JSON for storage.
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", "failed to serialize metadata")
		return
	}

	// Upsert proto source in DB.
	row, err := h.queries.UpsertProtoSource(c.Request.Context(), db.UpsertProtoSourceParams{
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: descriptorBytes,
		Metadata:        metadataJSON,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to store proto source")
		return
	}

	// Build response.
	c.JSON(http.StatusOK, gin.H{
		"source_type": row.SourceType,
		"filenames":   filenames,
		"services":    metadata.Services,
		"created_at":  row.CreatedAt,
		"size_bytes":  len(descriptorBytes),
	})
}

// grpcSettings is a minimal view of the monitor's settings JSON for extracting
// gRPC connection parameters needed by the Reflect handler.
type grpcSettings struct {
	TLSMode string `json:"tls_mode,omitempty"`
}

// Reflect handles POST /monitors/:id/proto-source/reflect.
// Triggers Server Reflection schema discovery against the monitor's configured target.
func (h *ProtoSourceHandler) Reflect(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Verify the monitor exists and load its target/settings.
	monitor, err := h.queries.GetMonitor(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusNotFound, "MONITOR_NOT_FOUND", "monitor not found")
		return
	}

	target := monitor.Target

	// Parse TLS settings from the monitor's settings JSON.
	var settings grpcSettings
	if monitor.Settings != nil {
		_ = json.Unmarshal(monitor.Settings, &settings)
	}
	// Apply same defaults as the gRPC checker.
	if settings.TLSMode == "" {
		settings.TLSMode = "tls"
	}

	// Build TLS config based on tls_mode.
	var tlsCfg *tls.Config
	switch settings.TLSMode {
	case "plaintext", "":
		// No TLS — pass nil to ReflectServices.
		tlsCfg = nil
	case "tls":
		tlsCfg = &tls.Config{}
	case "tls_skip_verify":
		tlsCfg = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	default:
		tlsCfg = &tls.Config{}
	}

	// Call reflection with a 10-second timeout.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	fds, err := protolib.ReflectServices(ctx, target, tlsCfg)
	if err != nil {
		classifyAndRespondReflectionError(c, err)
		return
	}

	// Extract metadata from the discovered FileDescriptorSet.
	metadata, err := protolib.ExtractMetadata(fds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to extract metadata: %s", err.Error()))
		return
	}

	// Validate non-empty services.
	if len(metadata.Services) == 0 {
		apiError(c, http.StatusBadRequest, "REFLECTION_NO_SERVICES", "no discoverable services found")
		return
	}

	// Serialize the FileDescriptorSet to bytes.
	descriptorBytes, err := proto.Marshal(fds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", "failed to serialize FileDescriptorSet")
		return
	}

	// Enforce 5MB size limit.
	if len(descriptorBytes) > maxProtoUploadSize {
		apiError(c, http.StatusBadRequest, "PROTO_SIZE_EXCEEDED", "reflected schema exceeds 5MB limit")
		return
	}

	// Marshal metadata to JSON for storage.
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", "failed to serialize metadata")
		return
	}

	// Upsert proto source in DB with source_type = "reflection".
	row, err := h.queries.UpsertProtoSource(c.Request.Context(), db.UpsertProtoSourceParams{
		MonitorID:       monitorID,
		SourceType:      "reflection",
		DescriptorBytes: descriptorBytes,
		Metadata:        metadataJSON,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to store proto source")
		return
	}

	// Build response.
	c.JSON(http.StatusOK, gin.H{
		"source_type": row.SourceType,
		"filenames":   metadata.Filenames,
		"services":    metadata.Services,
		"created_at":  row.CreatedAt,
		"size_bytes":  len(descriptorBytes),
	})
}

// classifyAndRespondReflectionError maps reflection errors to HTTP error responses.
func classifyAndRespondReflectionError(c *gin.Context, err error) {
	msg := err.Error()

	switch {
	case strings.Contains(msg, "does not support reflection"):
		apiError(c, http.StatusBadRequest, "REFLECTION_UNAVAILABLE", msg)
	case strings.Contains(msg, "reflection timeout"):
		apiError(c, http.StatusGatewayTimeout, "REFLECTION_TIMEOUT", msg)
	case strings.Contains(msg, "connection failed") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") || strings.Contains(msg, "unreachable"):
		apiError(c, http.StatusBadGateway, "REFLECTION_CONNECTION_FAILED", msg)
	case strings.Contains(msg, "no discoverable services"):
		apiError(c, http.StatusBadRequest, "REFLECTION_NO_SERVICES", msg)
	default:
		apiError(c, http.StatusBadGateway, "REFLECTION_CONNECTION_FAILED", msg)
	}
}

// Get handles GET /monitors/:id/proto-source.
// Returns the current proto source metadata for the monitor.
func (h *ProtoSourceHandler) Get(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Verify the monitor exists.
	_, err = h.queries.GetMonitor(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusNotFound, "MONITOR_NOT_FOUND", "monitor not found")
		return
	}

	// Fetch the proto source.
	row, err := h.queries.GetProtoSource(c.Request.Context(), monitorID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiError(c, http.StatusNotFound, "PROTO_SOURCE_NOT_FOUND", "no proto source configured for this monitor")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to retrieve proto source")
		return
	}

	// Parse stored metadata.
	var metadata protolib.ProtoSourceMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to parse stored metadata")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"source_type": row.SourceType,
		"filenames":   metadata.Filenames,
		"services":    metadata.Services,
		"created_at":  row.CreatedAt,
		"size_bytes":  len(row.DescriptorBytes),
	})
}

// Delete handles DELETE /monitors/:id/proto-source.
// Removes the stored proto source for the monitor (idempotent).
func (h *ProtoSourceHandler) Delete(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Verify the monitor exists.
	monitor, err := h.queries.GetMonitor(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusNotFound, "MONITOR_NOT_FOUND", "monitor not found")
		return
	}

	// Delete the proto source (idempotent — no error if it doesn't exist).
	if err := h.queries.DeleteProtoSource(c.Request.Context(), monitorID); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete proto source")
		return
	}

	// If payload_format was "proto_json", reset it to "raw".
	var settings map[string]interface{}
	if monitor.Settings != nil {
		_ = json.Unmarshal(monitor.Settings, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	if fmt.Sprintf("%v", settings["payload_format"]) == "proto_json" {
		settings["payload_format"] = "raw"
		updatedSettings, err := json.Marshal(settings)
		if err == nil {
			// Update the monitor settings in the database.
			_, _ = h.pool.Exec(c.Request.Context(),
				"UPDATE monitors SET settings = $1, updated_at = now() WHERE id = $2",
				updatedSettings, monitorID,
			)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
	})
}

// AdHocReflectRequest is the request body for the ad-hoc reflection endpoint.
type AdHocReflectRequest struct {
	Target  string `json:"target" binding:"required"`
	TLSMode string `json:"tls_mode,omitempty"`
}

// AdHocReflect handles POST /api/v1/grpc/reflect.
// Performs Server Reflection against an arbitrary target without requiring a saved monitor.
// Used during monitor creation to discover services before the monitor exists in the database.
func (h *ProtoSourceHandler) AdHocReflect(c *gin.Context) {
	var req AdHocReflectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_REQUEST", "target is required")
		return
	}

	if req.Target == "" {
		apiError(c, http.StatusBadRequest, "INVALID_REQUEST", "target must not be empty")
		return
	}

	// Apply default TLS mode (same as gRPC checker).
	tlsMode := req.TLSMode
	if tlsMode == "" {
		tlsMode = "tls"
	}

	// Build TLS config.
	var tlsCfg *tls.Config
	switch tlsMode {
	case "plaintext":
		tlsCfg = nil
	case "tls":
		tlsCfg = &tls.Config{}
	case "tls_skip_verify":
		tlsCfg = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	default:
		tlsCfg = &tls.Config{}
	}

	// Call reflection with a 10-second timeout.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	fds, err := protolib.ReflectServices(ctx, req.Target, tlsCfg)
	if err != nil {
		classifyAndRespondReflectionError(c, err)
		return
	}

	// Extract metadata.
	metadata, err := protolib.ExtractMetadata(fds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to extract metadata: %s", err.Error()))
		return
	}

	if len(metadata.Services) == 0 {
		apiError(c, http.StatusBadRequest, "REFLECTION_NO_SERVICES", "no discoverable services found")
		return
	}

	// Return metadata without persisting (no monitor ID to associate with).
	c.JSON(http.StatusOK, gin.H{
		"source_type": "reflection",
		"filenames":   metadata.Filenames,
		"services":    metadata.Services,
		"size_bytes":  0,
	})
}

// AdHocParseProto handles POST /api/v1/grpc/parse-proto.
// Parses uploaded .proto/.desc files without requiring a saved monitor.
// Returns service metadata for method selection during monitor creation.
func (h *ProtoSourceHandler) AdHocParseProto(c *gin.Context) {
	// Parse multipart form with 5MB max memory.
	if err := c.Request.ParseMultipartForm(maxProtoUploadSize); err != nil {
		apiError(c, http.StatusBadRequest, "PROTO_SIZE_EXCEEDED", "upload exceeds 5MB limit")
		return
	}

	form := c.Request.MultipartForm
	if form == nil || len(form.File) == 0 {
		apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", "no files uploaded")
		return
	}

	// Collect files (same logic as Upload handler).
	var totalSize int64
	protoFiles := make(map[string][]byte)
	var descriptorData []byte
	var descriptorFilename string
	var filenames []string
	isDescriptor := false

	for _, fileHeaders := range form.File {
		for _, fh := range fileHeaders {
			totalSize += fh.Size
			if totalSize > maxProtoUploadSize {
				apiError(c, http.StatusBadRequest, "PROTO_SIZE_EXCEEDED", "total upload size exceeds 5MB limit")
				return
			}

			f, err := fh.Open()
			if err != nil {
				apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to read file %q", fh.Filename))
				return
			}

			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to read file %q", fh.Filename))
				return
			}

			ext := strings.ToLower(filepath.Ext(fh.Filename))
			switch ext {
			case ".proto":
				protoFiles[fh.Filename] = data
				filenames = append(filenames, fh.Filename)
			case ".desc", ".bin":
				descriptorData = data
				descriptorFilename = fh.Filename
				isDescriptor = true
				filenames = append(filenames, fh.Filename)
			default:
				apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("unsupported file type %q; expected .proto, .desc, or .bin", ext))
				return
			}
		}
	}

	if len(protoFiles) == 0 && descriptorData == nil {
		apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", "no valid proto files uploaded")
		return
	}

	// Parse based on detected file type.
	var fds *descriptorpb.FileDescriptorSet
	var err error
	if isDescriptor {
		fds, err = h.registry.ParseFileDescriptorSet(descriptorData)
		if err != nil {
			apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to parse %q: %s", descriptorFilename, err.Error()))
			return
		}
	} else {
		fds, err = h.registry.ParseProtoFiles(protoFiles)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "unresolved imports") {
				apiError(c, http.StatusBadRequest, "PROTO_UNRESOLVED_IMPORTS", errMsg)
				return
			}
			apiError(c, http.StatusBadRequest, "PROTO_PARSE_ERROR", errMsg)
			return
		}
	}

	// Extract metadata.
	metadata, err := protolib.ExtractMetadata(fds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "PROTO_PARSE_ERROR", fmt.Sprintf("failed to extract metadata: %s", err.Error()))
		return
	}
	metadata.Filenames = filenames

	// Return metadata without persisting.
	c.JSON(http.StatusOK, gin.H{
		"source_type": "upload",
		"filenames":   filenames,
		"services":    metadata.Services,
		"size_bytes":  0,
	})
}
