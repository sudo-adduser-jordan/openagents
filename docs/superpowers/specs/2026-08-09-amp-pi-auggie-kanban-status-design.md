# Amp, Pi, and Auggie Kanban Status Design

## Goal

Give Amp, Pi, and Auggie TUI sessions honest live Kanban status by routing their native lifecycle events through Open Agents's existing `open-agents hooks <agent> <event>` pipeline.

## Scope

- Add activity hook installation and derivation for `amp`, `pi`, and `auggie`.
- Capture each agent's native thread, session, or conversation ID for restore.
- Register all three harnesses as signal-capable so prolonged callback silence becomes `no_signal` instead of a misleading `idle`.
- Preserve user-owned hook configuration and plugin files.
- Keep callback delivery best-effort: missing Open Agents binaries, stopped daemons, and malformed native payloads must not interrupt the agent.
- Do not change persisted status vocabulary, daemon endpoints, API DTOs, or frontend status mapping.

## Architecture

All three adapters emit their native lifecycle events to the hidden Open Agents hook command. The existing CLI extracts native session metadata, derives an `ActivityState`, and sends it to the daemon. `activitydispatch.Derivers` remains the single source of truth for both event routing and signal capability.

### Amp

Extend the existing managed `.amp/plugins/open-agents-system-prompt.ts` plugin. It will retain system-prompt injection, report `session.start`, `agent.start`, and `agent.end`, and subscribe once per Amp thread to `thread.state`. Amp's native `running`, `idle`, `awaiting-approval`, and `error` states map to Open Agents `active`, `idle`, `waiting_input`, and `waiting_input` respectively. Hook subprocess failures are logged through Amp's plugin logger and never thrown.

### Pi

Install one managed TypeScript extension at `.pi/extensions/open-agents-activity.ts` and pass it explicitly with `--extension` for launches and restores. Explicit loading avoids relying on Pi's project-local extension trust decision. The extension reports `session_start`, `before_agent_start`, `agent_end`, `agent_settled`, and `session_shutdown` with `ctx.sessionManager.getSessionId()` as `session_id`. `agent_end` keeps Pi 0.80.x compatible; newer releases also confirm the final idle transition with `agent_settled`.

### Auggie

Merge Open Agents hook groups into `.augment/settings.local.json` using the shared matcher-group JSON manager. Because Auggie requires hook commands to point to executable files with supported extensions, install one Open Agents-owned wrapper script per native event under `.augment/open-agents-hooks/`. Configure `SessionStart`, `PreToolUse`, `PostToolUse`, `Stop`, and `SessionEnd`. Preserve all unrelated settings and user hooks.

## Event Mapping

| Agent | Native event/state | Open Agents event | Open Agents activity |
|---|---|---|---|
| Amp | `session.start` | `session-start` | metadata only |
| Amp | thread `running` / `agent.start` | `thread-state` / `user-prompt-submit` | `active` |
| Amp | thread `idle` / `agent.end` | `thread-state` / `stop` | `idle` |
| Amp | thread `awaiting-approval` or `error` | `thread-state` | `waiting_input` |
| Pi | `session_start` | `session-start` | `idle` |
| Pi | `before_agent_start` | `user-prompt-submit` | `active` |
| Pi | `agent_end` / `agent_settled` | `stop` | `idle` |
| Pi | `session_shutdown` | `session-end` | `exited` |
| Auggie | `SessionStart`, `PreToolUse`, `PostToolUse` | `session-start`, `pre-tool-use`, `post-tool-use` | `active` |
| Auggie | `Stop` with `end_turn` or `interrupted` | `stop` | `idle` |
| Auggie | `Stop` with `error` or `max_iterations` | `stop` | `waiting_input` |
| Auggie | `SessionEnd` | `session-end` | `exited` |

## Failure Handling

- Generated plugin and extension callbacks have bounded timeouts and swallow failures after logging where the host exposes a logger.
- Auggie wrapper scripts execute Open Agents directly and inherit stdin so the original JSON payload is preserved.
- Managed files are written atomically, marked with Open Agents sentinels, and never overwrite foreign files.
- Generated workspace files are ignored locally so they do not dirty or block deletion of Open Agents worktrees.

## Verification

- Unit-test every native-to-Open Agents activity mapping, including unknown events and malformed payloads.
- Test managed file installation, idempotency, preservation of user files/settings, and foreign-file refusal.
- Test Pi launch and restore commands include the managed extension.
- Test `SupportsHarness` returns true for all three agents.
- Run focused adapter, activity-dispatch, session-status, CLI-hook, registry, and session-manager tests; then run backend `go test ./...` and frontend typecheck.
