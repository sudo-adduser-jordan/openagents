# HTTP, terminal, and browser bridge

Loopback-only daemon surface, terminal multiplexing, and the Electron browser bridge.

## HTTP Layer

### API Structure

```mermaid
flowchart TD
    subgraph HTTPD["HTTP Daemon"]
        Router[Router + Middleware]

        Router --> API[REST API]
        Router --> Events[SSE Events]
        Router --> Terminal[Terminal WebSocket]
    end

    subgraph Controllers["Controllers"]
        Sessions[Sessions Controller]
        Projects[Projects Controller]
        PRs[PRs Controller]
        Reviews[Reviews Controller]
    end

    subgraph Services["Services"]
        SessionSvc[Session Service]
        ProjectSvc[Project Service]
        PRSvc[PR Service]
        ReviewSvc[Review Service]
    end

    API --> Sessions
    API --> Projects
    API --> PRs
    API --> Reviews

    Sessions --> SessionSvc
    Projects --> ProjectSvc
    PRs --> PRSvc
    Reviews --> ReviewSvc

    Events -->|subscribe| CDC[CDC Broadcaster]
    Terminal --> TerminalMux[Terminal Manager]

```

### HTTP Listener Architecture

The daemon runs one HTTP listener: the primary loopback listener binds `127.0.0.1:3001` with no authentication and serves the desktop app and CLI. No additional network-facing listener is supported.

### Request Flow

```mermaid
sequenceDiagram
    participant Client
    participant Router
    participant Controller
    participant Service
    participant Manager
    participant Store
    participant DB

    Client->>Router: POST /api/v1/sessions
    Router->>Router: Middleware (auth, logging)
    Router->>Controller: handler(w, r)
    Controller->>Controller: decode JSON
    Controller->>Service: Spawn(config)
    Service->>Manager: Spawn(config)
    Manager->>Manager: Resolve mode and preflight its controller
    Manager->>Store: Create session
    Store->>DB: INSERT INTO sessions
    DB->>Store: session record
    Store->>Manager: session record
    Manager->>Manager: Create and provision workspace
    alt mode = tui
        Manager->>Manager: Launch terminal runtime/controller
    else mode = chat
        Manager->>Manager: Launch runtime-less Chat controller
    end
    Manager->>Service: Session response
    Service->>Controller: enriched session
    Controller->>Controller: encode JSON
    Controller->>Client: 201 Created + Session
```

---

## Terminal Multiplexing

The mux is the primary agent controller only for TUI-mode sessions. Chat-mode
sessions have no agent runtime handle and never attach their provider through
tmux. They may still open session-scoped shell terminals as a worktree escape
hatch; those shells are separate resources and do not become the agent
controller.

### Terminal Architecture

```mermaid
flowchart TD
    subgraph Frontend
        Browser[Browser Terminal]
    end

    subgraph HTTPD
        WS[WebSocket Handler]
    end

    subgraph Terminal
        Mux[Terminal Mux]
        Sessions[Session States]
    end

    subgraph Runtime
        TMux[tmux Runtime]
        MacPTY[macOS native PTY Host]
        ConPTY[conpty Runtime]
    end

    Browser -->|WebSocket| WS
    WS -->|attach| Mux
    Mux --> Sessions
    Sessions -->|create| TMux
    Sessions -->|create new macOS| MacPTY
    Sessions -->|create| ConPTY

    TMux -->|PTY attach| Mux
    MacPTY -->|loopback dial| Mux
    ConPTY -->|loopback dial| Mux

    Mux -->|frame| WS
    WS -->|binary| Browser

```

### Attach Flow

```mermaid
sequenceDiagram
    participant Client as Browser
    participant WS as WebSocket Handler
    participant Mux as Terminal Mux
    participant Runtime as tmux/conpty

    Client->>WS: WebSocket upgrade
    WS->>Mux: Attach(session, rows, cols)
    Mux->>Runtime: Attach(handle, rows, cols)

    Runtime->>Runtime: Create PTY
    Runtime->>Runtime: Spawn tmux attach

    loop Data Loop
        Runtime->>Mux: PTY output
        Mux->>WS: Binary frame
        WS->>Client: WebSocket message

        Client->>WS: User input
        WS->>Mux: Input frame
        Mux->>Runtime: Write to PTY
    end

    Client->>WS: Close
    WS->>Mux: Detach
    Mux->>Runtime: Close PTY
```


## Browser Runtime Bridge

Browser automation uses a dedicated local socket (`browser.sock` on Unix,
`open-agents-browser[-dev]` named pipe on Windows) between the daemon and Electron. The
daemon owns command authorization/correlation; Electron owns the actual browser
targets. Commands never use the supervisor liveness socket and never enable an
unauthenticated remote-debugging port.

Electron attaches its debugger directly to the selected session's
`WebContentsView`, so the protocol transport cannot enumerate or attach to the
Open Agents renderer or a different session. Browser control remains on the
loopback daemon surface.

Request observation is an explicit, temporary browser command rather than a
standing debugger feature. Capture is off by default, bound to the active tab
that starts it, limited to 200 in-memory metadata entries, and automatically
expires within at most five minutes. Open Agents never requests or stores request or
response bodies; it allowlists safe headers and redacts URL credentials,
fragments, and query values. Closing the tab, ending the session, or shutting
down Electron disables and discards the capture.

---

## Sources of truth

- Routes and DTOs: `backend/internal/httpd/controllers/dto.go` + `backend/internal/httpd/apispec/specgen/build.go` (via `task api`)
- Spec: `backend/internal/httpd/apispec/openapi.yaml`; typed client: `frontend/src/api/schema.ts`
- CLI mirror: [interfaces/cli.md](cli.md) (hand-mirrored DTOs are a deliberate manual boundary)
