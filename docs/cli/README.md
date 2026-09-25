# Open Agents CLI

The `open-agents` CLI is a thin Go/Cobra client for the local Open Agents daemon.
It resolves, opens, discovers, inspects, and stops the desktop-owned daemon
through the loopback HTTP surface and the `running.json` handshake. It must not
open SQLite directly or call runtime, workspace, tracker, or agent adapters
in-process.

When using the CLI directly from a shell, open the desktop app first (or run
`open-agents start` to resolve and launch it). Product commands such as
`open-agents agent ls` and `open-agents spawn` call the loopback daemon and will
fail with a "daemon is not running" error if no `running.json` points at a live
process. From a source checkout, build and run the local binary explicitly, for
example:

```bash
cd backend
go build -o ./bin/open-agents ./cmd/open-agents
./bin/open-agents agent ls
```

## Current commands

Every product command resolves to a daemon HTTP route. Run `open-agents <command>
--help` for the authoritative flag shape.

### Daemon control

| Command                       | Purpose                                                                                                                           |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `open-agents start`                    | Resolve, fetch when needed, and open the Open Agents desktop app. The desktop app owns the daemon.                                      |
| `open-agents stop`                     | Gracefully stop the daemon via loopback `POST /shutdown` after verifying daemon identity.                                         |
| `open-agents status` / `--json`        | Report daemon state from `running.json`, process liveness, `/healthz`, and `/readyz`.                                             |
| `open-agents doctor` / `--json`        | Check config, data directory, DB-file presence, daemon state, `git`, and (on Darwin/Linux) `tmux`; on Windows conpty is built in. |
| `open-agents completion <shell>`       | Generate completions for `bash`, `zsh`, `fish`, or `powershell`.                                                                  |
| `open-agents version` / `open-agents --version` | Print build metadata.                                                                                                             |
| `open-agents daemon`                   | Hidden internal daemon entrypoint used by the desktop supervisor.                                                                          |

### Product commands

| Command                             | Daemon route                                   |
| ----------------------------------- | ---------------------------------------------- |
| `open-agents project add`                    | `POST /api/v1/projects`                        |
| `open-agents project ls`                     | `GET /api/v1/projects`                         |
| `open-agents project get <id>`               | `GET /api/v1/projects/{id}`                    |
| `open-agents project set-config <id>`        | `PUT /api/v1/projects/{id}/config`             |
| `open-agents project rm <id>`                | `DELETE /api/v1/projects/{id}`                 |
| `open-agents agent ls`                       | `POST /api/v1/agents/readiness/ensure` (`display`) |
| `open-agents agent ls --refresh`             | `POST /api/v1/agents/refresh` (forced checks) |
| `open-agents spawn`                          | Targeted launch ensure, then `POST /api/v1/sessions` |
| `open-agents session ls`                     | `GET /api/v1/sessions` plus per-session PR summaries; shows branch, PR, CI, review, unresolved threads, activity, and age. |
| `open-agents session get <id>`               | `GET /api/v1/sessions/{id}`                    |
| `open-agents session kill <id>`              | `POST /api/v1/sessions/{id}/kill`              |
| `open-agents session restore <id>`           | `POST /api/v1/sessions/{id}/restore`           |
| `open-agents session exit-agent <id>`        | `POST /api/v1/sessions/{id}/exit-agent`        |
| `open-agents session resume-agent <id>`      | `POST /api/v1/sessions/{id}/resume-agent`      |
| `open-agents session rename <id> <name>`     | `PATCH /api/v1/sessions/{id}`                  |
| `open-agents session cleanup`                | `POST /api/v1/sessions/cleanup`                |
| `open-agents session claim-pr [<id>] <pr-ref>` | `POST /api/v1/sessions/{id}/pr/claim`        |
| `open-agents plan <id>`                        | `PATCH /api/v1/sessions/{id}/workflow-mode`  |
| `open-agents build <id>`                       | `PATCH /api/v1/sessions/{id}/workflow-mode`  |
| `open-agents orchestrator ls`                | `GET /api/v1/orchestrators`                    |
| `open-agents send`                           | `POST /api/v1/sessions/{id}/send`              |
| `open-agents preview [url]`                  | `POST /api/v1/sessions/{id}/preview`           |
| `open-agents preview start/status/stop`      | `POST/GET/DELETE /api/v1/sessions/{id}/preview/server` |
| `open-agents browser ...`                    | `GET /api/v1/browser/status`, `POST /api/v1/browser/commands` |
| `open-agents hooks <agent> <event>`          | `POST /api/v1/sessions/{id}/activity` (hidden) |

`open-agents agent ls` asks the daemon to ensure display readiness, then prints the
existing table or legacy JSON projection. The daemon alone decides whether a
native check is needed. `--refresh` forces fresh installation and authentication
checks before printing.

`open-agents spawn` resolves project context in this order: explicit `--project`,
`OPEN_AGENTS_PROJECT_ID`, `OPEN_AGENTS_SESSION_ID` (by fetching the current session from the
daemon), then the current working directory matched against registered project
paths. If `OPEN_AGENTS_SESSION_ID` is set but the session cannot be fetched, pass
`--project` explicitly. Use `open-agents spawn --standalone --agent <agent> --name
<name>` to launch a worker in an Open Agents-managed plain directory without resolving
or registering a project. Standalone sessions do not support orchestrator,
branch, issue, or PR-claim options.

`open-agents session claim-pr <pr-ref>` attaches a PR to the current worker by reading
`OPEN_AGENTS_SESSION_ID`. From an orchestrator or external shell, pass the target
explicitly with `open-agents session claim-pr <session-id> <pr-ref>`. The explicit
form supports cross-session coordination.

`open-agents plan <id>` and `open-agents build <id>` move a session between its delivery stages
(planning → building) by setting `workflow_mode` on the daemon. Setting either
stage is also one of the kanban review lock's release paths: a card frozen in
the review column is released so it can move with its PR facts again.

If `--agent` / `--harness` is omitted, `open-agents spawn` uses the resolved project's
`worker.agent` config. Before spawning, the CLI performs one targeted launch
ensure. It fails early for unsupported or definitely missing harnesses and
warns-but-continues for unauthorized or unknown observations; daemon session
creation repeats launch validation and native launch remains authoritative.
`--skip-agent-check` suppresses only the CLI warnings and early check, never the
daemon validation.

Standalone spawns require `--agent` because there is no project configuration
from which to resolve a default harness.

`open-agents preview` resolves its session from the `OPEN_AGENTS_SESSION_ID` environment variable
(it is meant to run inside a session), not a flag. With no argument it
autodetects an `index.html` in the session workspace. Relative file targets are
resolved from the session workspace root, regardless of the shell's current
directory, and served through Open Agents' confined loopback preview origin. Absolute
paths and `file://` URLs must resolve inside that workspace; explicit HTTP and
HTTPS targets remain regular browser URLs.

`open-agents preview start [configuration]` loads `.open-agents/launch.json` from the session
workspace, starts that exact command under a session-owned supervisor, selects
or records its loopback port, waits for readiness, and opens application
targets in the Browser panel. `status` reports bounded recent logs and `stop`
terminates the managed process tree. Multiple configurations must be selected
by name; Open Agents does not assign confidence scores to arbitrary localhost servers.
This is an optional, reusable project configuration, not a prerequisite for
preview. Agents must not create it automatically. Static HTML and Markdown use
the direct file preview and must not cause package-manager scaffolding,
dependency installation, or a development server to be introduced.

When a browser-displayable file is the requested artifact, agents should call
`open-agents preview <workspace-path>` immediately after creating or materially updating
the primary output. Markdown, HTML, PDF, SVG, and common images can be served
directly. Supporting assets must not replace an active application preview.

`open-agents browser` also resolves its target from `OPEN_AGENTS_SESSION_ID`, but controls the
session-owned live Electron browser rather than only setting its preview URL.
The target-isolated command set includes `status`, `open`, `snapshot`, `click`,
`dblclick`, `focus`, `fill`, `type`, `press`, `hover`, `scroll`,
`scrollintoview`, `drag`, `select`, `check`, `uncheck`, `get`, `highlight`,
`unhighlight`, `tabs`, `tab new`, `tab select`, `tab close`, `frame`, `dialog`,
`wait`, `screenshot`, `network start/status/list/stop/clear`, `console`, and
`errors`. The native engine is bound internally; there is no second command or
connection setup. Logical tab IDs remain stable for the session, and allowed popups
become Open Agents browser tabs rather than separate OS-browser windows. The Open Agents desktop
app must be open because Electron owns the `WebContentsView`.
References from a snapshot are invalidated after navigation or DOM replacement;
they are also invalidated when changing tabs. Take another snapshot when a
command reports `STALE_REFERENCE`.
Browser waits cover load completion, text or selector appearance and
disappearance, URL matching, fixed delays, and a configurable DOM-stability
window for HMR-driven verification.
Browser tabs in the same worker share a memory-only Electron profile. Different
workers receive distinct partitions, so cookies, authentication, local storage,
and session storage do not leak between their browser runtimes.
Network capture is disabled by default and must be started explicitly. It is
scoped to the active tab at start time, expires after 60 seconds by default
(maximum 300), retains at most 200 in-memory entries, and is cleared with the
tab/session. Captured data is metadata-only: request and response bodies are
never read, sensitive headers are omitted, and URL credentials, fragments, and
query values are redacted.

PR actions are available through `open-agents pr merge` and
`open-agents pr resolve-comments`. Review actions are available through `open-agents review ls`,
`open-agents review trigger` (also `execute` and `restart`), `open-agents review cancel` (also
`stop`), and `open-agents review submit`.

## Configuration

The CLI and daemon share the same environment-driven config:

| Var                   | Default              | Purpose                                                                                        |
| --------------------- | -------------------- | ---------------------------------------------------------------------------------------------- |
| `OPEN_AGENTS_PORT`             | `3001`               | Loopback daemon port.                                                                          |
| `OPEN_AGENTS_RUN_FILE`         | `~/.open-agents/running.json` | PID/port handshake.                                                                            |
| `OPEN_AGENTS_DATA_DIR`         | `~/.open-agents/data`         | SQLite data directory.                                                                         |
| `OPEN_AGENTS_REQUEST_TIMEOUT`  | `60s`                | REST request timeout.                                                                          |
| `OPEN_AGENTS_SHUTDOWN_TIMEOUT` | `10s`                | Graceful shutdown cap.                                                                         |
| `OPEN_AGENTS_KEEP_DAEMON`      | unset (off)          | Keep the desktop app's daemon running after the window closes; stop only via `open-agents stop`. (fork) |
| `OPEN_AGENTS_DISABLE_GPU`      | unset (off)          | Skip Chromium hardware acceleration; escape hatch for broken Linux GPU drivers.                |

The daemon always binds `127.0.0.1`.

## Manual smoke test

```bash
# Terminal 1: start the daemon directly from the backend wrapper.
tmp=/tmp/open-agents-cli-smoke
rm -rf "$tmp"
mkdir -p "$tmp"
cd backend
OPEN_AGENTS_RUN_FILE="$tmp/running.json" \
OPEN_AGENTS_DATA_DIR="$tmp/data" \
OPEN_AGENTS_PORT=3037 \
go run .
```

```bash
# Terminal 2: inspect and control the daemon through the CLI.
cd backend
go build -o /tmp/open-agents ./cmd/open-agents

tmp=/tmp/open-agents-cli-smoke
export OPEN_AGENTS_RUN_FILE="$tmp/running.json"
export OPEN_AGENTS_DATA_DIR="$tmp/data"
export OPEN_AGENTS_PORT=3037

/tmp/open-agents status --json
/tmp/open-agents doctor
/tmp/open-agents status --json
/tmp/open-agents stop
/tmp/open-agents status --json
rm -rf "$tmp"
```

## Adding new commands

Add a product command only when a daemon HTTP route owns the corresponding
mutation/read; the CLI must call that route rather than reimplementing daemon
behavior. Commands not yet exposed but with backend routes in place include
`open-agents events ...` (over the CDC/SSE endpoint) and CLI parity for PR/review
actions.

Do not port old in-process TypeScript CLI behavior that mixed command handling
with storage and runtime implementation details.

### Claiming workspace PRs

Workspace projects can claim a PR/MR on their root origin or any registered
child repository origin. Use the child's full PR/MR URL: numbers still resolve
against the root's canonical repository or origin, and a root without a remote
cannot resolve numbers. Check registered children with `open-agents project get <id> --json`.
Unregistered repositories are rejected even if a checkout has an additional Git
remote for them. `canonicalRepoURL` requires a valid root origin; it is not a
workspace child allowlist. Scratch projects cannot claim PRs.

For automatic attribution, workspace sessions recorded on a bare branch such as
`open-agents/ws-1` or `open-agents/ws-1-2` can use hyphen siblings (`open-agents/ws-1-fix` or
`open-agents/ws-1-2-fix`) in registered repositories. Keep the entire recorded branch,
including collision suffixes. Exact and stacked branches and `/root` slash
siblings remain supported. Matching prefers the most specific owner and leaves
ambiguous ownership for explicit claiming. Custom branches and single-repository
projects do not gain hyphen-sibling ownership.

### Claiming upstream PRs from a fork

The registered origin remains the checkout and push repository. An optional
`canonicalRepoURL` in project config explicitly authorizes one upstream repository
for PR claims. Git remotes, including a remote named `upstream`, never grant claim
permission automatically. Both identities must have the same provider and host;
claims match the entire namespace and repository, including GitLab subgroups.

For a project with no other config:

```bash
open-agents project set-config my-project \
  --canonical-repo-url https://github.com/my-org/my-repo
```

`set-config` replaces the whole config. For an existing configured project, read
`open-agents project get my-project --json`, preserve its `project.config` fields, add
`canonicalRepoURL`, and submit the complete object with `--config-json`. The same
object is accepted by `PUT /api/v1/projects/{id}/config` as `{"config": {...}}`.
Use an HTTPS repository URL, without a PR/MR suffix, credentials, query, or fragment.
Self-managed GitLab URLs and nested namespaces are supported. Explicit ports
are preserved and must match too; `gitlab.example.com:8443` is a different
authority from `gitlab.example.com`.

Both `open-agents session claim-pr 42` and `open-agents spawn --claim-pr 42` resolve numbers against
canonical when configured, otherwise origin. A full PR/MR URL may name either
identity. Unrelated repositories and different hosts/providers remain rejected.
Removing `canonicalRepoURL` restores origin-only claims. This does not unlink PRs
already claimed or move existing worktrees. Repository identity is read at claim
time, so existing sessions need no restart or duplicate project.

Migration 0126 adds an empty canonical identity to existing non-NULL config JSON
where absent, preserving all other settings and any explicit canonical value.
NULL configs retain their defaults. No Git discovery runs during migration, and
no earlier migration is modified. Downgrading preserves config data; older
versions do not support canonical claims and may drop this field when saving
project settings.
