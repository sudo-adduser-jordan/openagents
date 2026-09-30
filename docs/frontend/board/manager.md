# Board — manager card and flow

Managers coordinate; workers execute. A manager starts in Manager mode, can delegate from that mode, and cannot delegate while in Planning mode.

- `open-agents manage <id>` puts a manager into Manager mode; `open-agents manager ls` lists managers (`GET /api/v1/managers`).
- Workers delegated by a manager start in Planning with the same standing instructions as building workers.
- Manager narrative continuity is project-scoped; worker history is session-scoped (see [backend/storage-cdc.md](../../backend/storage-cdc.md)).
- The spawn-manager flow lives in the shell; session cards for managers follow the same worker-card layout with manager-specific actions.

References: [interfaces/cli.md](../../interfaces/cli.md), [operations/status.md](../../operations/status.md).
