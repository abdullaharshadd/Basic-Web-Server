# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `src/Connection.java` → `internal/connection.go` (68% confidence) ⚠️ needs review
- `src/WebServer.java` → `internal/webserver.go` (70% confidence) ⚠️ needs review
## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `src/Connection.java`
Confidence: 68%

### `src/WebServer.java`
Confidence: 70%
Issues:
  - [info] The migrated file still lacks any per-connection log line equivalent to the Java accept-loop println. The Target Expert concedes the loss and describes a correct fix (http.Server.ConnState firing on http.StateNew), but the fix is proposed, not shown as applied to the actual file.
