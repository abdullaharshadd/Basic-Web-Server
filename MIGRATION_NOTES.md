# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `src/Connection.java` → `internal/connection.go` (78% confidence) ⚠️ needs review
- `src/WebServer.java` → `internal/webserver.go` (78% confidence) ⚠️ needs review
## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `src/Connection.java`
Confidence: 78%

### `src/WebServer.java`
Confidence: 78%
Issues:
  - [info] The per-connection log line was genuinely dropped in the migration. The Target Expert concedes this and proposes a ConnState/StateNew callback, but the fix is described as a proposal and has not yet been confirmed as applied to the actual code.
