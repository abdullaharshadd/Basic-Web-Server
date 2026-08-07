```markdown
# Basic Web Server

A simple HTTP web server migrated from Java/Spring to Go using the standard library. This application accepts incoming TCP connections and serves basic HTTP responses without any external framework dependencies.

---

## Tech Stack

- **Language:** Go
- **HTTP / Networking:** Go standard library (`net/http`, `net`)
- **No external frameworks or dependencies**

---

## Prerequisites

- [Go](https://golang.org/dl/) 1.21 or later
- [Node.js / npm](https://nodejs.org/) — required only if the project includes a frontend build step (see setup below)
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

> **Note:** This command was detected in the setup plan. If the project has no frontend assets, this step may not apply. Verify whether a `package.json` exists at the project root before running.

### 3. Environment setup

No environment variables are required for this project (see [Environment Variables](#environment-variables) below).

### 4. Run the server

No automated run command was detected during migration. Use one of the following standard Go commands:

```bash
# Run directly
go run .

# Or build and execute
go build -o basic-web-server .
./basic-web-server
```

---

## Running Tests

No test command was detected during migration. To run any Go tests that exist in the project:

```bash
go test ./...
```

If no test files are present, this command will report `no test files`. Adding tests is recommended — see [Manual Review Required](#manual-review-required).

---

## Environment Variables

No environment variables were identified as required by this project.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| —        | —        | —       | None detected |

If the migrated server uses a configurable port or host, consider externalizing those values. Example:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT`   | No       | `8080`  | TCP port the server listens on |
| `HOST`   | No       | `0.0.0.0` | Interface to bind the server to |

---

## Architecture Overview

The migrated project uses Go's standard library with no external frameworks. The structure follows a flat layout matching the original two-class Java design:

```
Basic-Web-Server/
├── main.go            # Entry point; starts the HTTP listener (migrated from WebServer.java)
├── connection.go      # Handles individual client connections (migrated from Connection.java)
├── go.mod             # Go module definition
└── README.md
```

### Component responsibilities

| Go File | Original Java File | Responsibility |
|---|---|---|
| `main.go` | `src/WebServer.java` | Initializes the server, binds to a port, accepts connections in a loop |
| `connection.go` | `src/Connection.java` | Reads the HTTP request from a TCP connection and writes an HTTP response |

In the original Java/Spring version, Spring Boot managed the HTTP lifecycle. In the Go version, connection handling and HTTP parsing are done explicitly using `net` or `net/http` primitives.

---

## Migration Notes

### What changed from the original Java/Spring codebase

| Area | Java / Spring | Go / Standard Library |
|---|---|---|
| **Runtime** | JVM + Spring Boot auto-configuration | Go binary, no runtime dependencies |
| **HTTP handling** | Spring `@RestController`, `DispatcherServlet` | `net/http` handlers or raw `net.Listener` |
| **Connection model** | Spring manages thread-per-request via embedded Tomcat | Goroutines spawned per accepted connection |
| **Dependency injection** | Spring IoC container | Not applicable — direct instantiation |
| **Build** | Maven/Gradle | `go build` |
| **Configuration** | `application.properties` / `@Value` | Hardcoded or environment variables |
| **Entry point** | `SpringApplication.run()` | `main()` in `main.go` |

---

## Known Limitations

The overall migration confidence score is **0%**. The automated migration was unable to validate correctness for either source file. The following limitations apply:

- **No functional verification was performed.** The migrated Go code has not been confirmed to behave identically to the original Java server.
- **Spring features not replicated:** If the original `WebServer.java` used any Spring-specific features beyond basic request routing (e.g., filters, middleware, Spring Security), those will not be present in the Go version.
- **HTTP parsing fidelity:** If `Connection.java` implemented custom HTTP parsing, the Go equivalent may differ in edge-case handling (e.g., malformed requests, keep-alive behavior).
- **Error handling:** Spring provides default error pages and exception mapping; the Go version has no equivalent unless explicitly implemented.
- **Concurrency model:** The goroutine-per-connection model is a reasonable equivalent to Tomcat threads, but has not been load-tested.

---

## Manual Review Required

The following files have **low migration confidence** and must be manually reviewed and verified before the server is used in any environment:

### `connection.go` (migrated from `src/Connection.java`)

- Verify that request parsing correctly reads the HTTP method, path, headers, and body.
- Verify that the response written back to the client is valid HTTP/1.1.
- Check that the connection is properly closed after the response is sent (defer `conn.Close()`).
- Confirm that any original logic beyond simple echo/response (e.g., routing, file serving) has been preserved.

### `main.go` (migrated from `src/WebServer.java`)

- Verify the server binds to the correct port and interface.
- Confirm that the accept loop correctly spawns goroutines and does not block.
- Check that fatal errors (e.g., port already in use) are handled and logged clearly.
- If the original used Spring's lifecycle hooks (`@PostConstruct`, `ApplicationRunner`), confirm equivalent startup logic is present.

### General checklist

- [ ] Run `go vet ./...` and resolve all warnings
- [ ] Run `go build ./...` without errors
- [ ] Manually send an HTTP request (`curl http://localhost:8080/`) and confirm a valid response
- [ ] Add unit or integration tests covering at least the connection handler
- [ ] Remove or justify the `npm install` step — confirm whether it is actually needed for this Go project
```