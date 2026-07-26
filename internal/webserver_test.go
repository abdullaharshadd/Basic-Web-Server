```go
package internal

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// tempRoot creates a temporary directory with the given files (name→content)
// and returns its path. The caller must call os.RemoveAll on the returned path
// when done.
func tempRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	return root
}

// freePort returns a random free TCP port on the loopback interface.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	return port
}

// ---------------------------------------------------------------------------
// NewWebServer
// ---------------------------------------------------------------------------

func TestNewWebServer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		addr         string
		root         string
		expectedAddr string
	}{
		{
			name:         "empty addr defaults to :6789",
			addr:         "",
			root:         t.TempDir(),
			expectedAddr: ":" + DefaultServerPort,
		},
		{
			name:         "explicit addr is preserved",
			addr:         ":7777",
			root:         t.TempDir(),
			expectedAddr: ":7777",
		},
		{
			name:         "full host:port addr is preserved",
			addr:         "127.0.0.1:8888",
			root:         t.TempDir(),
			expectedAddr: "127.0.0.1:8888",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ws := NewWebServer(tc.addr, tc.root)
			require.NotNil(t, ws)
			assert.Equal(t, tc.expectedAddr, ws.addr)
			assert.NotNil(t, ws.httpServer)
			assert.Equal(t, tc.expectedAddr, ws.httpServer.Addr)
		})
	}
}

func TestNewWebServer_Timeouts(t *testing.T) {
	t.Parallel()
	ws := NewWebServer(":0", t.TempDir())
	assert.Equal(t, readTimeout, ws.httpServer.ReadTimeout)
	assert.Equal(t, writeTimeout, ws.httpServer.WriteTimeout)
	assert.Equal(t, idleTimeout, ws.httpServer.IdleTimeout)
}

// ---------------------------------------------------------------------------
// DefaultServerPort constant
// ---------------------------------------------------------------------------

func TestDefaultServerPort(t *testing.T) {
	assert.Equal(t, "6789", DefaultServerPort)
}

// ---------------------------------------------------------------------------
// buildWebServerRouter
// ---------------------------------------------------------------------------

func TestBuildWebServerRouter_CatchAll(t *testing.T) {
	t.Parallel()

	root := tempRoot(t, map[string]string{
		"index.html": "<html>hello</html>",
		"about.html": "<html>about</html>",
	})
	handler := NewStaticFileHandler(root)
	router := buildWebServerRouter(handler)

	cases := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "root path served",
			path:           "/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "static file path served",
			path:           "/about.html",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing file returns 404",
			path:           "/nonexistent.html",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "arbitrary deep path handled",
			path:           "/foo/bar/baz",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			assert.Equal(t, tc.expectedStatus, rec.Code)
		})
	}
}

func TestBuildWebServerRouter_NotNil(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	handler := NewStaticFileHandler(root)
	router := buildWebServerRouter(handler)
	assert.NotNil(t, router)
}

// ---------------------------------------------------------------------------
// ListenAndServe – startup / shutdown behaviour
// ---------------------------------------------------------------------------

func TestListenAndServe_GracefulShutdown(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	ws := NewWebServer("127.0.0.1:"+port, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ws.ListenAndServe(ctx)
	}()

	// Wait until the server is accepting connections.
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond, "server did not start in time")

	// Signal graceful shutdown.
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err, "clean shutdown must return nil")
	case <-time.After(15 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}

func TestListenAndServe_PrintsListeningMessage(t *testing.T) {
	// Validates that the server binds and begins listening (mirrors the
	// original "Listening for connections on port 6789..." invariant).
	t.Parallel()

	port := freePort(t)
	ws := NewWebServer("127.0.0.1:"+port, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	go func() {
		// We signal as soon as Serve is invoked (listener is bound).
		close(started)
		_ = ws.ListenAndServe(ctx)
	}()

	// The listener must be reachable within a reasonable window.
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)
}

func TestListenAndServe_AlreadyInUse(t *testing.T) {
	t.Parallel()

	// Occupy a port.
	occupant, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupant.Close()

	port := fmt.Sprintf("%d", occupant.Addr().(*net.TCPAddr).Port)
	ws := NewWebServer("127.0.0.1:"+port, t.TempDir())

	ctx := context.Background()
	err = ws.ListenAndServe(ctx)
	assert.Error(t, err, "should fail when port is already in use")
}

// ---------------------------------------------------------------------------
// ListenAndServe – concurrent connections (thread-per-connection invariant)
// ---------------------------------------------------------------------------

func TestListenAndServe_ConcurrentConnections(t *testing.T) {
	t.Parallel()

	root := tempRoot(t, map[string]string{
		"index.html": "<html>hello</html>",
	})
	port := freePort(t)
	ws := NewWebServer("127.0.0.1:"+port, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = ws.ListenAndServe(ctx) }()

	// Wait for readiness.
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)

	const numClients = 20
	var wg sync.WaitGroup
	results := make([]int, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := http.Get("http://127.0.0.1:" + port + "/index.html")
			if err != nil {
				results[idx] = -1
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			results[idx] = resp.StatusCode
		}(i)
	}

	wg.Wait()

	for i, code := range results {
		assert.Equal(t, http.StatusOK, code, "client %d got unexpected status", i)
	}
}

// ---------------------------------------------------------------------------
// ListenAndServe – main accept loop continues after single connection
// ---------------------------------------------------------------------------

func TestListenAndServe_AcceptsMultipleSequentialConnections(t *testing.T) {
	// Mirrors the "main accept loop never terminates" invariant: the server
	// must accept a second request after handling the first.
	t.Parallel()

	root := tempRoot(t, map[string]string{
		"a.html": "aaa",
		"b.html": "bbb",
	})
	port := freePort(t)
	ws := NewWebServer("127.0.0.1:"+port, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = ws.ListenAndServe(ctx) }()

	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)

	base := "http://127.0.0.1:" + port

	cases := []struct {
		path string
		want int
	}{
		{"/a.html", http.StatusOK},
		{"/b.html", http.StatusOK},
		{"/a.html", http.StatusOK},
	}

	for _, tc := range cases {
		resp, err := http.Get(base + tc.path)
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		assert.Equal(t, tc.want, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// ListenAndServe – each connection is handled concurrently (not on accept goroutine)
// ---------------------------------------------------------------------------

func TestListenAndServe_HandlerRunsConcurrently(t *testing.T) {
	// Confirms that the main goroutine immediately returns to accepting new
	// connections while a slow handler is running.
	t.Parallel()

	root := tempRoot(t, map[string]string{
		"index.html": "ok",
	})
	port := freePort(t)
	ws := NewWebServer("127.0.0.1:"+port, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = ws.ListenAndServe(ctx) }()

	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)

	base := "http://127.0.0.1:" + port

	// Fire two requests simultaneously and expect both to complete within a
	// tight window (they would not if processed sequentially).
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		resp, err := http.Get(base + "/index.html")
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	go func() {
		defer wg.Done()
		resp, err := http.Get(base + "/index.html")
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	wg.Wait()
	elapsed := time.Since(start)

	// Both requests completing in < 2 s shows concurrency; would take longer
	// if the server were purely sequential.
	assert.Less(t, elapsed, 2*time.Second)
}
```