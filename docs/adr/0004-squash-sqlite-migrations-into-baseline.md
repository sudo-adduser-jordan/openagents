# 4. Squash the SQLite migration chain into one baseline

Date: 2026-09-26
Status: Accepted

## Context

The schema was built by 139 migration files spanning goose versions 1 to 155.
Sixteen version numbers (22, 45, 46, 50, 51, 55-65) had been issued and then
withdrawn. Several migrations had been renumbered after the fact, and each
renumbering left behind a repair pass in `db.go` that rewrote the goose ledger
so the next build would not refuse to start. Burned version numbers needed their
own repair. A missing `sessions_revision_update` trigger needed a
`reconcileSchema` pass that recreated it, because a burned version meant the
trigger might never have been created. The `sessions.harness` CHECK constraint
needed a string-rewriting pass that widened it, and four tests existed only to
catch that rewrite silently no-opping.

The result was roughly 1,340 lines of repair code in `db.go` (1,699 lines total)
plus 3,359 lines of migration tests, all of it existing to serve databases that
were never released to anyone. The per-migration tests dominated the slowest
package in the Go suite: `internal/storage/sqlite` had a 151.6s floor, because
each test replayed the chain up to a historical version and then ran the
migration under test. The same 139 files were also the reason a fresh install
spent 0.60s inside `migrate()` before touching any data.

The chain also cost correctness. `goose.Up` was called with
`WithAllowMissing()` so a build that had advanced past a migration later added
or renumbered upstream would still start. That traded a loud startup failure for
a silent one: goose would treat the unknown versions as already applied and
leave the schema untouched, so the daemon came up healthy against a schema it
did not understand.

## Decision

Collapse the chain into a single `migrations/0001_baseline.sql` and delete the
repair layer with it.

The baseline is the current table state, not a replay: 36 tables, 57 indexes, 3
views, 30 triggers, plus the one row of seed data the chain created
(`app_settings`). Tables are emitted in a stable topological order so no foreign
key points forward. Its `Down` section drops every object in reverse creation
order, which is a closer approximation of reversible than the chain's own
`Down` sections were.

Databases stamped by the retired chain are **refused, not migrated**.
`rejectRetiredChainDatabase` refuses to start when the goose ledger records an
applied version that no embedded migration declares. The test is set membership
rather than a version threshold, so it stays correct as versions 2, 3, ... are
appended and never needs updating. goose's own version-0 bootstrap row is
treated as always-declared.

Refusing is a deliberate trade. A developer or pilot user on an auto-updated
build hits an error naming the data directory and telling them to delete
`open-agents.db`; they lose local session history and nothing else. The
alternative — accepting the database — risks the daemon reporting healthy
against a schema it cannot read, which is the failure the entire repair layer
was written to prevent. A loud, actionable, data-preserving refusal is the better
of the two, and it is bounded to installs that predate the baseline.

Because the chain is now append-only, `WithAllowMissing()` is removed. An
out-of-order ledger is a real fault, and goose rejecting it is the wanted
behaviour rather than something to work around.

The per-migration upgrade tests are deleted with the steps they exercised. What
replaces them is direct assertion on the resulting schema: that the baseline
admits every harness the domain ships, that it creates the session revision
fence, that migration version numbers stay unique, and that a fresh database
reaches the intended state. These describe the state the code depends on rather
than the path taken to reach it, so they are stronger checks, not weaker ones.

## Consequences

- `internal/storage/sqlite` drops from a 151.6s floor to about 11s, and
  `migrate()` on a fresh install from 0.60s to 0.03s.
- `db.go` goes from 1,699 lines to 382, and 1,340 lines of renumber-repair code
  and 3,359 lines of migration tests are gone.
- `npm run sqlc` produces byte-identical output in `gen/` across the squash,
  which is the strongest available proof that the baseline reproduces the
  chain's schema: sqlc derives its understanding by replaying the migrations
  directory, so identical generated code means an identical schema.
- The baseline is hand-maintained, which makes a new failure mode possible: a
  migration meant to widen a CHECK constraint that silently no-ops. That is what
  `TestBaselineAdmitsEveryShippedHarness` guards, and it was already a real
  failure mode under the old hand-written `replace()`.
- AGENTS.md's "do not modify already-merged migrations" rule is retained and
  tightened, not relaxed. The baseline is not an editable convenience; it is
  still changed only by appending a new version.
- Any install whose database predates the baseline needs a rebuild. This must
  ship as a release note, not as a silent behaviour change.
