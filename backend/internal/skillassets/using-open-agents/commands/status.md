# open-agents status

Show Open Agents daemon status. Use this to verify the daemon is up and check which port it is bound to.

## Syntax

```
open-agents status [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output status as JSON | - |

## Examples

```bash
# Check daemon status
open-agents status
```

```bash
# Get status as JSON (e.g. to check port programmatically)
open-agents status --json
```
