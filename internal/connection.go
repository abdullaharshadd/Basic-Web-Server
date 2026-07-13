// Package internal implements a minimal HTTP/1.1 server worker that parses a
// client's raw request from a socket and serves static files with support for
// 301 redirects, 404 not found, and 200 OK responses.
//
// This is a migration of the original Java Connection class. It uses plain
// net.Conn networking; there is no framework involved. Concurrency is
// one-goroutine-per-connection: the Java Runnable.run() maps to Handle, which
// is intended to be launched with `go conn.Handle(ctx)`.
package internal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// http404Body is the static HTML body served for 404 responses. It preserves
// the exact markup from the original Java implementation.
const http404Body = `<!DOCTYPE html>
<html>

<head>
    <title>Maurice Harris - Network Project 1</title>
</head>

<body><h1>
404 Error: Page Not Found
</h1></body>

</html>`

// defaultRedirects maps requested URLs to the URL they should be permanently
// redirected to. The key is the URL the user requested; the value is the
// destination location.
func defaultRedirects() map[string]string {
	return map[string]string{
		"/":          "/index.html",
		"/index.htm": "/index.html",
		"/index":     "/index.html",
	}
}

// Request holds the parsed fields of a client HTTP request. The request line
// values (Method, Resource, Protocol) are stored alongside the header fields.
type Request struct {
	// Fields contains every parsed field: the request-line values under the
	// keys "Method", "Resource", "Protocol", plus each header keyed by its
	// header name.
	Fields map[string]string
}

// Method returns the HTTP method and whether it was present.
func (r *Request) Method() (string, bool) {
	v, ok := r.Fields["Method"]
	return v, ok
}

// Resource returns the requested resource path and whether it was present.
func (r *Request) Resource() (string, bool) {
	v, ok := r.Fields["Resource"]
	return v, ok
}

// Connection handles a single client connection on behalf of the server,
// parsing the request and writing an appropriate response.
type Connection struct {
	conn     net.Conn
	root     string
	redirect map[string]string
}

// NewConnection creates a Connection for the given client conn. The root
// argument is the directory from which static files are served; it replaces
// the original `new File("." + resourcePath)` construction and is used to
// contain path traversal.
func NewConnection(conn net.Conn, root string) (*Connection, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection: conn must not be nil")
	}
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("connection: resolving root %q: %w", root, err)
	}
	return &Connection{
		conn:     conn,
		root:     absRoot,
		redirect: defaultRedirects(),
	}, nil
}

// Handle parses the client request and writes the response, then closes the
// connection. It is the Go equivalent of the Java Runnable.run(); launch it
// with `go conn.Handle(ctx)`.
//
// Unlike the original which swallowed IOExceptions with printStackTrace, this
// returns the error so the caller can log it as it sees fit.
func (c *Connection) Handle(ctx context.Context) error {
	defer c.conn.Close()

	req, err := c.parseRequest(ctx)
	if err != nil {
		return fmt.Errorf("connection: parsing request: %w", err)
	}
	// If there was no request line at all, the original did nothing.
	if req == nil {
		return nil
	}

	if err := c.sendResponse(ctx, req); err != nil {
		return fmt.Errorf("connection: sending response: %w", err)
	}
	return nil
}

// parseRequest reads and parses the client request from the socket, returning
// the populated Request. It returns (nil, nil) when the client sent no request
// line at all, mirroring the original behaviour of doing nothing.
//
// MIGRATION_NOTE: The original Java loop `while (!headerLine.isEmpty())` would
// throw a NullPointerException if the client closed the stream before sending a
// blank terminator line. Here we treat io.EOF as a normal end-of-headers so no
// panic occurs.
func (c *Connection) parseRequest(ctx context.Context) (*Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(c.conn)

	requestLine, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading request line: %w", err)
	}
	requestLine = strings.TrimRight(requestLine, "\r\n")
	if requestLine == "" {
		// No proper request line: do nothing (matches Java null check).
		return nil, nil
	}

	// The request line is formatted as: METHOD RESOURCE PROTOCOL
	parts := strings.Split(requestLine, " ")
	if len(parts) < 3 {
		return nil, fmt.Errorf("malformed request line: %q", requestLine)
	}

	req := &Request{Fields: make(map[string]string)}
	req.Fields["Method"] = parts[0]
	req.Fields["Resource"] = parts[1]
	req.Fields["Protocol"] = parts[2]

	// Read the remaining header lines until a blank line or EOF.
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			// Blank terminator line reached (or empty at EOF): headers done.
			break
		}

		kv := strings.SplitN(trimmed, ":", 2)
		key := kv[0]
		value := ""
		if len(kv) == 2 {
			// Strip a single leading space, matching Java replaceFirst(" ", "").
			value = strings.TrimPrefix(kv[1], " ")
		}
		req.Fields[key] = value

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading header line: %w", err)
		}
	}

	return req, nil
}

// sendResponse writes the appropriate response for the parsed request.
//
// If the resource is in the redirect map, a 301 is sent. If the resource does
// not exist, a 404 is sent. Otherwise the file is served with a 200 OK.
func (c *Connection) sendResponse(ctx context.Context, req *Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	resourcePath, ok := req.Resource()
	if !ok {
		return fmt.Errorf("request has no Resource field")
	}

	// 301 redirect if the resource is registered for redirection.
	if dest, redirected := c.redirect[resourcePath]; redirected {
		// MIGRATION_NOTE: The original 301 response was malformed: it used \n
		// instead of \r\n and omitted the terminating blank line. This has been
		// fixed to be HTTP-spec compliant. To replicate the original bug
		// exactly, change this to: "HTTP/1.1 301 Moved Permanently\nLocation: " + dest
		resp := "HTTP/1.1 301 Moved Permanently\r\nLocation: " + dest + "\r\n\r\n"
		if _, err := c.conn.Write([]byte(resp)); err != nil {
			return fmt.Errorf("writing 301 response: %w", err)
		}
		return nil
	}

	// Resolve and sanitize the file path to prevent directory traversal.
	// MIGRATION_NOTE: The original `new File("." + resourcePath)` was a
	// directory-traversal vulnerability (e.g. /../etc/passwd). We clean the
	// path and verify it stays within the configured root.
	resolved, err := c.resolvePath(resourcePath)
	if err != nil {
		return c.writeNotFound()
	}

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return c.writeNotFound()
	}

	return c.writeFile(resolved)
}

// resolvePath cleans the requested resource path and joins it to the server
// root, returning an error if the result escapes the root directory.
func (c *Connection) resolvePath(resourcePath string) (string, error) {
	// Clean removes any . and .. segments; joining with root then re-cleaning
	// yields an absolute path we can bound-check.
	cleaned := filepath.Clean("/" + strings.TrimPrefix(resourcePath, "/"))
	full := filepath.Join(c.root, cleaned)

	// Ensure the resolved path is still within root.
	rel, err := filepath.Rel(c.root, full)
	if err != nil {
		return "", fmt.Errorf("resolving path %q: %w", resourcePath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes root", resourcePath)
	}
	return full, nil
}

// writeNotFound writes the static 404 response to the client.
func (c *Connection) writeNotFound() error {
	resp := "HTTP/1.1 404 Not Found\r\n\r\n" + http404Body
	if _, err := c.conn.Write([]byte(resp)); err != nil {
		return fmt.Errorf("writing 404 response: %w", err)
	}
	return nil
}

// writeFile serves the file at path with a 200 OK response.
//
// MIGRATION_NOTE: The original read the whole file into a byte array and used a
// single read() that does not guarantee a full read for large files. We use
// io.Copy which streams the file correctly regardless of size.
//
// MIGRATION_NOTE: The original used java.nio.Files.probeContentType, which
// consults the OS. Here we use mime.TypeByExtension and fall back to
// content sniffing. The exact MIME string may differ from the Java version and
// should be verified in tests.
func (c *Connection) writeFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file %q: %w", path, err)
	}
	defer file.Close()

	contentType := detectContentType(path, file)

	header := "HTTP/1.1 200 OK\r\nContent-Type: " + contentType + "\r\n\r\n"
	if _, err := c.conn.Write([]byte(header)); err != nil {
		return fmt.Errorf("writing 200 header: %w", err)
	}

	if _, err := io.Copy(c.conn, file); err != nil {
		return fmt.Errorf("streaming file %q: %w", path, err)
	}
	return nil
}

// detectContentType returns the MIME type for the file, using the extension
// first and falling back to sniffing the first bytes of content. It rewinds
// the file after sniffing so the caller can still read it from the start.
func detectContentType(path string, file *os.File) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}

	// Fall back to content sniffing (http.DetectContentType inlined via the
	// net/http package would add a dependency; use a small buffer read here).
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	// Rewind so the subsequent io.Copy serves the whole file.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "application/octet-stream"
	}
	return sniffContentType(buf[:n])
}

// sniffContentType returns a best-effort content type for the given prefix
// bytes. It defers to net/http's detection algorithm.
func sniffContentType(data []byte) string {
	// Import kept local to the function via package-level http import would be
	// cleaner; using the standard detection here.
	return detectViaHTTP(data)
}
