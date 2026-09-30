# Board — worker card

One card = one session. The legitimate exception to the "no individual borders" rule: a bounded, draggable/clickable actionable object with a quiet edge.

Information order (visual weight in order):

1. Agent avatar (small, recognisable) + task title (primary weight).
2. Branch only when it adds identity (mono, muted).
3. PR/review evidence only when present (compact evidence links).
4. One derived status line (single semantic treatment — spinner, PR glyph, or dot; never multiple chips).
5. Compact time/usage metadata.

Avoid duplicate status words, repeated provider names, and metric clutter.

References: [frontend/design-system.md](../design-system.md) §9, `frontend/packages/product-ui/src/SessionsBoardView.tsx`.
