# Board — review lane

Sessions with actionable PR/review facts: CI state, review threads, mergeability. Placement is derived from PR facts at read time — never stored.

- Shows failing check names + links, reviewer IDs/counts/links for unresolved threads, mergeability reasons. Raw CI logs and comment bodies are intentionally excluded from the V1 card.
- A card frozen in review by the kanban review lock is released by setting any workflow stage (`plan`/`manage`/`build`) so it can move with its PR facts again.
- Needs-human attention may use the semantic needs-you color and restrained motion. Do not pulse multiple cards or animate lane counts.

References: [backend/scm-observer.md](../../backend/scm-observer.md), [backend/lifecycle-status.md](../../backend/lifecycle-status.md), `frontend/src/renderer/lib/pr-display.ts`.
