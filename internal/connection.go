package internal

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// redirects maps requested paths to their permanent (HTTP 301) targets.
//
// This mirrors the in-memory redirect HashMap from the original Java
// Connection class: '/', '/index.htm', and '/index' all redirect to
// '/index.html'.
var redirects = map[string]string{
	"/":          "/index.html",
	"/index.htm": "/index.html",
	"/index":     "/index.html",
}

// http404Body is the HTML page served with a 404 Not Found response,
// preserved verbatim from the original Java implementation.
const http404Body = `<!DOCTYPE html>
<html>

<head>
    <title>Maurice Harris - Network Project 1</title>
</head>

<body><h1>
404 Error: Page Not Found
</h1></body>

</html>`

// RegisterRoutes registers the static file server handler.
//
// It wires the catch-all GET route so that every requested resource is
// served relative to the working directory, matching the original
// thread-per-connection Connection handler's behavior:
//   - '/', '/index.htm', '/index' redirect to '/index.html' with HTTP 301
//   - missing files return HTTP 404 with an HTML body
//   - existing files return HTTP 200 with the file bytes and a probed MIME type
//
// MIGRATION_NOTE: The original code used a raw socket, thread-per-connection
// model with manual HTTP protocol parsing. In idiomatic Go we delegate socket
// handling, request parsing, and concurrency to net/http, which already runs
// each request in its own goroutine. The manual parseRequest logic is
// therefore replaced entirely by the standard library.
func RegisterRoutes(r chi.Router) {
	r.Get("/*", SendResponse)
}

// SendResponse serves the appropriate response for the requested resource.
//
// The decision chain mirrors the Java sendResponse if/else-if/else structure
// exactly: redirect first, then not-found, then serve the file.
//
//	redirect present -> 301 Moved Permanently (Location = redirect target)
//	file missing      -> 404 Not Found (HTML body)
//	otherwise         -> 200 OK (file bytes + probed content type)
func SendResponse(w http.ResponseWriter, req *http.Request) {
	resourcePath := req.URL.Path

	// If the requested path is in the redirect map, send a 301 to the
	// mapped target (not the request path).
	if target, ok := redirects[resourcePath]; ok {
		write301(w, target)
		return
	}

	// Resolve the file relative to the working directory ("." + path in Java).
	filePath := filepath.Join(".", filepath.Clean(resourcePath))

	// Stat before writing any headers so that directories or missing files
	// are treated as "not found" and never produce a malformed 200 response.
	//
	// MIGRATION_NOTE: Java's File.exists() returns true for directories, which
	// would have caused a malformed 200. Here we deliberately treat a directory
	// (or any stat error) as a 404, a documented deviation that fixes that bug.
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		write404(w)
		return
	}

	serveFile(w, req, filePath)
}

// write301 sends an HTTP 301 Moved Permanently response redirecting the
// client to target.
func write301(w http.ResponseWriter, target string) {
	w.Header().Set("Location", target)
	w.WriteHeader(http.StatusMovedPermanently)
}

// write404 sends an HTTP 404 Not Found response with the static HTML body.
func write404(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if _, err := w.Write([]byte(http404Body)); err != nil {
		log.Error().Err(err).Msg("failed to write 404 response body")
	}
}

// serveFile writes a 200 OK response containing the file at filePath.
//
// http.ServeContent probes the content type (falling back on extension and
// content sniffing), sets Content-Length, and writes the 200 status line and
// body. This replaces the manual byte-buffer copy in the Java original.
//
// MIGRATION_NOTE: The Java code wrote the "200 OK" status line before reading
// the file body. Here the status line is emitted by ServeContent only after
// the file is confirmed openable, which is a safer ordering. If the file was
// stat-able but cannot be opened, we return a 404 rather than emitting a
// truncated 200.
func serveFile(w http.ResponseWriter, req *http.Request, filePath string) {
	f, err := os.Open(filePath)
	if err != nil {
		write404(w)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		write404(w)
		return
	}

	http.ServeContent(w, req, filepath.Base(filePath), info.ModTime(), f)
}