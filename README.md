# Basic-Web-Server

A basic HTTP web server migrated from Java/Spring to Go using the standard library. The server accepts incoming TCP connections and handles HTTP requests, delegating connection handling to a dedicated connection processor.

---

## Tech Stack

- **Language:** Go (standard library)
- **HTTP handling:** `net/http` or `net` (standard library only — no external frameworks)
- **Build tooling:** Go modules

---

## Prerequisites

- [Go](https://go.dev/dl/) 1.21 or later
- [Node.js / npm](https://nodejs.org/) (required for dependency setup — see note below)
- Git

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/abdullaharshadd/Basic-Web-Server.git
cd Basic-Web-Server
```

### 2. Install dependencies

```bash
npm install
```

> ⚠️ **Note:** An `npm install` step was detected during migration analysis. This is unusual for a Go project. Verify whether a frontend asset pipeline, code generation tool, or build script relies on Node.js. If this step is not needed for the Go server itself, it may be a leftover artifact from the migration and can be skipped.

### 3. Initialize Go modules (if not already present)

```bash
go mod tidy
```

### 4. Run the server

```bash
go run .
```

Or build and run the binary:

```bash
go build -o basic-web-server .
./basic-web-server
```

---

## Running Tests

```bash
go test ./...
```

> ⚠️ No test commands were detected in the original project's setup plan. The Go test suite may be incomplete or absent. See [Manual Review Required](#manual-review-required) for details.

---

## Environment Variables

No environment variables were detected as required by this project.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| —        | —        | —       | No environment variables currently configured |

> If the server port or host becomes configurable, document those variables here (e.g., `PORT`, `HOST`).

---

## Architecture Overview

The migrated Go project mirrors the two-module structure of the original Java source:

```
Basic-Web-Server/
├── main.go            # Entry point; replaces src/WebServer.java
│                      # Listens on a TCP port and accepts incoming connections
├── connection.go      # Replaces src/Connection.java
│                      # Handles per-connection HTTP request parsing and response generation
├── go.mod             # Go module definition
└── go.sum             # Dependency lock file (if external packages are added)
```

**Request flow:**

```
Client → TCP Listener (main.go) → Connection Handler (connection.go) → HTTP Response
```

The server opens a TCP socket, accepts connections in a loop, and spawns a goroutine per connection to process the HTTP request and write a response — functionally equivalent to the original Java `ServerSocket` + `Connection` thread model.

---

## Migration Notes

### What changed from the original Java/Spring codebase

| Concern | Original (Java/Spring) | Migrated (Go/standard library) |
|---|---|---|
| Language | Java | Go |
| Framework | Spring (Boot/MVC) | None — Go standard library only |
| Entry point | `WebServer.java` with `main()` | `main.go` |
| Connection handling | `Connection.java` (likely a `Runnable`/`Thread`) | `connection.go` (goroutine) |
| HTTP parsing | Spring DispatcherServlet / embedded Tomcat | Manual parsing via `net` or `net/http` |
| Dependency injection | Spring IoC container | None — direct function calls |
| Build tool | Maven or Gradle | Go modules (`go mod`) |
| Concurrency model | Java threads | Go goroutines |

### Key behavioral differences to be aware of

- **No Spring auto-configuration.** Any behavior that Spring provided implicitly (error pages, content negotiation, request mapping annotations) must now be implemented explicitly in Go.
- **No servlet container.** The original Java code likely relied on Tomcat for HTTP protocol compliance. The Go replacement must handle HTTP framing directly or use `net/http`.
- **Thread-per-connection → goroutine-per-connection.** The concurrency model is lighter in Go but the logic must be verified to be goroutine-safe.

---

## Known Limitations

### Components that could not be fully migrated

#### `src/WebServer.java` — Connection dependency

- **Reason:** `WebServer.java` only accepts sockets and delegates all processing to `Connection`. Because the actual HTTP parsing, routing, and response generation live entirely in `Connection.java`, the behavior of `WebServer` could not be independently determined or migrated from its source alone.
- **Impact:** The migrated `main.go` may be structurally correct (accept loop, goroutine dispatch) but the end-to-end HTTP behavior depends entirely on whether `connection.go` was migrated accurately.
- **Suggestion:** `WebServer.java` and `Connection.java` must be analyzed and validated together. Do not treat either file's migration as complete in isolation.

### Overall migration confidence

> **0% confidence** was reported for this migration. The migrated code should be treated as a structural scaffold only. All logic must be manually verified before the server is considered functional or production-ready.

---

## Manual Review Required

The following files and components **must be manually inspected and verified** by a developer before this project is used:

| File | Component | Issue | Action Required |
|------|-----------|-------|-----------------|
| `src/Connection.java` → `connection.go` | HTTP request handler | Low confidence migration; core logic for parsing requests, routing, and building responses | Read the original Java line-by-line and verify the Go equivalent produces identical behavior |
| `src/WebServer.java` → `main.go` | TCP accept loop + Connection dispatcher | WebServer delegates entirely to Connection; cannot be validated without Connection | Validate together with `connection.go`; confirm port binding, error handling, and graceful shutdown |
| `connection.go` | HTTP parsing | If raw TCP sockets are used, HTTP spec compliance (headers, status codes, keep-alive) must be verified manually | Consider switching to `net/http` handlers if full HTTP compliance is required |
| Build setup | `npm install` step | Origin of this step is unclear in a Go project | Determine whether Node.js tooling is genuinely required and remove or document accordingly |

---

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b fix/connection-handler`
3. Commit your changes: `git commit -m "fix: correct HTTP response header formatting"`
4. Push and open a pull request

---

## License

Refer to the original repository at [abdullaharshadd/Basic-Web-Server](https://github.com/abdullaharshadd/Basic-Web-Server) for license information.