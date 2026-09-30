# Board — ready lane (ready to merge)

Mergeable + approved sessions. Derived terminal-ready state before merge; `merged` itself is a historical terminal state.

- Ready treatment uses `--color-status-ready`; merged history uses `--color-status-merged`. Never color-only: pair with the PR glyph and label for accessibility.
- Merge executes through the daemon PR action engine (`POST /prs/{id}/merge`), surfaced in CLI as `open-agents pr merge`.
- Archive is a separate history state, not a fifth lane.

References: [interfaces/cli.md](../../interfaces/cli.md), [frontend/design-system.md](../design-system.md) §5.
