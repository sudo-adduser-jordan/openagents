# docs index

Open Agents is a long-running Go daemon (`backend/`) plus an Electron + TypeScript frontend (`frontend/`). The daemon supervises coding-agent sessions and exposes daemon control, project/session state, terminal streaming, and CDC/event infrastructure.

Start with [overview.md](overview.md) for the mental model, then the track you need.

## Tracks

| Doc | Covers |
|---|---|
| [overview.md](overview.md) | Mental model, durable-vs-derived contract, load-bearing rules (single canonical copy). |
| [backend/architecture.md](backend/architecture.md) | Backend system overview, principles, component layout, data flows. |
| [backend/packages.md](backend/packages.md) | Package ownership rules: domain, services, ports, adapters, storage, HTTP, CLI, daemon wiring. |
| [backend/storage-cdc.md](backend/storage-cdc.md) | SQLite schema, migrations, CDC pipeline. |
| [backend/lifecycle-status.md](backend/lifecycle-status.md) | Status derivation precedence, lifecycle reducer, termination guardrails. |
| [backend/scm-observer.md](backend/scm-observer.md) | SCM subsystem: polling pipeline, durable-state invariants, PR identity design. |
| [frontend/design-system.md](frontend/design-system.md) | Renderer design system (skill: `open-agents-design-system`). |
| [frontend/board/plan.md](frontend/board/plan.md), [build.md](frontend/board/build.md), [review.md](frontend/board/review.md), [ready.md](frontend/board/ready.md), [worker.md](frontend/board/worker.md), [manager.md](frontend/board/manager.md) | Kanban lanes and card contracts. |
| [interfaces/cli.md](interfaces/cli.md) | CLI commands and daemon control surface (thin client; daemon owns behavior). |
| [interfaces/http-terminal.md](interfaces/http-terminal.md) | Loopback HTTP, SSE, terminal mux, browser bridge. |
| [operations/stack.md](operations/stack.md) | Durable technology decisions. |
| [operations/development.md](operations/development.md) | Prerequisites, build, test, troubleshooting. |
| [operations/status.md](operations/status.md) | What ships on `main` today and what is in flight. |
| [archive/onboarding-contract-review.md](archive/onboarding-contract-review.md) | Point-in-time PR #5126 onboarding review (historical). |
| [adr/](adr/) | Architecture decision records (why a boundary exists). |

## Contract layer (wins over prose)

| Artifact | Source of truth for | Generated from | Drift gate |
|---|---|---|---|
| `backend/internal/httpd/apispec/openapi.yaml` | Daemon HTTP API | `controllers/dto.go` + `apispec/specgen/build.go` via `task api:spec` | `go test ./internal/httpd/...` |
| `frontend/src/api/schema.ts` | Typed frontend client | `openapi.yaml` via `task api:ts` | `api-drift` CI job |
| `backend/internal/storage/sqlite/gen/` | SQLite query/DTO code | `queries/*` + migrations via `task db:sqlc` | `sqlc-drift` CI job |
| `AGENTS.md` | Agent operating contract | Hand-written | Hard rules enforced by tests |
| `open-agents <command> --help` | Authoritative CLI flags | Cobra definitions in `backend/internal/cli/` | Table tests in `backend/internal/cli/*_test.go` |

If prose disagrees with the contract layer, fix the prose. The CLI's hand-mirrored DTOs are a deliberate manual boundary covered by tests, not a generator.

## Where to add new documentation

- User needs it: https://orchestrator.inc/docs (source under `frontend/src/landing/content/docs/`).
- Contributor needs it: `docs/`, and add a row here.
- Agent runtime needs it: `backend/internal/skillassets/using-open-agents/`.
- Decision with trade-offs: `docs/adr/`.
