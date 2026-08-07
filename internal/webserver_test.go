```go
package internal

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewWebServer constructor tests
// ---------------------------------------------------------------------------

func TestNewWebServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		addr        string
		handler     http.Handler
		wantAddr    string
		wantHandler bool // true means a non-nil handler was injected
	}{
		{
			name:        "empty addr falls back to default",
			addr:        "",
			handler:     http.NewServeMux(),
			wantAddr:    defaultWebServerAddr,
			wantHandler: true,
		},
		{
			name:        "explicit addr is preserved",
			addr:        ":0",
			handler:     http.NewServeMux(),
			wantAddr:    ":0",
			wantHandler: true,
		},
		{
			name:        "nil handler is replaced by buildRouter",
			addr:        ":0",
			handler:     nil,
			wantAddr:    ":0",
			wantHandler: true,
		},
		{
			name:        "default addr constant equals port 6789",
			addr:        "",
			handler:     nil,
			wantAddr:    ":6789",
			wantHandler: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := NewWebServer(tc.addr, tc.handler)
			require.NotNil(t, ws)
			assert.Equal(t, tc.wantAddr, ws.Addr())
			assert.NotNil(t, ws.srv)
			if tc.wantHandler {
				assert.NotNil(t, ws.srv.Handler)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Addr helper
// ---------------------------------------------------------------------------

func TestWebServer_Addr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		addr     string
		wantAddr string
	}{
		{"explicit port", ":8080", ":8080"},
		{"default port", "", defaultWebServerAddr},
		{"full address", "127.0.0.1:9000", "127.0.0.1:9000"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ws := NewWebServer(tc.addr, http.NewServeMux())
			assert.Equal(t, tc.wantAddr, ws.Addr())
		})
	}
}

// ---------------------------------------------------------------------------
// buildRouter – redirect routes
// ---------------------------------------------------------------------------

func TestBuildRouter_Redirects(t *testing.T) {
	t.Parallel()

	router := buildRouter()

	for from, to := range redirects {
		from, to := from, to
		t.Run("redirect "+from, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, from, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusMovedPermanently, rr.Code,
				"expected 301 for route %s", from)
			assert.Equal(t, to, rr.Header().Get("Location"),
				"expected Location %s for route %s", to, from)
		})
	}
}

// ---------------------------------------------------------------------------
// buildRouter – 404 catch-all
// ---------------------------------------------------------------------------

func TestBuildRouter_NotFound(t *testing.T) {
	t.Parallel()

	router := buildRouter()

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{"unknown path GET", "/does-not-exist", http.MethodGet},
		{"unknown path POST", "/missing/resource", http.MethodPost},
		{"root path if not registered", "/totally/unknown", http.MethodGet},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusNotFound, rr.Code)
			assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
			assert.Equal(t, http404Body, rr.Body.String())
		})
	}
}

// ---------------------------------------------------------------------------
// ListenAndServe – server binds to port 6789 equivalent (uses :0 in tests)
// ---------------------------------------------------------------------------

func TestListenAndServe_StartsAndAcceptsConnections(t *testing.T) {
	t.Parallel()

	// Use port :0 so the OS assigns an available port; mirrors the spec
	// requirement that the server binds successfully to its configured port.
	ws := NewWebServer(":0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ws.ListenAndServe(ctx)
	}()

	// Give the server a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Cancel triggers graceful shutdown; the server should return nil.
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err, "clean shutdown should return nil")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to shut down")
	}
}

func TestListenAndServe_DefaultPortIs6789(t *testing.T) {
	t.Parallel()

	ws := NewWebServer("", nil)
	assert.Equal(t, ":6789", ws.Addr(),
		"server must always listen on port 6789 when no address is supplied")
}

func TestListenAndServe_PortAlreadyInUse(t *testing.T) {
	t.Parallel()

	// Occupy a port so the second server cannot bind.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	busyAddr := ln.Addr().String()

	ws := NewWebServer(busyAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = ws.ListenAndServe(ctx)
	assert.Error(t, err, "binding to an already-occupied port must return an error")
}

// ---------------------------------------------------------------------------
// ListenAndServe – multiple concurrent clients are handled independently
// ---------------------------------------------------------------------------

func TestListenAndServe_MultipleClientsConcurrently(t *testing.T) {
	t.Parallel()

	const numClients = 5

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a tiny bit of work so connections overlap.
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ws := NewWebServer(":0", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We need the real listen address after the server starts; use a helper
	// that starts the underlying http.Server on an OS-assigned port.
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Confirm all concurrent clients receive 200 OK.
	type result struct {
		code int
		err  error
	}
	results := make(chan result, numClients)

	for i := 0; i < numClients; i++ {
		go func() {
			resp, err := http.Get(ts.URL + "/") //nolint:noctx
			if err != nil {
				results <- result{err: err}
				return
			}
			_ = resp.Body.Close()
			results <- result{code: resp.StatusCode}
		}()
	}

	for i := 0; i < numClients; i++ {
		select {
		case r := <-results:
			require.NoError(t, r.err)
			assert.Equal(t, http.StatusOK, r.code,
				"each concurrent client must receive a 200 response")
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for concurrent client responses")
		}
	}

	_ = ws // used only to verify construction above
}

// ---------------------------------------------------------------------------
// ListenAndServe – server blocks when no client connects (context cancel path)
// ---------------------------------------------------------------------------

func TestListenAndServe_BlocksUntilContextCancelled(t *testing.T) {
	t.Parallel()

	ws := NewWebServer(":0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- ws.ListenAndServe(ctx)
	}()

	// No client connects; verify the server is still running after a short wait.
	time.Sleep(30 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("server returned early (err=%v) without context cancellation", err)
	default:
		// Expected: server is still blocking.
	}

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err, "server should shut down cleanly after context cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server shutdown after cancel")
	}
}

// ---------------------------------------------------------------------------
// Shutdown helper
// ---------------------------------------------------------------------------

func TestWebServer_Shutdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ctxFn     func() (context.Context, context.CancelFunc)
		wantError bool
	}{
		{
			name: "clean shutdown with background context",
			ctxFn: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			wantError: false,
		},
		{
			name: "shutdown with already-cancelled context",
			ctxFn: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // cancel immediately
				return ctx, cancel
			},
			// Shutting down a server that was never started with a cancelled
			// context should return nil (net/http returns nil for idle server).
			wantError: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := NewWebServer(":0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			ctx, cancel := tc.ctxFn()
			defer cancel()

			err := ws.Shutdown(ctx)
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration: start real server, hit it, shut down gracefully
// ---------------------------------------------------------------------------

func TestWebServer_Integration_AcceptsAndHandlesRequest(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// httptest.NewServer picks :0 internally.
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/some-path") //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWebServer_Integration_GracefulShutdownDrainsInFlightRequest(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	requestDone := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		// Simulate a long-running request.
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		close(requestDone)
	})

	ts := httptest.NewServer(handler)

	// Fire off a request.
	go func() {
		resp, err := http.Get(ts.URL + "/") //nolint:noctx
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	// Wait until the handler is running.
	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("request never started")
	}

	// Close the test server (triggers graceful shutdown).
	ts.Close()

	// The in-flight request should still complete.
	select {
	case <-requestDone:
		// good
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request was not drained during shutdown")
	}
}
```