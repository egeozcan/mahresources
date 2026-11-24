# Persist a Resource Reduction as a database row, not a file-backed plan

> **Superseded by [0011](0011-reduction-clusters-live-in-tables.md)** in one respect. A Reduction is still one row owned through `CreatedByUserId`, but its plan (every Cluster, member and decision) is no longer a JSON value on that row: it lives in tables. The reasoning below about owner authorization, resumability and the missing retention sweep still holds.

Group import already implements compute-review-apply: a parse job writes an immutable `ImportPlan` to `_imports/<jobId>.plan.json`, the browser renders a review UI, and apply posts a separate `ImportDecisions` payload validated against the plan. We deliberately do not copy that shape.

Two of its properties are wrong here. Review decisions live only in an Alpine object with no rehydration, so reloading the page loses the entire review. And authorization for the plan is re-derived from the in-memory job record, which is swept an hour after completion — so a non-admin owner is eventually locked out of their own artifact, because the files outlive the only thing that says who owns them.

A Resource Reduction is named, accumulated across several sittings, and applied in parts, so the review is the thing most worth protecting — and a meaningful review is the entire justification for the feature. One row carrying the plan and the decisions as JSON takes its authorization from the existing `CreatedByUserId` machinery, survives a reload and a restart, and is resumable.

## Consequences

A 28th `AutoMigrate` model, and a table with no retention sweep. Both accepted: a Resource Reduction is domain data the user named and curated, not an expiring artifact like an export tar. Visibility is not automatic — `scopeColumn` maps only `groups`, `resources` and `notes` — so the owner-restricted predicate must be written explicitly, following `DownloadHistoryQuery`.
