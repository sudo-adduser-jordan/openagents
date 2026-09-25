# open-agents start

Fetch (if needed) and open the Open Agents desktop app. The desktop app owns the daemon, state, and updates. `open-agents start` no longer runs a daemon: it resolves the installed app (or downloads the latest release), opens it, and exits.

## Syntax

```
open-agents start [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output start result as JSON | - |

## Examples

```bash
# Open the Open Agents desktop app
open-agents start
```

```bash
# Open the app and get the result as JSON
open-agents start --json
```
