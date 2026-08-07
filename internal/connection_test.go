```go
package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newRouter builds a chi router wired with RegisterRoutes and returns it.
func newRouter() chi.Router {
	r := chi.NewRouter()
	RegisterRoutes(r)
	return r
}

// withTempFile creates a temporary directory, writes content to a file with
// the given name inside it, changes the working directory to that temp dir for
// the duration of the test, and restores the original working directory via
// t.Cleanup. It returns the temp directory path.
func withTempFile(t *testing.T, name, content string) string {
	t.Helper()

	origWD, err := os.Getwd()
	require.NoError(t, err, "os.Getwd should succeed")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))

	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	return dir
}

// withEmptyDir changes the working directory to an empty temp dir.
func withEmptyDir(t *testing.T) string {
	t.Helper()

	origWD, err := os.Getwd()
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	return dir
}

// ---------------------------------------------------------------------------
// redirects map – constructor-equivalent invariant tests
// ---------------------------------------------------------------------------

func TestRedirectMap_DefaultRules(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		target string
	}{
		{
			name:   "root redirects to index.html",
			path:   "/",
			target: "/index.html",
		},
		{
			name:   "/index.htm redirects to index.html",
			path:   "/index.htm",
			target: "/index.html",
		},
		{
			name:   "/index redirects to index.html",
			path:   "/index",
			target: "/index.html",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := redirects[tc.path]
			assert.True(t, ok, "redirect map must contain key %q", tc.path)
			assert.Equal(t, tc.target, got)
		})
	}
}

func TestRedirectMap_OnlyThreeDefaultEntries(t *testing.T) {
	assert.Len(t, redirects, 3, "redirect map must have exactly three default entries")
}

// ---------------------------------------------------------------------------
// write301
// ---------------------------------------------------------------------------

func TestWrite301(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "redirect to index.html",
			target: "/index.html",
		},
		{
			name:   "redirect to arbitrary path",
			target: "/some/other/path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			write301(w, tc.target)

			res := w.Result()
			assert.Equal(t, http.StatusMovedPermanently, res.StatusCode)
			assert.Equal(t, tc.target, res.Header.Get("Location"))
		})
	}
}

// ---------------------------------------------------------------------------
// write404
// ---------------------------------------------------------------------------

func TestWrite404(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "404 response has correct status and body"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			write404(w)

			res := w.Result()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
			assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
			assert.Contains(t, string(body), "404 Error: Page Not Found")
			assert.Equal(t, http404Body, string(body))
		})
	}
}

// ---------------------------------------------------------------------------
// SendResponse via httptest – redirect behavior
// ---------------------------------------------------------------------------

func TestSendResponse_Redirects(t *testing.T) {
	tests := []struct {
		name           string
		requestPath    string
		expectedTarget string
	}{
		{
			name:           "root path redirects to /index.html",
			requestPath:    "/",
			expectedTarget: "/index.html",
		},
		{
			name:           "/index.htm redirects to /index.html",
			requestPath:    "/index.htm",
			expectedTarget: "/index.html",
		},
		{
			name:           "/index redirects to /index.html",
			requestPath:    "/index",
			expectedTarget: "/index.html",
		},
	}

	// Redirect tests do not read the filesystem, so no temp dir is needed.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
			w := httptest.NewRecorder()

			SendResponse(w, req)

			res := w.Result()
			assert.Equal(t, http.StatusMovedPermanently, res.StatusCode,
				"expected 301 for path %q", tc.requestPath)
			assert.Equal(t, tc.expectedTarget, res.Header.Get("Location"),
				"Location header mismatch for path %q", tc.requestPath)
		})
	}
}

// ---------------------------------------------------------------------------
// SendResponse via httptest – 404 behavior
// ---------------------------------------------------------------------------

func TestSendResponse_404(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		setup       func(t *testing.T)
	}{
		{
			name:        "missing file returns 404",
			requestPath: "/nonexistent.html",
			setup: func(t *testing.T) {
				withEmptyDir(t)
			},
		},
		{
			name:        "directory path treated as 404",
			requestPath: "/subdir",
			setup: func(t *testing.T) {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))

				origWD, err := os.Getwd()
				require.NoError(t, err)
				require.NoError(t, os.Chdir(dir))
				t.Cleanup(func() { _ = os.Chdir(origWD) })
			},
		},
		{
			name:        "deeply nested missing file returns 404",
			requestPath: "/a/b/c/missing.txt",
			setup: func(t *testing.T) {
				withEmptyDir(t)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)

			req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
			w := httptest.NewRecorder()

			SendResponse(w, req)

			res := w.Result()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
			assert.Contains(t, string(body), "404 Error: Page Not Found")
		})
	}
}

// ---------------------------------------------------------------------------
// SendResponse via httptest – 200 / file serving behavior
// ---------------------------------------------------------------------------

func TestSendResponse_ServeFile(t *testing.T) {
	tests := []struct {
		name            string
		fileName        string
		fileContent     string
		requestPath     string
		expectedStatus  int
		expectedBodySub string
		expectedCT      string // partial content-type match; empty means skip
	}{
		{
			name:            "existing html file returns 200 with correct body",
			fileName:        "index.html",
			fileContent:     "<html><body>Hello</body></html>",
			requestPath:     "/index.html",
			expectedStatus:  http.StatusOK,
			expectedBodySub: "Hello",
			expectedCT:      "text/html",
		},
		{
			name:            "existing txt file returns 200 with correct body",
			fileName:        "readme.txt",
			fileContent:     "plain text content",
			requestPath:     "/readme.txt",
			expectedStatus:  http.StatusOK,
			expectedBodySub: "plain text content",
			expectedCT:      "text/plain",
		},
		{
			name:            "existing css file returns 200",
			fileName:        "style.css",
			fileContent:     "body { color: red; }",
			requestPath:     "/style.css",
			expectedStatus:  http.StatusOK,
			expectedBodySub: "color: red",
			expectedCT:      "text/css",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withTempFile(t, tc.fileName, tc.fileContent)

			req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
			w := httptest.NewRecorder()

			SendResponse(w, req)

			res := w.Result()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, res.StatusCode)
			assert.Contains(t, string(body), tc.expectedBodySub)
			if tc.expectedCT != "" {
				assert.Contains(t, res.Header.Get("Content-Type"), tc.expectedCT)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SendResponse – precedence: redirect before file existence
// ---------------------------------------------------------------------------

func TestSendResponse_RedirectTakesPrecedenceOverFile(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		targetPath  string
	}{
		{
			// Even if an "index.html" file exists in the working dir,
			// a request for "/" must still redirect to "/index.html".
			name:        "root redirect even when index.html file exists",
			requestPath: "/",
			targetPath:  "/index.html",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Put an index.html in the wd – redirect must still take priority.
			withTempFile(t, "index.html", "<html>exists</html>")

			req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
			w := httptest.NewRecorder()

			SendResponse(w, req)

			res := w.Result()
			assert.Equal(t, http.StatusMovedPermanently, res.StatusCode)
			assert.Equal(t, tc.targetPath, res.Header.Get("Location"))
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterRoutes – integration through chi router
// ---------------------------------------------------------------------------

func TestRegisterRoutes_Integration(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T)
		requestPath    string
		expectedStatus int
		locationHeader string
		bodyContains   string
	}{
		{
			name: "router returns 301 for root",
			setup: func(t *testing.T) {
				withEmptyDir(t)
			},
			requestPath:    "/",
			expectedStatus: http.StatusMovedPermanently,
			locationHeader: "/index.html",
		},
		{
			name: "router returns 301 for /index.htm",
			setup: func(t *testing.T) {
				withEmptyDir(t)
			},
			requestPath:    "/index.htm",
			expectedStatus: http.StatusMovedPermanently,
			locationHeader: "/index.html",
		},
		{
			name: "router returns 301 for /index",
			setup: func(t *testing.T) {
				withEmptyDir(t)
			},
			requestPath:    "/index",
			expectedStatus: http.StatusMovedPermanently,
			locationHeader: "/index.html",
		},
		{
			name: "router returns 404 for missing resource",
			setup: func(t *testing.T) {
				withEmptyDir(t)
			},
			requestPath:    "/missing.html",
			expectedStatus: http.StatusNotFound,
			bodyContains:   "404 Error: Page Not Found",
		},
		{
			name: "router returns 200 for existing file",
			setup: func(t *testing.T) {
				withTempFile(t, "hello.html", "<p>hi</p>")
			},
			requestPath:    "/hello.html",
			expectedStatus: http.StatusOK,
			bodyContains:   "hi",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)

			router := newRouter()
			ts := httptest.NewServer(router)
			t.Cleanup(ts.Close)

			// Use a non-following client so we observe the 301 directly.
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			resp, err := client.Get(ts.URL + tc.requestPath)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.locationHeader != "" {
				assert.Equal(t, tc.locationHeader, resp.Header.Get("Location"))
			}
			if tc.bodyContains != "" {
				assert.Contains(t, string(body), tc.bodyContains)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// serveFile – direct unit tests
// ---------------------------------------------------------------------------

func TestServeFile(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) string // returns filePath
		expectedStatus int
		bodyContains   string
	}{
		{
			name: "existing file returns 200 with content",
			setup: func(t *testing.T) string {
				dir := withTempFile(t, "data.html", "<h1>data</h1>")
				return filepath.Join(dir, "data.html")
			},
			expectedStatus: http.StatusOK,
			bodyContains:   "data",
		},
		{
			name: "non-existent file returns 404",
			setup: func(t *testing.T) string {
				withEmptyDir(t)
				return "./ghost.html"
			},
			expectedStatus: http.StatusNotFound,
			bodyContains:   "404 Error: Page Not Found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t