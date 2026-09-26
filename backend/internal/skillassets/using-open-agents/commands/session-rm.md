# open-agents session rm

Permanently remove a terminated session and its record. This is the counterpart to `open-agents session kill`: **kill** ends a running session, **rm** removes the record of one that already finished.

## Syntax

```
open-agents session rm <id> [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--project string` | Project id used to scope the session lookup | - |
| `--json` | Output as JSON | - |

## What it deletes

The session row, its change log, and everything that cascades from it: PR facts, checks, reviews, and conversation turns. The session's number is recorded as retired so it is never handed to a new session.

## What it does not do

- It does **not** terminate a running session. `rm` refuses with a conflict error, because `kill` is the operation that ends a process and runs its teardown.
- It does **not** remove the worktree directory. `kill` leaves it alone whenever it cannot prove it clean, and `rm` deliberately does not re-implement that judgement.

So the order is always: `kill` first, then `rm`.

## Examples

```bash
# Remove a finished session from the board for good
open-agents session kill demo-2
open-agents session rm demo-2
```

```bash
# Scope the lookup to one project
open-agents session rm demo-2 --project demo
```
