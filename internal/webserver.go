package internal

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// DefaultServerPort is the TCP port the original Java WebServer listened on.
//
// MIGRATION_NOTE: The original Java WebServer opened a raw java.net.ServerSocket
// on port 6789 and hand-rolled a thread-per-connection accept loop. In
// idiomatic Go we do NOT re-implement a raw accept loop with manual HTTP
// parsing; instead we run net/http's *http.Server, which internally performs a
// resilient accept loop (it already sleeps/retries on transient net.Errors
// rather than crashing) and spawns a goroutine per connection. This preserves
// the original concurrency model (one lightweight worker per connection) while
// delegating request parsing and response framing to the standard library.
const DefaultServerPort = "6789"

const (
	// readTimeout bounds how long a client may take to send its request.
	readTimeout = 15 * time.Second
	// writeTimeout bounds how long the server may take to write a response.
	writeTimeout = 15 * time.Second
	// idleTimeout bounds how long an idle keep-alive connection is kept open.
	idleTimeout = 60 * time.Second
)

// WebServer wraps an *http.Server configured to reproduce the behavior of the
// original Java WebServer: it listens indefinitely for client connections and
// serves each one concurrently.
type WebServer struct {
	httpServer *http.Server
	addr       string
}

// NewWebServer constructs a WebServer bound to the given address (for example
// ":6789"). The provided root directory is used by the StaticFileHandler to
// resolve static files, matching the original server's document root behavior.
//
// Passing an empty addr defaults to the original port 6789.
func NewWebServer(addr, root string) *WebServer {
	if addr == "" {
		addr = ":" + DefaultServerPort
	}

	handler := NewStaticFileHandler(root)

	return &WebServer{
		addr: addr,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      buildWebServerRouter(handler),
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
		},
	}
}

// buildWebServerRouter wires the StaticFileHandler into a chi router.
//
// The router registers a catch-all route so that redirect lookup and path
// resolution happen inside StaticFileHandler.ServeHTTP, which operates on the
// raw request resource (including any query string) exactly as required to
// preserve routing and 404 parity with the original Java implementation.
func buildWebServerRouter(handler *StaticFileHandler) http.Handler {
	r := chi.NewRouter()
	// Catch-all: every request (including "/", "/index.htm", "/index", and
	// arbitrary static paths) is dispatched to the StaticFileHandler, which
	// owns the redirect map and file-serving logic.
	r.Handle("/*", handler)
	return r
}

// ListenAndServe starts the server and blocks, serving connections until the
// provided context is cancelled or a non-recoverable error occurs. On context
// cancellation it performs a graceful shutdown.
//
// It returns nil on a clean shutdown and a non-nil error otherwise.
func (s *WebServer) ListenAndServe(ctx context.Context) error {
	// Ensure we hold the listener explicitly so we log the exact bound address,
	// mirroring the original "Listening for connections on port ..." message.
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	log.Info().Str("addr", ln.Addr().String()).Msg("listening for connections")

	serveErr := make(chan error, 1)
	go func() {
		// Serve runs the resilient accept loop internally, spawning a goroutine
		// per accepted connection (the Go equivalent of the Java
		// thread-per-connection model).
		serveErr <- s.httpServer.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutCtx); err != nil {
			return err
		}
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
