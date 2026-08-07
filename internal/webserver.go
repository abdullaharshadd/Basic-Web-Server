package internal

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// defaultWebServerAddr is the listen address preserved from the original
// Java WebServer, which bound a raw ServerSocket to port 6789.
const defaultWebServerAddr = ":6789"

// WebServer is a small HTTP server that serves web content over its
// configured listen address.
//
// It replaces the original Java WebServer class, which used a raw
// ServerSocket with a thread-per-connection concurrency model. In Go the
// standard library's net/http server already manages a goroutine per
// connection internally, so the explicit accept loop and manual thread
// spawning from the Java source are no longer necessary.
//
// MIGRATION_NOTE: the Java implementation delegated each accepted socket to
// a Connection Runnable. Here the routing/handler behaviour that lived in
// Connection is expressed through the chi router built by buildRouter, and
// http.Server owns connection lifecycle and concurrency.
type WebServer struct {
	srv *http.Server
}

// NewWebServer constructs a WebServer that listens on the given address and
// serves requests using the provided handler. If addr is empty the default
// address (:6789) is used, and if handler is nil the router built by
// buildRouter is used.
func NewWebServer(addr string, handler http.Handler) *WebServer {
	if addr == "" {
		addr = defaultWebServerAddr
	}
	if handler == nil {
		handler = buildRouter()
	}
	return &WebServer{
		srv: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Addr returns the address the server is configured to listen on.
func (w *WebServer) Addr() string {
	return w.srv.Addr
}

// ListenAndServe starts the server and blocks until it is shut down or the
// provided context is cancelled. It returns nil on a clean shutdown and a
// non-nil error otherwise.
func (w *WebServer) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		log.Info().Str("addr", w.srv.Addr).Msg("listening for connections")
		if err := w.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := w.srv.Shutdown(shutCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the server, waiting for in-flight requests to
// complete or the given context to be cancelled.
func (w *WebServer) Shutdown(ctx context.Context) error {
	return w.srv.Shutdown(ctx)
}

// BuildRouter constructs and returns the chi router that serves the redirect
// and static-content routes migrated from the original Connection handler.
// It is the exported entry point used by cmd/server/main.go.
func BuildRouter() http.Handler {
	return buildRouter()
}

// buildRouter constructs the chi router that serves the redirect and
// static-content routes migrated from the original Connection handler.
//
// MIGRATION_NOTE: this wires RegisterRoutes from connection.go, which
// registers the catch-all GET /* handler (SendResponse) that handles
// redirects, 404s, and static file serving.
func buildRouter() http.Handler {
	r := chi.NewRouter()
	RegisterRoutes(r)
	return r
}
