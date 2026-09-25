# open-agents build

Move a session into the building stage. A worker starts in planning mode and only begins implementing once a manager has reviewed its plan and advanced it with this command.

## Syntax

```
open-agents build <id> [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--project string` | Project id used to scope the session lookup | - |
| `--json` | Output as JSON | - |

## Examples

```bash
# Approve a worker's plan and let it start implementing
open-agents build demo-2
```

```bash
# Scope the lookup to one project
open-agents build demo-2 --project demo
```

## Notes

Only a worker accepts `building`. A manager is rejected, because coordinating is its building stage; use `open-agents manage` for a manager.

Setting the delivery stage also releases the kanban review lock, so a card frozen in the review column may move with its PR facts again.
