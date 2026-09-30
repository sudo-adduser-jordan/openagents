# Board — build lane (working)

Actively working sessions (`activity_state = active`, no blocking PR state). The board's Working lane is the default operational overview — not a KPI dashboard.

- One fixed status slot: working spinner while observed activity is live.
- Card order: agent avatar + task title; branch (mono, muted) only when it adds identity; PR/review evidence only when present; one derived status line; compact time/usage metadata with tabular figures.
- Lanes are one continuous grid with shared vertical dividers; compact header (semantic dot, sentence-case name, count). Never a rounded panel per lane.
- One hover/focus action slot; must not shift the title. Destructive actions stay quiet until hover/focus but keyboard-reachable.

References: [frontend/design-system.md](../design-system.md) §9, [overview.md](../../overview.md).
