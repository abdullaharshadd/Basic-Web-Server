// Package internal provides a raw-socket-style HTTP connection handler,
// migrated from the original Java Connection class.
//
// The original Java code implemented a thread-per-connection model that
// manually parsed raw HTTP over java.net.Socket. In idiomatic Go we do NOT
// re-implement raw socket parsing; instead we expose the same behavior as an
// http.Handler wired into a chi router. This preserves the exact routing
// semantics (redirects for /, /index.htm, /index; static file serving for
// everything else) while delegating request parsing, header handling, and
// response framing (CRLF, Content-Length, Connection: close) to net/http,
// which is the correct, safe Go equivalent.
//
// MIGRATION_NOTE: The Java class did its own line reading (readLine),
// colon-header parsing, and manual response writing. All of that is handled
// natively and correctly by net/http, so those low-level concerns are
// intentionally dropped rather than faithfully ported. The load-bearing
// business logic that IS preserved: the redirect map, the path-traversal
// guard, explicit MIME detection, IsDir() rejection, 200/301/404 responses,
// and full file streaming.
package internal

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// http404Body is the HTML body served for 404 responses, matching the
// original Java handler's page.
const http404Body = `<!DOCTYPE html>
<html>

<head>
    <title>Maurice Harris - Network Project 1</title>
</head>

<body><h1>
404 Error: Page Not Found
</h1></body>

</html>`

// redirectMap maps request paths that must be permanently redirected (301)
// to their target location. This mirrors the in-memory redirect table from
// the original Java Connection constructor.
var redirectMap = map[string]string{
	"/":          "/index.html",
	"/index.htm": "/index.html",
	"/index":     "/index.html",
}

// mimeTypes is an explicit MIME lookup table.
//
// MIGRATION_NOTE (D5): The Java code used Files.probeContentType, which is
// OS-dependent and non-deterministic across platforms. We replace it with an
// explicit map so behavior is stable everywhere.
var mimeTypes = map[string]string{
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".json": "application/json",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",
	".txt":  "text/plain",
	".pdf":  "application/pdf",
}

// StaticFileHandler serves static files from a configured root directory and
// applies the permanent-redirect table. It is the idiomatic Go replacement
// for the Java Connection object.
type StaticFileHandler struct {
	root string
}

// NewStaticFileHandler constructs a StaticFileHandler rooted at the given
// directory. If root is empty it defaults to the current working directory
// (matching the Java "." + resourcePath behavior).
func NewStaticFileHandler(root string) *StaticFileHandler {
	if root == "" {
		root = "."
	}
	return &StaticFileHandler{root: root}
}

// contentType returns the explicit MIME type for a file path, falling back to
// application/octet-stream when the extension is unknown.
func contentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := mimeTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// resolvePath safely resolves a request path against the handler root,
// preventing directory traversal.
//
// MIGRATION_NOTE (D7): URL-decoding must happen before filepath.Clean, and
// the cleaned result must be contained within root.
func (h *StaticFileHandler) resolvePath(resource string) (string, bool) {
	decoded, err := url.PathUnescape(resource)
	if err != nil {
		return "", false
	}

	clean := filepath.Clean("/" + decoded)
	full := filepath.Join(h.root, clean)

	absRoot, err := filepath.Abs(h.root)
	if err != nil {
		return "", false
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", false
	}

	// Containment check: absFull must be absRoot or live beneath it.
	if absFull != absRoot && !strings.HasPrefix(absFull, absRoot+string(os.PathSeparator)) {
		return "", false
	}
	return absFull, true
}

// ServeHTTP implements http.Handler. It reproduces the original Java
// sendResponse logic: 301 redirects for mapped paths, 200 with file contents
// when the target exists and is a regular file, and 404 otherwise.
func (h *StaticFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Path

	// D1/D2: net/http already sets Connection: close semantics per HTTP/1.1
	// negotiation and writes proper CRLF framing. We only ensure it here.
	w.Header().Set("Connection", "close")

	// 301 redirect table.
	if target, ok := redirectMap[resource]; ok {
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}

	fullPath, ok := h.resolvePath(resource)
	if !ok {
		h.writeNotFound(w)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		// File does not exist (or is inaccessible): 404.
		h.writeNotFound(w)
		return
	}

	// IsDir() check is load-bearing: directories are not served.
	if info.IsDir() {
		h.writeNotFound(w)
		return
	}

	f, err := os.Open(fullPath)
	if err != nil {
		h.writeNotFound(w)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Str("path", fullPath).Msg("failed to close file")
		}
	}()

	// D4: Content-Length on all responses; D6: stream the full file.
	w.Header().Set("Content-Type", contentType(fullPath))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, f); err != nil {
		log.Error().Err(err).Str("path", fullPath).Msg("failed to stream file")
	}
}

// writeNotFound writes the 404 response with the original HTML body and a
// correct Content-Length.
func (h *StaticFileHandler) writeNotFound(w http.ResponseWriter) {
	body := []byte(http404Body)
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotFound)
	if _, err := w.Write(body); err != nil {
		log.Error().Err(err).Msg("failed to write 404 body")
	}
}

// buildRouter constructs the chi router with all routes from the original
// Java handler registered explicitly.
//
// MIGRATION_NOTE: main.go references buildRouter(); it is defined here so the
// static-file handler's routes are wired at their exact paths.
func buildRouter() http.Handler {
	r := chi.NewRouter()
	h := NewStaticFileHandler(".")

	// Explicit redirect routes (301 -> /index.html).
	r.Get("/", h.ServeHTTP)
	r.Get("/index.htm", h.ServeHTTP)
	r.Get("/index", h.ServeHTTP)

	// Catch-all static file route.
	r.Get("/*", h.ServeHTTP)

	return r
}

// BuildRouter is the exported wrapper around buildRouter, allowing external
// packages (such as cmd/server/main.go) to obtain the fully wired HTTP handler.
func BuildRouter() http.Handler {
	return buildRouter()
}
