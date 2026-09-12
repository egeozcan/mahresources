# Shared entity picker — final inline review

Baseline: `c7df44b3` (approved spec/plan, before implementation). Reviewed the
implementation commits through `322f828b` and the Task 9 working-tree fixes.
Spec: `docs/superpowers/specs/2026-09-12-shared-entity-picker-design.md`.
Standards: `CLAUDE.md`, selector architecture, and the approved implementation plan.

This is an **inline self-review**, following the requested inline/no-subagent
execution mode. It is not a fresh-context or independent review. Validation
results and the final commit are recorded in `docs/todo.md`.

## Standards

- Dependency direction and typed Reader contracts remain intact. Database scope
  and transaction membership ride on the per-call handle. Browse identity
  collection uses scoped `Pluck` subqueries, then bounded page hydration.
- Autocomplete stays separate from browse transport. The session owns request
  generations; the originating profile owns one normal non-silent replacement
  and ordinary persistence. No hidden-input scraping or selected-array mutation.
- The dialog owns one cancellable trap, restores prior background inertness and
  scrolling, and keeps host selection controls outside ignored custom content.
  The legacy parent modals suspend their traps while it is active.
- Template fields travel through models, DTOs, partial edits, plugin adapters,
  bundles, generation, preview, CLI and additive archive fields. CLI file errors
  occur before HTTP; partial edits distinguish absent from explicitly empty.
- Fixed a projection issue found during review: note `Description` is its body,
  not a small selector description. It is omitted from selector state without
  mutating the entity available to custom rendering.
- No remaining blocking standards issue identified in this review. Some existing
  compact frontend formatting remains; this was not used as a reason to broaden
  the implementation into a repository-wide formatting change.

## Spec

- All ten catalog types and host/profile families have browse metadata and
  buttons, including nested filters, standalone mounts, compare/upload fields,
  block references, plugin parameters and autosaving lightbox tags.
- Explicit paging, pending-set retention, single Confirm, multi append/dedup,
  live constraints/capacity/exclusions and dead-origin cancellation are covered.
  Resource note-tab filtering uses associations, not coincident owner IDs.
- Default identifying content and three custom carriers are exercised in actual
  dialogs. New-tab links do not select; CSS is deduplicated/replaced; failed
  templates log and fall back rather than borrowing CustomSummary.
- Real authenticated HTTP coverage proves nested custom MRQL cannot reach an
  outside subtree. A budget regression fails when the render-context builder is
  replaced with plain cancellation, and passes with the builder restored.
- PostgreSQL regressions exercise tied names, hidden rows preceding a page,
  intersecting category constraints, metadata/note associations and ordered
  constrained resolution.
- The remaining CLI clear/file-input requirement needed partial `edit --id`
  commands for categories and resource categories, plus the two explicit picker
  `-file` flags. Existing name/description commands remain unchanged.
- No remaining blocking functional omission identified in this review.

## Validation findings

The full suite exposed broad test locators that matched the new Browse buttons
or hidden pager navigation, and a fixed sixty-Tab budget. Tests now identify the
actual named control/landmark and bound keyboard traversal by the page's controls.
The static `Select` heading fallback preserves the server-side heading boundary.

Accessibility diagnostics previously discarded everything after "Fix any of the
following". They now retain the actual cause. The full-suite screenshot exposed
an unbounded, nowrap category badge painting across its card into the sidebar,
covering half a checkbox. The new long-name fixture made this existing layout
bug reproducible. A geometry regression measured the badge reaching x=3942 on a
1280px page. Shared card badges now constrain their width and wrap long text.
The earlier checkbox-sizing and audit-scroll experiments were withdrawn: they
did not address the covering element. No rules are disabled, scans are not
moved to hide failures, and no violations are allowlisted. A schema-route test
also waits for active handlers during teardown instead of un-routing a fetch
that still intends to fulfill its response.

## Limits

Pagination is deterministic but not a database snapshot. Custom CSS is global,
not isolated. Large pending selections resolve in bounded batches; they are not
an all-matching selection or a database transaction. Normal write endpoints still
validate permissions. This review provides no claim of independent sign-off.
