# Basic-Web-Server

A basic HTTP web server migrated from Java/Spring to Go using the standard library. The server handles incoming TCP connections and serves HTTP responses without any external framework dependencies.

---

## Tech Stack

- **Language:** Go (standard library only)
- **HTTP layer:** `net/http` and `net` packages
- **No third-party dependencies**

---

## Prerequisites

- Go 1.21 or later — [https://go.dev/dl/](https://go.dev/dl/)
- Node.js / npm (only if using the detected `npm install` step for tooling — see [Migration Notes](#migration-notes))

Verify your Go installation:

```bash
go version
```

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

> **Note:** This command was detected in the setup plan. If it refers to a build tool or script runner rather than application dependencies, it may not be required to run the Go server itself. See [Migration Notes](#migration-notes).

### 3. Environment variables

No environment variables are required for this project. See the [Environment Variables](#environment-variables) section for details.

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

> **Warning:** The migration confidence is **0%**. Automated tests may not exist or may not reflect correct behavior. Manual verification is strongly recommended before relying on test results. See [Manual Review Required](#manual-review-required).

---

## Environment Variables

No environment variables were detected as required by this application.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| —        | —        | —       | None currently defined |

If you add configuration (e.g., port binding), document new variables here.

---

## Architecture Overview

The migrated project follows a flat Go package structure, replacing the original two Java source files with equivalent Go files:

```
Basic-Web-Server/
├── main.go           # Entry point — replaces src/WebServer.java
├── connection.go     # Connection handling logic — replaces src/Connection.java
└── go.mod            # Go module definition
```

### Component responsibilities

| Go File | Original Java File | Responsibility |
|---|---|---|
| `main.go` | `src/WebServer.java` | Initializes the server, binds to a port, accepts incoming connections |
| `connection.go` | `src/Connection.java` | Reads HTTP requests from a TCP connection and writes HTTP responses |

**Request lifecycle:**

1. `main.go` opens a TCP listener on the configured port
2. Each accepted connection is passed to the handler in `connection.go`
3. The handler parses the request and writes a plain HTTP response
4. The connection is closed after the response is sent

---

## Migration Notes

### What changed from the original Java/Spring codebase

| Area | Java/Spring | Go/Standard Library |
|---|---|---|
| HTTP server bootstrap | Spring Boot application context and `@SpringBootApplication` | `net.Listen` or `net/http.ListenAndServe` |
| Connection handling | Spring's servlet/filter chain or manual `ServerSocket` | Raw `net.Conn` handled per goroutine |
| Threading model | Thread-per-connection or Spring-managed thread pool | Goroutine-per-connection |
| Dependency injection | Spring IoC container / `@Autowired` | Direct function calls and struct composition |
| Build tool | Maven or Gradle | Go toolchain (`go build`, `go run`) |
| Package structure | `src/` Java package hierarchy | Flat Go package at module root |

### `npm install` command

An `npm install` command was detected in the setup plan. This is **unexpected** for a Go project. Likely explanations:

- It was carried over from an unrelated part of the project (e.g., a frontend or documentation toolchain)
- It refers to a script runner used during development

**Action required:** Confirm whether `npm install` is actually needed to run this server. If not, remove it from the setup documentation.

---

## Known Limitations

- **Migration confidence is 0%.** The automated migration produced output but could not verify correctness of the translated logic. The Go code may not behave identically to the original Java implementation.
- The original Spring project's HTTP handling abstractions may not map cleanly to raw Go `net` code. Edge cases around request parsing, keep-alive connections, and error handling should be assumed unverified.
- No test suite was migrated. Behavioral correctness has not been validated by automated tests.

---

## Manual Review Required

The following files were flagged as low-confidence and **must be manually reviewed** before this server is used in any environment:

### `connection.go` (migrated from `src/Connection.java`)

- Verify that HTTP request parsing correctly reads headers and body from the TCP stream
- Confirm that response formatting (status line, headers, body) matches valid HTTP/1.1 syntax
- Check that the connection is properly closed after each response and that resources are not leaked
- Review error handling — Java exceptions do not map directly to Go error returns

### `main.go` (migrated from `src/WebServer.java`)

- Confirm the server binds to the intended port and that the port is configurable
- Verify that each connection is dispatched to a goroutine (`go handleConnection(...)`) to prevent blocking
- Check that the server shuts down gracefully on interrupt signals (`os.Signal` / `context` handling)
- Ensure the entry point correctly initializes all components that the original `WebServer.java` set up

---

## Contributing

Because this migration is at 0% confidence, treat the codebase as **untested**. Before making changes:

1. Manually verify each flagged file above
2. Write integration tests that confirm expected HTTP responses
3. Compare behavior against the original Java implementation if available