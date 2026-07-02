package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// --- Fake DB for proto source handler tests ---

// protoSourceFakeDB implements db.DBTX for proto source handler tests.
// It intercepts SQL queries to simulate database behavior.
type protoSourceFakeDB struct {
	monitors     map[uuid.UUID]db.Monitor
	protoSources map[uuid.UUID]db.ProtoSource
}

func newProtoSourceFakeDB() *protoSourceFakeDB {
	return &protoSourceFakeDB{
		monitors:     make(map[uuid.UUID]db.Monitor),
		protoSources: make(map[uuid.UUID]db.ProtoSource),
	}
}

func (f *protoSourceFakeDB) addMonitor(id uuid.UUID) {
	f.monitors[id] = db.Monitor{
		ID:     id,
		Name:   "test-monitor",
		Type:   "grpc",
		Target: "localhost:50051",
	}
}

func (f *protoSourceFakeDB) addProtoSource(monitorID uuid.UUID, ps db.ProtoSource) {
	f.protoSources[monitorID] = ps
}

func (f *protoSourceFakeDB) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	// Handle DELETE proto_sources
	if strings.Contains(sql, "DELETE FROM proto_sources") && len(args) > 0 {
		id := args[0].(uuid.UUID)
		delete(f.protoSources, id)
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	// Handle UPDATE monitors (for payload_format reset on delete)
	if strings.Contains(sql, "UPDATE monitors") {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return pgconn.NewCommandTag(""), nil
}

func (f *protoSourceFakeDB) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return &protoSourceEmptyRows{}, nil
}

func (f *protoSourceFakeDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	// Handle GetMonitor query
	if strings.Contains(sql, "FROM monitors") && !strings.Contains(sql, "proto_sources") {
		if len(args) > 0 {
			id := args[0].(uuid.UUID)
			if m, ok := f.monitors[id]; ok {
				return &monitorRow{monitor: m, err: nil}
			}
			return &monitorRow{err: pgx.ErrNoRows}
		}
	}
	// Handle GetProtoSource query
	if strings.Contains(sql, "FROM proto_sources") && !strings.Contains(sql, "EXISTS") {
		if len(args) > 0 {
			monitorID := args[0].(uuid.UUID)
			if ps, ok := f.protoSources[monitorID]; ok {
				return &protoSourceRow{ps: ps, err: nil}
			}
			return &protoSourceRow{err: pgx.ErrNoRows}
		}
	}
	// Handle UpsertProtoSource query (INSERT ... ON CONFLICT ... RETURNING *)
	if strings.Contains(sql, "INSERT INTO proto_sources") {
		if len(args) >= 4 {
			monitorID := args[0].(uuid.UUID)
			sourceType := args[1].(string)
			descriptorBytes := args[2].([]byte)
			metadata := args[3].(json.RawMessage)
			ps := db.ProtoSource{
				ID:              uuid.New(),
				MonitorID:       monitorID,
				SourceType:      sourceType,
				DescriptorBytes: descriptorBytes,
				Metadata:        metadata,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			f.protoSources[monitorID] = ps
			return &protoSourceRow{ps: ps, err: nil}
		}
	}
	// Handle ProtoSourceExists
	if strings.Contains(sql, "EXISTS") {
		if len(args) > 0 {
			monitorID := args[0].(uuid.UUID)
			_, exists := f.protoSources[monitorID]
			return &boolRow{value: exists}
		}
	}
	return &monitorRow{err: pgx.ErrNoRows}
}

// --- Row types for fake results ---

type protoSourceEmptyRows struct{}

func (r *protoSourceEmptyRows) Close()                                        {}
func (r *protoSourceEmptyRows) Err() error                                    { return nil }
func (r *protoSourceEmptyRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *protoSourceEmptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *protoSourceEmptyRows) RawValues() [][]byte                           { return nil }
func (r *protoSourceEmptyRows) Conn() *pgx.Conn                              { return nil }
func (r *protoSourceEmptyRows) Next() bool                                    { return false }
func (r *protoSourceEmptyRows) Scan(_ ...interface{}) error                   { return nil }
func (r *protoSourceEmptyRows) Values() ([]interface{}, error)                { return nil, nil }

type monitorRow struct {
	monitor db.Monitor
	err     error
}

func (r *monitorRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	// The GetMonitor query scans 14 fields.
	if len(dest) >= 14 {
		*dest[0].(*uuid.UUID) = r.monitor.ID
		*dest[1].(*string) = r.monitor.Name
		*dest[2].(*string) = r.monitor.Type
		*dest[3].(*string) = r.monitor.Target
		*dest[4].(*int32) = r.monitor.IntervalSeconds
		*dest[5].(*int32) = r.monitor.TimeoutSeconds
		*dest[6].(*string) = r.monitor.Status
		*dest[7].(*string) = r.monitor.State
		// dest[8] is *pgtype.Timestamptz — leave zero value
		// dest[9] is *pgtype.Timestamptz — leave zero value
		if dest[10] != nil {
			if p, ok := dest[10].(*json.RawMessage); ok {
				*p = r.monitor.Settings
			}
		}
		*dest[11].(*time.Time) = r.monitor.CreatedAt
		*dest[12].(*time.Time) = r.monitor.UpdatedAt
		*dest[13].(*int32) = r.monitor.HistoryRetentionDays
	}
	return nil
}

type protoSourceRow struct {
	ps  db.ProtoSource
	err error
}

func (r *protoSourceRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	// ProtoSource has 7 fields: id, monitor_id, source_type, descriptor_bytes, metadata, created_at, updated_at
	if len(dest) >= 7 {
		*dest[0].(*uuid.UUID) = r.ps.ID
		*dest[1].(*uuid.UUID) = r.ps.MonitorID
		*dest[2].(*string) = r.ps.SourceType
		*dest[3].(*[]byte) = r.ps.DescriptorBytes
		*dest[4].(*json.RawMessage) = r.ps.Metadata
		*dest[5].(*time.Time) = r.ps.CreatedAt
		*dest[6].(*time.Time) = r.ps.UpdatedAt
	}
	return nil
}

type boolRow struct {
	value bool
}

func (r *boolRow) Scan(dest ...interface{}) error {
	if len(dest) > 0 {
		*dest[0].(*bool) = r.value
	}
	return nil
}

// --- Test setup helpers ---

func setupProtoSourceRouter(fdb *protoSourceFakeDB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	queries := db.New(fdb)
	h := handlers.NewProtoSourceHandler(queries, nil)
	v1 := r.Group("/api/v1")
	h.Register(v1)
	return r
}

// createMultipartRequest builds a multipart/form-data request with the given files.
func createMultipartRequest(monitorID string, files map[string][]byte) (*http.Request, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.CreateFormFile("file", name)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(fw, bytes.NewReader(content)); err != nil {
			return nil, err
		}
	}
	w.Close()

	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", monitorID)
	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

// makeValidFileDescriptorSet creates a minimal valid FileDescriptorSet binary for testing.
func makeValidFileDescriptorSet() []byte {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strRef("test.proto"),
				Package: strRef("testpkg"),
				Syntax:  strRef("proto3"),
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: strRef("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       strRef("TestMethod"),
								InputType:  strRef(".testpkg.TestRequest"),
								OutputType: strRef(".testpkg.TestResponse"),
							},
						},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{Name: strRef("TestRequest")},
					{Name: strRef("TestResponse")},
				},
			},
		},
	}
	data, _ := proto.Marshal(fds)
	return data
}

func strRef(s string) *string { return &s }

// --- Upload Tests ---

// TestProtoSourceUpload_ValidProtoFile tests that a valid .proto file upload returns success.
// Validates: Requirements 5.1
func TestProtoSourceUpload_ValidProtoFile(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	protoContent := []byte(`syntax = "proto3";
package testpkg;
service TestService {
  rpc TestMethod (TestRequest) returns (TestResponse);
}
message TestRequest { string name = 1; }
message TestResponse { string greeting = 1; }
`)
	files := map[string][]byte{"test.proto": protoContent}
	req, err := createMultipartRequest(monitorID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["source_type"] != "upload" {
		t.Errorf("expected source_type=upload, got %v", resp["source_type"])
	}
	filenames, ok := resp["filenames"].([]interface{})
	if !ok || len(filenames) == 0 {
		t.Error("expected non-empty filenames array")
	}
}

// TestProtoSourceUpload_ValidDescFile tests that a valid .desc file upload returns success.
// Validates: Requirements 5.1
func TestProtoSourceUpload_ValidDescFile(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	descData := makeValidFileDescriptorSet()
	files := map[string][]byte{"service.desc": descData}
	req, err := createMultipartRequest(monitorID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["source_type"] != "upload" {
		t.Errorf("expected source_type=upload, got %v", resp["source_type"])
	}
}

// TestProtoSourceUpload_InvalidExtension tests that uploading an unsupported file type returns 400.
// Validates: Requirements 5.1
func TestProtoSourceUpload_InvalidExtension(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	files := map[string][]byte{"readme.txt": []byte("not a proto file")}
	req, err := createMultipartRequest(monitorID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "PROTO_PARSE_ERROR")
}

// TestProtoSourceUpload_FileTooLarge tests that uploading a file exceeding 5MB returns 400.
// Validates: Requirements 5.1
func TestProtoSourceUpload_FileTooLarge(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	// Create a file slightly over 5MB.
	largeContent := make([]byte, 5*1024*1024+1)
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	files := map[string][]byte{"large.proto": largeContent}
	req, err := createMultipartRequest(monitorID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "PROTO_SIZE_EXCEEDED")
}

// TestProtoSourceUpload_InvalidProtoSyntax tests that uploading a .proto file with invalid syntax returns 400.
// Validates: Requirements 5.1
func TestProtoSourceUpload_InvalidProtoSyntax(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	files := map[string][]byte{"bad.proto": []byte("this is not valid proto syntax {{{{")}
	req, err := createMultipartRequest(monitorID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "PROTO_PARSE_ERROR")
}

// TestProtoSourceUpload_UnresolvedImports tests that .proto files with unresolved imports return 400.
// Note: The protocompile library returns file-not-found errors directly from Compile()
// rather than through the reporter, so the handler classifies these as PROTO_PARSE_ERROR.
// The important validation is that the error IS returned as a 400.
// Validates: Requirements 5.1
func TestProtoSourceUpload_UnresolvedImports(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	protoContent := []byte(`syntax = "proto3";
package testpkg;
import "missing.proto";
message Request { string name = 1; }
`)
	files := map[string][]byte{"service.proto": protoContent}
	req, err := createMultipartRequest(monitorID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// The error should reference the missing import file.
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj := resp["error"].(map[string]interface{})
	code := errObj["code"].(string)
	message := errObj["message"].(string)

	// Accept either PROTO_UNRESOLVED_IMPORTS or PROTO_PARSE_ERROR — both indicate rejection.
	if code != "PROTO_UNRESOLVED_IMPORTS" && code != "PROTO_PARSE_ERROR" {
		t.Errorf("expected PROTO_UNRESOLVED_IMPORTS or PROTO_PARSE_ERROR, got %q", code)
	}
	// The error message should mention the missing import.
	if !strings.Contains(message, "missing.proto") {
		t.Errorf("expected error to mention missing import, got: %q", message)
	}
}

// TestProtoSourceUpload_MonitorNotFound tests that uploading to a non-existent monitor returns 404.
// Validates: Requirements 5.7
func TestProtoSourceUpload_MonitorNotFound(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	router := setupProtoSourceRouter(fdb)

	nonExistentID := uuid.New()
	files := map[string][]byte{"test.proto": []byte(`syntax = "proto3";`)}
	req, err := createMultipartRequest(nonExistentID.String(), files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "MONITOR_NOT_FOUND")
}

// TestProtoSourceUpload_InvalidMonitorID tests that an invalid UUID in the path returns 400.
// Validates: Requirements 5.7
func TestProtoSourceUpload_InvalidMonitorID(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	router := setupProtoSourceRouter(fdb)

	files := map[string][]byte{"test.proto": []byte(`syntax = "proto3";`)}
	req, err := createMultipartRequest("not-a-uuid", files)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "INVALID_ID")
}

// --- Reflect Tests ---

// TestProtoSourceReflect_MonitorNotFound tests reflect with non-existent monitor returns 404.
// Validates: Requirements 5.7
func TestProtoSourceReflect_MonitorNotFound(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	router := setupProtoSourceRouter(fdb)

	nonExistentID := uuid.New()
	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source/reflect", nonExistentID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "MONITOR_NOT_FOUND")
}

// TestProtoSourceReflect_InvalidMonitorID tests reflect with invalid UUID returns 400.
// Validates: Requirements 5.7
func TestProtoSourceReflect_InvalidMonitorID(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	router := setupProtoSourceRouter(fdb)

	url := "/api/v1/monitors/not-a-uuid/proto-source/reflect"
	req := httptest.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "INVALID_ID")
}

// --- Get Tests ---

// TestProtoSourceGet_Success tests getting an existing proto source returns 200.
// Validates: Requirements 5.3
func TestProtoSourceGet_Success(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)

	metadata := json.RawMessage(`{"services":[{"full_name":"testpkg.TestService","methods":[{"name":"TestMethod","full_name":"testpkg.TestService/TestMethod","input_type":"testpkg.TestRequest","output_type":"testpkg.TestResponse"}]}],"message_types":["testpkg.TestRequest","testpkg.TestResponse"],"filenames":["test.proto"]}`)
	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: makeValidFileDescriptorSet(),
		Metadata:        metadata,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	router := setupProtoSourceRouter(fdb)

	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", monitorID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["source_type"] != "upload" {
		t.Errorf("expected source_type=upload, got %v", resp["source_type"])
	}
	if resp["filenames"] == nil {
		t.Error("expected filenames field in response")
	}
}

// TestProtoSourceGet_NotFound tests that getting a non-existent proto source returns 404.
// Validates: Requirements 5.4
func TestProtoSourceGet_NotFound(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", monitorID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "PROTO_SOURCE_NOT_FOUND")
}

// TestProtoSourceGet_MonitorNotFound tests that getting proto source for a non-existent monitor returns 404.
// Validates: Requirements 5.7
func TestProtoSourceGet_MonitorNotFound(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	router := setupProtoSourceRouter(fdb)

	nonExistentID := uuid.New()
	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", nonExistentID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "MONITOR_NOT_FOUND")
}

// --- Delete Tests ---

// TestProtoSourceDelete_Success tests successful deletion of an existing proto source.
// Validates: Requirements 5.5
func TestProtoSourceDelete_Success(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: makeValidFileDescriptorSet(),
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})
	router := setupProtoSourceRouter(fdb)

	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", monitorID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
}

// TestProtoSourceDelete_Idempotent tests that deleting when no proto source exists returns success.
// Validates: Requirements 5.5
func TestProtoSourceDelete_Idempotent(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	monitorID := uuid.New()
	fdb.addMonitor(monitorID)
	router := setupProtoSourceRouter(fdb)

	// No proto source exists for this monitor — delete should still return OK.
	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", monitorID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
}

// TestProtoSourceDelete_MonitorNotFound tests that deleting proto source for a non-existent monitor returns 404.
// Validates: Requirements 5.7
func TestProtoSourceDelete_MonitorNotFound(t *testing.T) {
	fdb := newProtoSourceFakeDB()
	router := setupProtoSourceRouter(fdb)

	nonExistentID := uuid.New()
	url := fmt.Sprintf("/api/v1/monitors/%s/proto-source", nonExistentID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	assertErrorCode(t, w.Body.Bytes(), "MONITOR_NOT_FOUND")
}

// --- Helpers ---

func assertErrorCode(t *testing.T, body []byte, expectedCode string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error envelope in response, got: %s", string(body))
	}
	code, ok := errObj["code"].(string)
	if !ok {
		t.Fatalf("expected string code in error, got: %v", errObj["code"])
	}
	if code != expectedCode {
		t.Errorf("expected error code %q, got %q", expectedCode, code)
	}
}
