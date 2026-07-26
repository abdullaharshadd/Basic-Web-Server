```go
package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// tempDir creates a temporary directory with the given files and returns the
// directory path.  The files map is filename -> content.
func makeTempDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		// create parent dirs if needed
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return dir
}

// ---------------------------------------------------------------------------
// redirectMap invariants
// ---------------------------------------------------------------------------

func TestRedirectMap_Invariants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"/":          "/index.html",
		"/index.htm": "/index.html",
		"/index":     "/index.html",
	}

	assert.Equal(t, len(expected), len(redirectMap),
		"redirect map must contain exactly 3 entries")

	for src, dst := range expected {
		got, ok := redirectMap[src]
		assert.True(t, ok, "redirect map must contain key %q", src)
		assert.Equal(t, dst, got, "redirect map[%q] must equal %q", src, dst)
	}
}

// ---------------------------------------------------------------------------
// NewStaticFileHandler
// ---------------------------------------------------------------------------

func TestNewStaticFileHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		root     string
		wantRoot string
	}{
		{
			name:     "explicit root",
			root:     "/tmp/foo",
			wantRoot: "/tmp/foo",
		},
		{
			name:     "empty root defaults to dot",
			root:     "",
			wantRoot: ".",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := NewStaticFileHandler(tc.root)
			require.NotNil(t, h)
			assert.Equal(t, tc.wantRoot, h.root)
		})
	}
}

// ---------------------------------------------------------------------------
// contentType
// ---------------------------------------------------------------------------

func TestContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		wantMIME string
	}{
		{"file.html", "text/html"},
		{"file.htm", "text/html"},
		{"file.css", "text/css"},
		{"file.js", "application/javascript"},
		{"file.json", "application/json"},
		{"file.png", "image/png"},
		{"file.jpg", "image/jpeg"},
		{"file.jpeg", "image/jpeg"},
		{"file.gif", "image/gif"},
		{"file.svg", "image/svg+xml"},
		{"file.ico", "image/x-icon"},
		{"file.txt", "text/plain"},
		{"file.pdf", "application/pdf"},
		{"file.HTML", "text/html"},  // case-insensitive extension
		{"file.CSS", "text/css"},    // case-insensitive extension
		{"file.xyz", "application/octet-stream"}, // unknown
		{"file", "application/octet-stream"},      // no extension
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			got := contentType(tc.path)
			assert.Equal(t, tc.wantMIME, got)
		})
	}
}

// ---------------------------------------------------------------------------
// resolvePath
// ---------------------------------------------------------------------------

func TestResolvePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	h := NewStaticFileHandler(dir)

	tests := []struct {
		name      string
		resource  string
		wantOK    bool
		// wantSuffix is a suffix of the resolved path (relative to dir)
		wantSuffix string
	}{
		{
			name:       "simple file path",
			resource:   "/index.html",
			wantOK:     true,
			wantSuffix: "index.html",
		},
		{
			name:       "nested path",
			resource:   "/static/app.js",
			wantOK:     true,
			wantSuffix: filepath.Join("static", "app.js"),
		},
		{
			name:     "directory traversal attempt with ..",
			resource: "/../../../etc/passwd",
			wantOK:   false,
		},
		{
			name:     "encoded traversal attempt",
			resource: "/%2e%2e/%2e%2e/etc/passwd",
			wantOK:   false,
		},
		{
			name:       "url-encoded space in path",
			resource:   "/my%20file.html",
			wantOK:     true,
			wantSuffix: "my file.html",
		},
		{
			name:     "invalid percent encoding",
			resource: "/%GG",
			wantOK:   false,
		},
		{
			name:       "root path resolves to handler root",
			resource:   "/",
			wantOK:     true,
			wantSuffix: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := h.resolvePath(tc.resource)
			assert.Equal(t, tc.wantOK, ok, "ok mismatch")
			if tc.wantOK && tc.wantSuffix != "" {
				assert.True(t,
					strings.HasSuffix(got, tc.wantSuffix),
					"resolved path %q should have suffix %q", got, tc.wantSuffix)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP – redirect behavior
// ---------------------------------------------------------------------------

func TestServeHTTP_Redirects(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	h := NewStaticFileHandler(dir)

	redirectPaths := []string{"/", "/index.htm", "/index"}

	for _, path := range redirectPaths {
		path := path
		t.Run("redirect"+path, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode,
				"expected 301 for path %q", path)
			assert.Equal(t, "/index.html", resp.Header.Get("Location"),
				"location header mismatch for path %q", path)
			assert.Equal(t, "close", resp.Header.Get("Connection"))
		})
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP – 404 behavior
// ---------------------------------------------------------------------------

func TestServeHTTP_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setupDir func(t *testing.T) string
		path     string
	}{
		{
			name: "file does not exist",
			setupDir: func(t *testing.T) string {
				return t.TempDir()
			},
			path: "/missing.html",
		},
		{
			name: "path is a directory",
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			path: "/subdir",
		},
		{
			name: "traversal path blocked",
			setupDir: func(t *testing.T) string {
				return t.TempDir()
			},
			path: "/../../../etc/passwd",
		},
		{
			name: "invalid percent encoding",
			setupDir: func(t *testing.T) string {
				return t.TempDir()
			},
			path: "/%ZZ",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := tc.setupDir(t)
			h := NewStaticFileHandler(dir)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)

			body := w.Body.String()
			assert.Contains(t, body, "404 Error: Page Not Found",
				"404 body should contain expected message")
			assert.Equal(t, "text/html", resp.Header.Get("Content-Type"))
			assert.Equal(t, "close", resp.Header.Get("Connection"))

			// Content-Length must match actual body length
			cl := resp.Header.Get("Content-Length")
			clInt, err := strconv.Atoi(cl)
			require.NoError(t, err, "Content-Length must be a valid integer")
			assert.Equal(t, len([]byte(http404Body)), clInt)
		})
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP – 200 with file content
// ---------------------------------------------------------------------------

func TestServeHTTP_200_FileServing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filename    string
		content     string
		requestPath string
		wantMIME    string
	}{
		{
			name:        "html file",
			filename:    "hello.html",
			content:     "<html><body>Hello</body></html>",
			requestPath: "/hello.html",
			wantMIME:    "text/html",
		},
		{
			name:        "css file",
			filename:    "style.css",
			content:     "body { color: red; }",
			requestPath: "/style.css",
			wantMIME:    "text/css",
		},
		{
			name:        "js file",
			filename:    "app.js",
			content:     "console.log('hi');",
			requestPath: "/app.js",
			wantMIME:    "application/javascript",
		},
		{
			name:        "text file",
			filename:    "readme.txt",
			content:     "Just a text file",
			requestPath: "/readme.txt",
			wantMIME:    "text/plain",
		},
		{
			name:        "unknown extension falls back to octet-stream",
			filename:    "data.bin",
			content:     "binary data here",
			requestPath: "/data.bin",
			wantMIME:    "application/octet-stream",
		},
		{
			name:        "url-encoded filename",
			filename:    "my file.html",
			content:     "<html>encoded</html>",
			requestPath: "/my%20file.html",
			wantMIME:    "text/html",
		},
		{
			name:        "empty file",
			filename:    "empty.txt",
			content:     "",
			requestPath: "/empty.txt",
			wantMIME:    "text/plain",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := makeTempDir(t, map[string]string{
				tc.filename: tc.content,
			})
			h := NewStaticFileHandler(dir)

			req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tc.wantMIME, resp.Header.Get("Content-Type"))
			assert.Equal(t, "close", resp.Header.Get("Connection"))

			body := w.Body.String()
			assert.Equal(t, tc.content, body)

			// Content-Length must match
			cl := resp.Header.Get("Content-Length")
			clInt, err := strconv.Atoi(cl)
			require.NoError(t, err, "Content-Length must be a valid integer")
			assert.Equal(t, len([]byte(tc.content)), clInt)
		})
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP – nested path file serving
// ---------------------------------------------------------------------------

func TestServeHTTP_NestedPath(t *testing.T) {
	t.Parallel()

	dir := makeTempDir(t, map[string]string{
		filepath.Join("static", "app.js"): "alert('nested');",
	})
	h := NewStaticFileHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/javascript", resp.Header.Get("Content-Type"))
	assert.Equal(t, "alert('nested');", w.Body.String())
}

// ---------------------------------------------------------------------------
// ServeHTTP – Connection header always set
// ---------------------------------------------------------------------------

func TestServeHTTP_ConnectionCloseAlwaysSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	h := NewStaticFileHandler(dir)

	// These cover all three branches: redirect, 404, 200
	paths := []struct {
		path    string
		setup   func()
	}{
		{path: "/"},
		{path: "/missing.html"},
	}

	for _, tc := range paths {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, "close", w.Result().Header.Get("Connection"))
		})
	}

	// Also test 200 branch
	t.Run("200 file", func(t *testing.T) {
		t.Parallel()
		dir2 := makeTempDir(t, map[string]string{"foo.txt": "bar"})
		h2 := NewStaticFileHandler(dir2)
		req := httptest.NewRequest(http.MethodGet, "/foo.txt", nil)
		w := httptest.NewRecorder()
		h2.ServeHTTP(w, req)
		assert.Equal(t, "close", w.Result().Header.Get("Connection"))
	})
}

// ---------------------------------------------------------------------------
// ServeHTTP – redirect takes precedence over file