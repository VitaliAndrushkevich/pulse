package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testFS creates an in-memory filesystem that mimics a SvelteKit static build.
func testFS() fs.FS {
	return fstest.MapFS{
		"index.html":                           {Data: []byte("<html><body>SPA</body></html>")},
		"favicon.ico":                          {Data: []byte("icon-data")},
		"_app/immutable/chunks/entry.Abc123.js": {Data: []byte("console.log('entry')")},
		"_app/immutable/assets/style.Xyz789.css": {Data: []byte("body{margin:0}")},
	}
}

func setupSPARouter(distFS fs.FS) *gin.Engine {
	r := gin.New()
	// Register a couple of routes to simulate the real router.
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/api/v1/monitors", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"monitors": []string{}})
	})
	r.NoRoute(spaHandler(distFS))
	return r
}

func TestSPAHandler_ServesIndexHTML(t *testing.T) {
	r := setupSPARouter(testFS())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html content-type, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("expected no-cache for index.html, got %q", cc)
	}
}

func TestSPAHandler_SPAFallbackForClientRoutes(t *testing.T) {
	r := setupSPARouter(testFS())

	// SPA client route that doesn't match any file.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/monitors/abc123", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html for SPA fallback, got %q", ct)
	}
	if body := w.Body.String(); body != "<html><body>SPA</body></html>" {
		t.Fatalf("expected index.html content, got %q", body)
	}
}

func TestSPAHandler_ServesStaticAssets(t *testing.T) {
	r := setupSPARouter(testFS())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/chunks/entry.Abc123.js", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Fatalf("expected application/javascript, got %q", ct)
	}
	if body := w.Body.String(); body != "console.log('entry')" {
		t.Fatalf("unexpected JS body: %q", body)
	}
}

func TestSPAHandler_HashedAssetCacheHeaders(t *testing.T) {
	r := setupSPARouter(testFS())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/assets/style.Xyz789.css", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	expected := "public, max-age=31536000, immutable"
	if cc := w.Header().Get("Cache-Control"); cc != expected {
		t.Fatalf("expected immutable cache header, got %q", cc)
	}
}

func TestSPAHandler_APIPathReturnsJSON404(t *testing.T) {
	r := setupSPARouter(testFS())

	paths := []string{
		"/api/v1/nonexistent",
		"/api/v2/something",
		"/ws/extra",
		"/metrics/foo",
		"/healthz/deep",
		"/swagger/extra",
	}

	for _, p := range paths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("path %s: expected 404, got %d", p, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("path %s: expected JSON content-type, got %q", p, ct)
		}
	}
}

func TestSPAHandler_ServesFavicon(t *testing.T) {
	r := setupSPARouter(testFS())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/x-icon" {
		t.Fatalf("expected image/x-icon, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("expected no-cache for non-hashed asset, got %q", cc)
	}
}

func TestSPAHandler_RegisteredRoutesStillWork(t *testing.T) {
	r := setupSPARouter(testFS())

	// /healthz is a registered route, should still work normally.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", w.Code)
	}
}
