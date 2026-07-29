# Adversarial review round 3 — feature/user-accounts-rbac

Two more P1 scope-leak findings (RBAC group-subtree confinement), both verified
against the code, fixed, and regression-tested (TDD red→green). SQLite + Postgres
green; full browser+CLI E2E green.

- [x] **Plugin block rendering ignored group scope** — `server/routes.go:664` wired
      the `/v1/plugins/{pluginName}/block/render` route with the **unscoped singleton**
      context, so `GetBlock`/`GetNote` ran without the caller's subtree filter. A
      group-limited user/guest could render an out-of-subtree plugin block (and leak
      its note name) by guessing the block ID. `GetBlock` already enforces
      `NoteVisible(block.NoteID)` and `GetNote` is scope-filtered — the only defect was
      the route passing the wrong context. Fix: route through `scopedCtx(appContext, r)`.
      (`/display/render` loads no entity by ID, so it is unaffected and left as-is.)
      Test: `TestScopedUser_PluginBlockRenderConfined` (out-of-subtree → 404;
      in-subtree → reaches handler past the visibility gate; admin → never blocked).

- [x] **Thumbnail mutations lost the GORM scope filter** — `SetCustomThumbnail` /
      `ClearThumbnails` (`application_context/resource_custom_thumbnail_context.go`)
      re-bind the DB to the bare HTTP request context via `ctx.db.WithContext(httpContext)`
      for cancellation, which **drops** the scope filter (it lives only in the GORM
      statement context, `scoping.go:127`). Preserving the filter alone would not fix
      `ClearThumbnails`, because the delete targets the `previews` table, which is not
      owner-scoped (`scopeColumn` covers only groups/resources/notes). So a group-limited
      principal could set or clear previews for an arbitrary resource ID. Fix: assert
      `ctx.ResourceVisible(resourceID)` before mutating in both methods (no-op for
      unscoped/admin; fail-closed for a scoped principal whose subtree is unresolvable),
      and map the resulting not-found to **404** in the handlers via `statusCodeForError`
      (out-of-scope and genuinely-missing resources are indistinguishable to the caller).
      Test: `TestScopedUser_ThumbnailMutationsConfined` (out-of-subtree set/clear fail and
      mutate nothing; in-subtree set/clear succeed; admin may mutate any resource).

Root-cause note for future work: any code path that calls
`ctx.db.WithContext(<raw request context>)` silently discards subtree scoping. The
remaining such call sites (`resource_media_context.go`, `mrql_context.go`) either run
on the auto-thumbnail/worker path behind a prior scoped resource lookup, or are the
MRQL executors that force scope explicitly via `ApplyScopeCTE`/`subtreeScopeIDs`. The
two thumbnail mutations were the only ones reachable as a direct by-ID mutation without
a preceding scoped visibility check.
