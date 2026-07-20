package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// DefaultPort is the TCP port the web server listens on, matching the
// original Java WebServer which bound to port 6789.
const DefaultPort = 6789

// RunWebServer creates a Server bound to the given port and serves client
// connections until the provided context is cancelled.
//
// This replaces the thread-per-connection main loop of the original Java
// WebServer. Concurrency is handled inside Server.ListenAndServe (see
// internal/connection.go), which spawns a goroutine per accepted connection
// instead of a Java Thread.
//
// MIGRATION_NOTE: The original Java main() looped forever via `while(true)`
// with no shutdown mechanism. Here we accept a context so callers can
// terminate the server gracefully. The infinite-listen behaviour is preserved
// as long as the context is never cancelled.
func RunWebServer(ctx context.Context, port int) error {
	addr := fmt.Sprintf(":%d", port)
	srv, err := NewServer(addr)
	if err != nil {
		return fmt.Errorf("creating server on port %d: %w", port, err)
	}

	log.Printf("Listening for connections on port %d...", port)

	if err := srv.ListenAndServe(ctx, addr); err != nil {
		return fmt.Errorf("serving on port %d: %w", port, err)
	}
	return nil
}

// Main is the entry point equivalent to the Java WebServer.main method.
//
// It wires up signal-based cancellation for graceful shutdown (SIGINT,
// SIGTERM) and starts the server on DefaultPort. It returns an error rather
// than terminating the process directly, so callers in cmd/ can decide how to
// report failures and set exit codes.
//
// MIGRATION_NOTE: In idiomatic Go the actual process entry point should live
// in a cmd/ package (e.g. cmd/webserver/main.go) that calls internal.Main and
// exits accordingly. This function contains the orchestration logic that the
// Java static main() held.
func Main() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return RunWebServer(ctx, DefaultPort)
}