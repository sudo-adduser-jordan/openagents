# open-agents session

Manage agent sessions: list, inspect, rename, kill, restore, clean up, and claim PRs.

## Syntax

```
open-agents session <subcommand> [args] [flags]
```

## Subcommands

---

### open-agents session ls

List sessions.

**Syntax:**
```
open-agents session ls [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-a, --all` | Include manager sessions | - |
| `--include-terminated` | Include terminated sessions | - |
| `--json` | Output as JSON | - |
| `-p, --project string` | Filter by project ID | - |

**Examples:**

```bash
# List all active worker sessions
open-agents session ls
```

```bash
# List all sessions including terminated, scoped to one project
open-agents session ls --include-terminated -p open-agents
```

---

### open-agents session get

Fetch one session.

**Syntax:**
```
open-agents session get <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output as JSON | - |
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Get details for session mer-3
open-agents session get mer-3
```

```bash
# Get session details as JSON
open-agents session get mer-3 --json
```

---

### open-agents session kill

Terminate a session.

**Syntax:**
```
open-agents session kill <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Kill session mer-3
open-agents session kill mer-3
```

---

### open-agents session rm

Permanently remove a terminated session and its record. The counterpart to `kill`: **kill** ends a running session, **rm** removes the record of one that already finished.

Deleting is refused while the session is still running — `kill` first. The session row, its change log, and the PR facts and conversation turns that cascade from it are removed, and the session's number is retired so it is never reused. The worktree directory is left alone.

**Syntax:**
```
open-agents session rm <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--project string` | Project id to scope the session lookup | - |
| `--json` | Output as JSON | - |

**Examples:**
```bash
# Kill, then remove for good
open-agents session kill mer-3
open-agents session rm mer-3
```

---

### open-agents session rename

Rename a session.

**Syntax:**
```
open-agents session rename <id> <name> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Rename session mer-3 to a new display name
open-agents session rename mer-3 "fix-auth-bug"
```

---

### open-agents session restore

Relaunch a terminated session.

**Syntax:**
```
open-agents session restore <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Restore a terminated session
open-agents session restore mer-3
```

---

### open-agents session cleanup

Clean up terminated sessions by reclaiming eligible workspaces. Dirty worktrees are skipped by the daemon.

**Syntax:**
```
open-agents session cleanup [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Filter by project ID | - |
| `-y, --yes` | Skip confirmation prompt | - |

**Examples:**

```bash
# Clean up all terminated sessions (skip prompt)
open-agents session cleanup -y
```

```bash
# Clean up terminated sessions for one project
open-agents session cleanup -p open-agents
```

---

### open-agents session claim-pr

Attach an existing PR to the current Open Agents session, or target another session explicitly.

**Syntax:**
```
open-agents session claim-pr <pr-ref> [flags]
open-agents session claim-pr <session-id> <pr-ref> [flags]
```

With one positional argument, `OPEN_AGENTS_SESSION_ID` supplies the session. This is the preferred form inside a worker. Pass both arguments from a manager or external shell when targeting another session.

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output as JSON | - |
| `--no-takeover` | Refuse if another active session owns the PR | - |
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Attach PR 88 to the current worker session
open-agents session claim-pr 88
```

```bash
# Attach PR 88 to session mer-3 explicitly
open-agents session claim-pr mer-3 88
```

```bash
# Claim PR 88 for the current worker but refuse if another session owns it
open-agents session claim-pr 88 --no-takeover
```
