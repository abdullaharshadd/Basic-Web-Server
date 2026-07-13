package internal

import (
	"context"
	"fmt"
	"net"
)

// DefaultAddr is the default listen address for the web server. It preserves
// the original Java implementation's port 6789 while binding to all
// interfaces, matching the behavior of Java's ServerSocket(int) constructor.
const DefaultAddr = ":6789"

// WebServer is a minimal raw-socket HTTP server. It listens on a TCP address
// and spawns a new goroutine per incoming client connection to handle HTTP
// requests, mirroring the thread-per-connection model of the original Java
// implementation.
type WebServer struct {
	addr string
}

// NewWebServer constructs a WebServer bound to the given address. Pass an
// empty string to use DefaultAddr (":6789").
func NewWebServer(addr string) *WebServer {
	if addr == "" {
		addr = DefaultAddr
	}
	return &WebServer{addr: addr}
}

// ListenAndServe creates the listening socket and accepts client connections
// indefinitely, launching a goroutine to handle each one. It blocks until the
// listener fails or the provided context is cancelled.
//
// This is the Go equivalent of the original Java main() accept loop. Rather
// than looping forever, it honours ctx cancellation for graceful shutdown.
func (s *WebServer) ListenAndServe(ctx context.Context) error {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.addr, err)
	}
	defer listener.Close()

	fmt.Printf("Listening for connections on %s...\r\n", s.addr)

	// Close the listener when the context is cancelled so the blocking
	// Accept call below unblocks and returns an error, allowing graceful
	// shutdown.
	//
	// MIGRATION_NOTE: The Java version had no shutdown mechanism (infinite
	// while(true) loop). Context-based cancellation is added here as the
	// idiomatic Go replacement; review whether the caller wires up an
	// appropriate cancellation signal (e.g. on SIGINT/SIGTERM).
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// If the context was cancelled, treat the resulting Accept
			// error as a clean shutdown rather than a failure.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("accepting connection: %w", err)
		}

		// Hand off to a per-connection goroutine, mirroring the Java
		// thread-per-connection model (new Thread(new Connection(...))).
		connection := NewConnection(conn)
		go connection.Handle(ctx)

		fmt.Printf("New connection on %s...\r\n", s.addr)
	}
}
