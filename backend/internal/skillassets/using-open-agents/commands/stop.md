# open-agents stop

Stop the Open Agents daemon.

## Syntax

```
open-agents stop [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output stop result as JSON | - |
| `--timeout duration` | How long to wait for daemon shutdown | `10s` |

## Examples

```bash
# Stop the daemon
open-agents stop
```

```bash
# Stop with a longer timeout
open-agents stop --timeout 30s
```
