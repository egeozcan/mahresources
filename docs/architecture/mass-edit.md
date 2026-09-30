# Mass Edit

`POST /v1/{resources,notes,groups}/massEdit` applies several ops (tags, related
groups/notes/resources, owner, meta) to many entities in **one transaction**,
targeting either an explicit id list or **every entity matching the list page's
current filter** — the raw query string re-run server-side through the entity's
real search scope. It is the one write in the app that reaches past the sticky
bulk bar's per-page, per-op shape.

- **Target resolution rests on `Pluck`, never `Scan`.** `Pluck` runs the Query
  callback chain so `scopeReadCallback` appends the subtree predicate; `Scan`
  does not. A well-meaning rewrite to `Scan` hands a group-limited principal the
  whole database, and `TestMassEditPluckIsScoped` is the only thing that catches
  it. Pages are keyed on the primary key (`Offset` is O(n²) and drifts), the
  filter is decoded through `query_models.Decode{Resource,Note,Group}Filter`, and
  `MaxResults`/`SortBy` are zeroed after the decode — they are pagination and
  presentation, not predicate; leaving `MaxResults` would silently mass-edit
  only the first page's worth. `MetaQuery` survives via the extracted
  `FillMetaQueryFromValues` (gorilla/schema converters never fire for slices; a
  dropped one widens the set beyond what the reader was shown).
- **`ExpectedCount` is an exact match, answered 409.** The number in the
  confirmation dialog is the number the server acts on, or nobody acts.
  `DryRun` produces that count without committing. `ErrMassEditSetChanged` and
  `ErrMassEditOwnershipCycle` are 409, `ErrMassEditOwnerClearScoped` 403,
  `ErrMassEditTooLarge` 400 — all typed, matched by `errors.Is` **before**
  error_status.go's substring scan.
- **All-or-nothing, ops outer, chunks inner.** Chunking (500 ids per statement,
  `massEditChunkSize`) is a bind-count strategy, not a transaction strategy —
  filter-mode partial success is unrecoverable because the successful edits
  changed the filter's own result set. The owner op is parsed last and applied
  last: re-parenting changes subtree membership and the scope allow-list is a
  snapshot from request start. Owner/group re-parent validation runs on `tx`,
  and group cycles are **refused, never repaired** — unlike `MergeGroups`, where
  NULLing an owner is cleanup of an incidental cycle, here the cycle is the
  primary effect of what was asked.
- **Every dependency is LOCKED, in one canonical phase, before anything
  applies.** The raw join-table statements are `Exec` that no GORM callback
  touches, and Postgres migrations create no join-table foreign keys — so a
  plain Count gate is check-then-act twice over. All requirements (targets,
  far endpoints, owner) are UNIONED BY MODEL and taken in one fixed model
  order (`validateAndLockIDs`, the non-generic core
  `ValidateAndLockAssociationIDs` was refactored to expose; order groups →
  notes → resources → tags, matching the upload path's validation order), ids
  ascending within each model. Role-based phases would invert: two edits with
  mirrored roles (A targets group 1 and references group 2, B the reverse)
  would take the same row locks in opposite orders and deadlock with 40P01.
  Tags are global — existence-locked, no subtree predicate; on remove they
  carry no requirement at all.
- **Scoped replace is one atomic DELETE, not a classification pass.** Which
  existing edges may go is decided by the statement itself: unscoped callers
  get the plain `NOT IN` (or unqualified, for the empty new set — the
  `IN (NULL)` trap) shape; scoped callers get the visibility condition
  inlined — an edge goes when its far endpoint is inside the caller's subtree
  OR the far row is gone (dangling; Postgres creates no join-table FKs), and
  only a LIVE far endpoint outside the subtree is spared
  (`deleteRelationsSparingUnseen`'s contract, immune to a concurrent commit
  moving an edge between the "visible" and "spared" buckets between two
  probes). `dropSelf` families additionally sweep pre-existing self-edges even
  when the new set names the target itself.
- **Every unbounded request-controlled bind is budgeted at parse time.**
  `massEditBindBudget` computes the live dialect limit (from the Dialector,
  not the configured DbType) minus the request's scope allow-list, the target
  chunk and a margin; remove/replace far sets, meta-key lists, Postgres
  merge null-keys and decoded-filter id lists are all refused ABOVE it with
  `ErrMassEditTooLarge` — at parse, so a DryRun and the confirmed submit fail
  identically instead of the submit dying with a driver error. (NOT IN
  semantics do not survive chunking, which is why the far set cannot simply be
  split like the targets can.)

- **The materialized id set is authoritative.** Count and Pluck are two
  queries; anything that moves the set between them (insert, delete, re-parent)
  is the same conflict a stale `ExpectedCount` is, so the plucked length is
  re-checked against the confirmed count after resolution, and the pluck
  aborts early once the ceiling is passed. The ceiling itself applies in ids
  mode too — an explicit selection holds the same write lock a filter-resolved
  set does.
- **An empty filter string is legitimate**: `Target=filter` with no query means
  every entity the caller can see — the set the unfiltered list page shows —
  still gated by `ExpectedCount`, the ceiling and the scope predicate.
- **Resource meta ops keep the series contract.** Merge/removeKeys run the
  extracted `applySeriesNullOverrides` on the **transaction's** db (the old
  `BulkAddMetaToResources` fixup ran on `ctx.db` after the update, so a crash
  between them let a removed key silently reappear from the series), and merge
  carries the `COALESCE(NULLIF(...))` wrapper that path was missing — bare
  `json_patch(meta,?)`/`meta || ?` NULLs the column when `meta` is SQL NULL.
  Postgres merge chains a `- ?` removal **per explicit-null key** after `||`,
  because jsonb `||` stores `"k": null` where SQLite's `json_patch` deletes it —
  the two engines must stay JSON-equivalent. removeKeys is **not** "merge a
  null" (`json_patch` deletes it, `||` stores it) — the removal is a real
  `json_remove`/`-` on both columns, with the JSON path **bound and quoted**
  (`$."k"`), never `fmt.Sprintf`'d, and keys containing `.`/`$`/`"` refused.
  Replace on series rows splits: non-series rows take one statement per chunk;
  series rows get `own_meta = computeOwnMeta(submitted, series.Meta, true)` per
  row, the same call `EditResource` makes.
- **Hooks are one veto-only pair.** `before_mass_edit` / `after_mass_edit` fire
  once per request, no field rewriting, no per-row hooks — per-row Lua inside a
  10,000-row write transaction on single-writer SQLite is exactly what
  `BulkDeleteResources`' hook pair avoids. The id list is omitted from the
  before-payload above 100 entities.
- **No role guard, by the same argument as every other bulk endpoint:** the
  HTTP layer gates at `capWrite`; adding an application-layer check would make
  mass-editing an owner stricter than `EditResource` doing the same thing one
  row at a time.
- **The UI is one modal** (`templates/partials/massEditModal.tpl` +
  `src/components/massEdit.js`), opened from the bulk bar (selection) or from a
  "Mass edit all N results" button (filter mode). The panel probes with `DryRun`,
  sends the probe's count as `ExpectedCount`, confirms through `askToConfirm`
  whenever the target is filter mode or any verb discards data, and refreshes
  with the same `.body` refetch + `Alpine.morph` routine as the bulk toolbar.
  The list pages publish `totalCount` and `massEditEntity` so the modal can be
  included once from `base.tpl`.
