# open-agents manage

Move a manager session into manager mode. In this mode the manager may delegate work by running `open-agents spawn`; delegated workers always start in planning mode.

## Syntax

```
open-agents manage <id> [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--project string` | Project id used to scope the session lookup | - |
| `--json` | Output as JSON | - |

## Examples

```bash
# Return a planning manager to manager mode
open-agents manage demo-1
```

```bash
# Scope the lookup to one project
open-agents manage demo-1 --project demo
```
