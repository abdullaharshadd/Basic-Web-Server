# Basic Web Server

A basic HTTP web server migrated from Java/Spring to Go using the standard library. Handles incoming TCP connections and serves HTTP responses without any third-party frameworks.

---

## Tech Stack

- **Language:** Go (standard library only)
- **HTTP/Networking:** `net/http`, `net` packages
- **No external dependencies**

---

## Prerequisites

- Go 1.21 or later
- npm (used during migration tooling only — see [Migration Notes](#migration-notes))
- Git

Verify your Go installation:

```bash
go version
```

---

## Getting Started

### 1. Clone the Repository

```bash
git clone https://github.com/abdullaharshadd/Basic-Web-Server.git
cd Basic-Web-Server
```

### 2. Install Dependencies

> **Note:** An `npm install` step was detected during migration scaffolding. This applies to migration tooling only and is not required to run the Go server. The Go project has no external package dependencies.

```bash
# No Go dependencies to install — standard library only
go mod tidy
```

If a `go.mod` file is not present, initialize the module:

```bash
go mod init github.com/abdullaharshadd/Basic-Web-Server
```

### 3. Environment Setup

No environment variables are required to run this project. See the [Environment Variables](#environment-variables) section for details.

### 4. Run the Server

```bash
go run .
```

Or build and run the binary:

```bash
go build -o basic-web-server .
./basic-web-server
```

The server will start and listen on the configured port (see [Known Limitations](#known-limitations) for port configuration caveats).

---

## Running Tests

```bash
go test ./...
```

To run tests with verbose output:

```bash
go test -v ./...
```

> **Warning:** Test coverage for migrated components is not guaranteed. The migration was completed at 0% confidence. Manually verify that tests exercise the actual server behavior before relying on them. See [Manual Review Required](#manual-review-required).

---

## Environment Variables

No environment variables are required by this project.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| —        | —        | —       | None defined |

If you add configuration (e.g., port, host), document variables here and load them using `os.Getenv`.

---

## Architecture Overview

The migrated Go project maps directly from the original two Java source files:

```
Basic-Web-Server/
├── main.go              # Entry point — replaces WebServer.java bootstrap logic
├── connection.go        # Connection handling logic — replaces Connection.java
├── go.mod               # Go module definition
└── README.md
```

### Component Mapping

| Original (Java/Spring)     | Migrated (Go)         | Responsibility                          |
|----------------------------|-----------------------|-----------------------------------------|
| `src/WebServer.java`       | `main.go`             | Server startup, port binding, listener loop |
| `src/Connection.java`      | `connection.go`       | Per-connection handling, HTTP parsing, response writing |

**Request flow:**

1. `main.go` opens a TCP listener on the configured port.
2. Each accepted connection is passed to the handler defined in `connection.go`.
3. The handler reads the incoming HTTP request, constructs a response, and writes it back to the client.

The Go implementation uses goroutines (`go handleConnection(conn)`) in place of Java threads or Spring's embedded servlet container.

---

## Migration Notes

### What Changed from the Original Java/Spring Codebase

| Area | Java/Spring | Go (Standard Library) |
|------|-------------|----------------------|
| Server framework | Spring Boot / embedded Tomcat | `net` or `net/http` standard library |
| Connection handling | Spring MVC `@Controller` / Servlet API | Manual TCP accept loop with goroutines |
| HTTP parsing | Handled by Spring/Servlet container | Manual parsing or `net/http` server handler |
| Thread model | Java threads managed by Tomcat thread pool | Go goroutines (one per connection) |
| Dependency injection | Spring IoC container (`@Autowired`, etc.) | Direct function calls and struct initialization |
| Build system | Maven or Gradle | Go toolchain (`go build`, `go mod`) |
| Configuration | `application.properties` / `@Value` | Hardcoded defaults or `os.Getenv` |
| Packaging | `.jar` (fat jar via Spring Boot plugin) | Single native binary via `go build` |

### npm Install Note

The migration process detected an `npm install` command. This originates from the migration scaffolding toolchain and has **no relevance** to building or running the Go server. Disregard it for day-to-day development.

---

## Known Limitations

### Overall Migration Confidence: 0%

This migration was flagged at **0% confidence**, meaning the automated translation of Java semantics to Go is unreliable. The output should be treated as a starting draft, not production-ready code.

Specific concerns:

- **HTTP protocol handling:** The original `Connection.java` may have relied on Servlet API abstractions that do not map cleanly to raw Go socket I/O. Request parsing logic must be validated manually.
- **Concurrency model:** Java thread-per-connection semantics have been replaced with goroutines, but any use of `synchronized` blocks, thread-local storage, or Java locks (`ReentrantLock`, etc.) will not have a correct Go equivalent without manual rewriting.
- **Error handling:** Java exception hierarchies (`IOException`, etc.) are replaced with Go's `error` return values. Any exception handling that was swallowed or broadly caught in the original code may now fail silently or panic.
- **Port configuration:** If the original server read its port from Spring configuration, that mechanism no longer exists. The port may be hardcoded and will need to be made configurable.
- **No tests migrated:** There is no evidence that unit or integration tests from the original project were successfully migrated.

---

## Manual Review Required

The following files require thorough manual review before this server is considered functional or safe to deploy:

### `src/Connection.java` → `connection.go`

- **Why:** Core connection-handling logic. Low migration confidence means HTTP request parsing, response construction, and connection lifecycle management may be incorrect.
- **Check:**
  - Raw socket reads are correctly buffered and delimited (HTTP uses `\r\n\r\n` to terminate headers).
  - Response status lines and headers are correctly formatted.
  - Connections are properly closed after the response is sent (check for goroutine leaks).
  - Any multi-request (keep-alive) handling from the original is either preserved or explicitly dropped.

### `src/WebServer.java` → `main.go`

- **Why:** Server startup and listener loop. Incorrect port binding or missing error handling will prevent the server from starting.
- **Check:**
  - The listener binds to the correct address and port.
  - The accept loop properly spawns goroutines and does not block or drop connections under load.
  - Fatal errors (e.g., address already in use) are surfaced clearly rather than silently ignored.
  - Graceful shutdown (signal handling via `os/signal`) is implemented if required.

### General

- Run the server against a real HTTP client (e.g., `curl`, browser) and verify responses before any further development.
- Add integration tests that send actual HTTP requests and assert on response codes and bodies.
- Review for any hardcoded values (ports, paths, response bodies) that should be configurable.

```bash
# Quick smoke test once the server is running
curl -v http://localhost:<PORT>/
```

---

## Contributing

This project requires significant manual verification before it is stable. If you are continuing development:

1. Resolve all items in [Manual Review Required](#manual-review-required) first.
2. Add tests before making further changes.
3. Document any environment variables or configuration you introduce in the table above.