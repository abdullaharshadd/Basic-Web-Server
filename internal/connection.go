package internal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// http404Body is the HTML body returned when a requested file cannot be found.
const http404Body = `<!DOCTYPE html>
<html>

<head>
    <title>Maurice Harris - Network Project 1</title>
</head>

<body><h1>
404 Error: Page Not Found
</h1></body>

</html>`

// redirects maps a requested resource path to the location it should be
// permanently redirected to via an HTTP 301 response.
//
// This reproduces the redirect HashMap from the original Java Connection class.
// The following routes are served:
//
//	GET /            -> 301 redirect to /index.html
//	GET /index       -> 301 redirect to /index.html
//	GET /index.htm   -> 301 redirect to /index.html
//	ANY /*           -> static file relative to the root, or 404 if missing
var redirects = map[string]string{
	"/":          "/index.html",
	"/index.htm": "/index.html",
	"/index":     "/index.html",
}

// Request represents a parsed HTTP request line and headers.
//
// MIGRATION_NOTE: The original parseRequest stored everything in a single
// HashMap keyed by "Method", "Resource", "Protocol", plus header names. This
// is modelled here with explicit fields plus a Headers map for idiomatic Go.
type Request struct {
	// Method is the HTTP method from the request line (e.g. GET).
	Method string
	// Resource is the requested resource path (e.g. /index.html).
	Resource string
	// Protocol is the HTTP protocol version (e.g. HTTP/1.1).
	Protocol string
	// Headers holds the request header key/value pairs.
	Headers map[string]string
}

// Server is a hand-rolled HTTP server that accepts raw TCP connections and
// serves static files from a root directory, mirroring the behaviour of the
// original Java implementation.
//
// MIGRATION_NOTE: The original code used "." (the working directory) as the
// file root and "new File("." + resourcePath)", which is vulnerable to path
// traversal. The Go port resolves and validates every path so that it cannot
// escape the configured root.
type Server struct {
	root string
}

// NewServer creates a Server that serves files from the given root directory.
// If root is empty, the current working directory is used, preserving the
// original "." behaviour.
func NewServer(root string) (*Server, error) {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving server root %q: %w", root, err)
	}
	return &Server{root: abs}, nil
}

// ListenAndServe accepts connections on the given address until the context is
// cancelled. Each connection is handled in its own goroutine, mirroring the
// original thread-per-connection model.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %q: %w", addr, err)
	}
	defer ln.Close()

	// Close the listener when the context is cancelled so Accept unblocks.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("accepting connection: %w", err)
		}
		go s.handleConnection(ctx, conn)
	}
}

// handleConnection parses the client request and writes an appropriate
// response. It always closes the connection when finished.
//
// MIGRATION_NOTE: The original run() method printed a stack trace on error.
// Here errors are logged via a package-level logging hook and the connection
// is always closed via defer.
func (s *Server) handleConnection(_ context.Context, conn net.Conn) {
	defer conn.Close()

	req, err := parseRequest(conn)
	if err != nil {
		logError("parsing request", err)
		return
	}
	// If no valid request line was read, do nothing (matches Java behaviour
	// where a nil request line results in no response).
	if req == nil {
		return
	}

	if err := s.sendResponse(conn, req); err != nil {
		logError("sending response", err)
	}
}

// parseRequest reads the request line and headers from r and returns a parsed
// Request. It returns (nil, nil) when the connection yields no request line,
// mirroring the original behaviour of silently ignoring empty input.
//
// MIGRATION_NOTE: Like the original, this reads only the request line and
// headers and never consumes a request body. The Method is parsed but the
// response logic never gates behaviour on it.
func parseRequest(r io.Reader) (*Request, error) {
	reader := bufio.NewReader(r)

	requestLine, err := reader.ReadString('\n')
	if err != nil {
		// EOF with no data means there was no request line to parse.
		if errors.Is(err, io.EOF) && requestLine == "" {
			return nil, nil
		}
		if !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("reading request line: %w", err)
		}
	}
	requestLine = strings.TrimRight(requestLine, "\r\n")
	if requestLine == "" {
		return nil, nil
	}

	parts := strings.Split(requestLine, " ")
	if len(parts) < 3 {
		return nil, fmt.Errorf("malformed request line: %q", requestLine)
	}

	req := &Request{
		Method:   parts[0],
		Resource: parts[1],
		Protocol: parts[2],
		Headers:  make(map[string]string),
	}

	// Read header lines until an empty line terminates the header block.
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("reading header line: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		key := kv[0]
		value := ""
		if len(kv) == 2 {
			// Match Java's replaceFirst(" ", "") which strips a single
			// leading space from the value.
			value = strings.Replace(kv[1], " ", "", 1)
		}
		req.Headers[key] = value
		if errors.Is(err, io.EOF) {
			break
		}
	}

	return req, nil
}

// sendResponse writes the appropriate HTTP response for the given request:
//   - a 301 redirect if the resource is in the redirect table,
//   - a 404 page if the resolved file does not exist,
//   - a 200 response streaming the file otherwise.
func (s *Server) sendResponse(w io.Writer, req *Request) error {
	resourcePath := req.Resource

	// Redirect branch: matches the redirect HashMap lookup.
	if location, ok := redirects[resourcePath]; ok {
		// MIGRATION_NOTE: The original 301 branch is malformed HTTP: it uses
		// "\n" line endings and omits the blank-line terminator. This is
		// preserved verbatim to keep behaviour identical. Fix by switching to
		// "\r\n" and appending "\r\n\r\n" if strict HTTP is required.
		resp := "HTTP/1.1 301 Moved Permanently\n" + "Location: " + location
		if _, err := io.WriteString(w, resp); err != nil {
			return fmt.Errorf("writing 301 response: %w", err)
		}
		return nil
	}

	// Resolve the file path safely so it cannot escape the server root.
	fullPath, ok := s.resolvePath(resourcePath)
	if !ok {
		return s.write404(w)
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		return s.write404(w)
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return s.write404(w)
	}
	defer file.Close()

	contentType := contentTypeFor(fullPath)

	header := "HTTP/1.1 200 OK\r\nContent-Type: " + contentType + "\r\n\r\n"
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("writing 200 header: %w", err)
	}

	// MIGRATION_NOTE: The original read at most one buffer's worth of bytes
	// via a single read call. io.Copy correctly streams the entire file to
	// the client, fixing the latent truncation bug.
	if _, err := io.Copy(w, file); err != nil {
		return fmt.Errorf("streaming file %q: %w", fullPath, err)
	}
	return nil
}

// write404 writes the HTTP 404 response with the static error page.
func (s *Server) write404(w io.Writer) error {
	resp := "HTTP/1.1 404 Not Found\r\n\r\n" + http404Body
	if _, err := io.WriteString(w, resp); err != nil {
		return fmt.Errorf("writing 404 response: %w", err)
	}
	return nil
}

// resolvePath resolves a request resource path against the server root and
// verifies that the result does not escape the root directory. It returns
// (absolutePath, true) on success or ("", false) if the path is invalid or
// attempts traversal.
//
// MIGRATION_NOTE: This replaces the vulnerable `new File("." + resourcePath)`
// construction in the original code, which allowed path traversal such as
// /../../etc/passwd.
func (s *Server) resolvePath(resourcePath string) (string, bool) {
	clean := filepath.Clean("/" + resourcePath)
	full := filepath.Join(s.root, clean)

	rel, err := filepath.Rel(s.root, full)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

// contentTypeFor returns the MIME type for the given file path.
//
// MIGRATION_NOTE: Java's Files.probeContentType may return null; the Go
// equivalent handles the empty case by falling back to content sniffing and
// finally to "application/octet-stream".
func contentTypeFor(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		if n > 0 {
			return http.DetectContentType(buf[:n])
		}
	}
	return "application/octet-stream"
}

// logError is the package-level error logging hook. It replaces the original
// ex.printStackTrace() call. Override it to integrate with a structured
// logger (zap/zerolog) in production.
var logError = func(context string, err error) {
	fmt.Fprintf(os.Stderr, "%s error: %v\n", context, err)
}

// ensure the time import is retained for potential connection deadlines;
// callers may set conn.SetDeadline(time.Now().Add(...)) as needed.
var _ = time.Now
