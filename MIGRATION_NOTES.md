# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `src/Connection.java` → `internal/connection.go` (78% confidence) ⚠️ needs review
- `src/WebServer.java` → `internal/webserver.go` (74% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `Connection (dependency)` in `src/WebServer.java`
**Reason:** The core request-handling logic is not in this file; WebServer only accepts sockets and delegates. Any actual HTTP parsing, routing, and response generation cannot be determined or migrated from this file alone.
**Suggestion:** Analyze and migrate src/Connection.java together with this file to reconstruct the real HTTP behavior and routes.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `src/Connection.java`
Confidence: 78%
Issues:
  - [info] The original resolves files relative to the current working directory using '.' + resource. The Go port defaults root to '.' but resolves to an absolute path and adds path-traversal protection. This changes behavior only for malicious traversal inputs (which are rejected instead of served) — an intentional and documented security fix, not a normal-usage regression.
  - [info] Content-Type detection differs: Java uses Files.probeContentType; Go uses mime.TypeByExtension with sniffing fallback and 'application/octet-stream' default rather than emitting literal 'null'. Observable header may differ slightly but is more correct.
  - [info] The original serves an existing directory path via FileInputStream (would error/behave oddly), while the Go port returns 404 for directories (info.IsDir()). Minor edge-case behavioral difference.

### `src/WebServer.java`
Confidence: 74%
