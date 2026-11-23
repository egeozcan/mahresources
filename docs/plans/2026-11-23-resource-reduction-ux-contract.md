# Resource Reduction UX: backend contract

What the backend lane (B1-B8 of `2026-11-23-resource-reduction-ux.md`) delivers, for the frontend lane. Everything below is implemented and tested; where the plan and the code differ, the code is described here.

Naming rule for template access: Go structs have no json tags unless stated, so in pongo2 and in the `.json` twin of a page their fields keep Go names (`review.TabCounts.Open`). Keys built as `pongo2.Context` maps are lowerCamel. Both are listed with the name the template uses.

## 1. Review page: `GET /reduction?id=N` (and `.json`)

### 1.1 Query parameters

All combine; unknown values fall back to the default and never error.

| Param | Values | Default | Meaning |
|-------|--------|---------|---------|
| `status` (repeatable) | `open`, `reviewed`, `skipped`, `stale`, `applied`, `all` | `open` when absent | `open` is every open Cluster, acted on or not (the old `reviewed` folds into it). `reviewed` is still accepted and means open Clusters the reviewer acted on. `all` is no state filter. Several values are an OR. |
| `tier` (repeatable) | `identical`, `near` | none | unchanged |
| `attention` | `true` | off | unchanged meaning, now also matches `KeepsSmaller` |
| `approval` | `approved`, `unapproved` | none | `approved` = checked, `unapproved` = not checked |
| `sort` | `copies`, `size`, `computed` | `copies` | `copies`: most members first. `size`: largest total member file size first. `computed`: the plan's own order. Ties break by Cluster id, so pages are stable across refreshes. |
| `pageSize` | `20`, `50`, `100` | `20` | anything else is 20 |
| `page` | 1-based | 1 | unchanged |

A Cluster the viewer may not fully see ("withheld") matches no status, tier, approval or attention filter and is on no tab, so the default view never shows withheld stubs. They only appear with `status=all` and no other filter, as redacted stubs (unchanged).

### 1.2 New and changed top-level context keys

| Key | Type | Meaning |
|-----|------|---------|
| `statusTab` | string | The tab the page is on: first requested status, or `open`. |
| `tabCounts` | map `{open, skipped, stale, applied}` ints | Clusters per tab over visible Clusters only, ignoring every filter and chip. |
| `reviewSort` | string | Effective sort: `copies`, `size` or `computed`. |
| `reviewApproval` | string | `approved`, `unapproved` or `""`. |
| `pageSize` | int | Effective page size. Also `review.PageSize`. |
| `pageSizes` | `[20, 50, 100]` | The offered sizes. |
| `reviewFilterActive` | bool | True when the reviewer set a filter. The default `open` tab does not count. Now includes `approval`. |
| `applyFigures` | map | `{identical: {clusters, losers, keepAsVersion}, near: {clusters, losers, keepAsVersion}, keepsSmaller}`. Over approved, open, fully visible Clusters of the whole plan. Fully visible means every member, ejected ones included, is one the viewer may see: the same rule as the tabs, bulk and apply. `losers` is copies that would be removed. `keepAsVersion` is the Reduction's flag for that tier. `keepsSmaller` is how many of those approved Clusters keep a lower-resolution copy. |
| `bulk` | map `{identicalApprovable, identicalOpen}` | `identicalApprovable` is the number on `Approve all Identical (n)`: open, fully visible Identical Clusters that do not need attention. `identicalOpen` is the count for the matching uncheck (attention Clusters included). Pass these as `expectedCount`. |
| `settingsChanged` | bool | The row's matching mode, winner rule or exclude-external differs from what the plan was computed with. False for plans computed before this change. Keep-as-version flags are not compared. |
| `lastApply` | null or map | `{startedAt, finishedAt, applied, removed, stale, running, interrupted}`. Times are RFC 3339 strings, `finishedAt` is null while running. `running` is true until finished. `interrupted` is true when `running` for longer than 30 minutes (the apply ceiling): then Clusters it did not reach are still approved. Null before the first apply. Also `countsHidden` (bool): true when `applied`, `removed` and `stale` were zeroed because a scoped viewer can no longer see every Cluster they count; render the times and state without numbers ("The last apply finished at ..."). Always false for an unrestricted viewer. |
| `extentAll` | bool | Whole-library Reduction: render `Everything you can access`, hide the exclude-external option. |

Unchanged and still present: `reduction`, `raw`, `review`, `clusters`, `coverage`, `pagination`, `winnerRule`, `extent`, `winnerCriteria`, `winnerRuleSlots`, `clusterStatusOptions`, `clusterTierOptions`, `attentionActive`. `clusterStatusOptions` is built from the request's own `status` values (empty by default), not from the defaulted `open`; use `statusTab`.

`review.ClusterCount` is the count after filters (what pagination uses). `review.CheckedCount` and `review.CheckedLoserCount` remain whole-plan, visible-only, and equal `applyFigures` sums.

Not yet computed (`Status` draft): `tabCounts` are zero, `lastApply` null, no Clusters.

### 1.3 Per-Cluster view (`clusters[i]`, a `ReductionClusterView`)

The embedded Cluster keeps its json tags (`id`, `tier`, `winnerId`, `members`, `decidedBy`, `margin`, `undecided`, `lossy`, `state`, `checked`, `reviewed`, `oversized`, `staleReason`, `appliedAt`, `merged`) and gains:

| Field | Type | Meaning |
|-------|------|---------|
| `keepsSmaller` (embedded, json) / `KeepsSmaller` (template) | bool | The kept copy has fewer pixels than a copy being removed. Pixels only. Suppressed when the first criterion of the rule the plan was computed under (`plan.settings.winnerRule`; the row's current rule only for a plan with none) is `pixels_asc`, never suppressed after a manual promotion. Recomputed on promote, eject and restore. For a plan stored before the flag existed (no `plan.settings`) the review derives it at read time, so the card, `applyFigures`, the attention filter and bulk approval all agree. |
| `NeedsAttention` | bool | Lossy, Oversized or KeepsSmaller. Bulk approval skips these. |
| `KeptDimensions`, `LargerDimensions` | string | `"800×533"`, `"1600×1066"`: the kept copy and the largest copy being removed. Empty unless `KeepsSmaller`. |
| `LargestMemberID` | uint | The copy `Keep the larger copy instead` promotes (`action=promote`, `resourceId`). 0 unless `KeepsSmaller`. |
| `DecidedByLabel` | string | Now also `you chose it` for `decidedBy = "manual"`. |

`decidedBy` is `manual` after any promotion, with empty `margin` and `undecided` false. A withheld Cluster is the redacted stub as before: all new fields are zero.

### 1.4 Per-member view (`clusters[i].Members[j]`)

Existing: `Resource` (full model: `Width`, `Height`, `FileSize`, `ContentType`, `CreatedAt`, ...), `IsWinner`, `IsLoser`, `DistanceLabel`, plus the embedded member (`resourceId`, `hash`, `inExtent`, `ejected`, `ejectedReason`, `distance`). New: `TagCount`, `NoteCount`, `GroupCount` (ints, counted through the viewer's scope; 0 when `Resource` is nil).

## 2. List page: `GET /reductions` (and `.json`)

Each `reductions[i]` row gains:

| Field | Type | Meaning |
|-------|------|---------|
| `Progress` | `{Clusters, Open, Approved, Skipped, Stale, Applied}` ints | Over visible Clusters. `Open` is state open (includes the approved ones), `Approved` is the open and checked part. Zero before the first compute. |
| `DonePercent` | int 0-100 | `(Applied + Skipped) * 100 / Clusters`. |
| `ActionKind`, `ActionLabel` | string | `find` / `Find duplicates` (not computed), `retry` / `Try again` (failed), `continue` / `Continue review` (ready with open Clusters), otherwise `view` / `View` (also computing). |

Computed on read for the page of rows; a scope-confined viewer's counts exclude withheld Clusters.

## 3. API

### 3.1 `POST /v1/reduction/clusters` (operationId `bulkCheckResourceReductionClusters`, MCP class `mutating`)

JSON or form body:

| Field | Type | Notes |
|-------|------|-------|
| `id` | uint | the Reduction |
| `version` | uint | the version the caller last saw; stale is 409 |
| `action` | `check` or `uncheck` | anything else is 400 |
| `clusterIds` | string[] | the ids the page rendered, any tier. Exactly one of `clusterIds` / `allIdentical`. |
| `allIdentical` | bool | every open Identical Cluster in the whole plan; never the Near tier |
| `expectedCount` | uint | required with `allIdentical`; `bulk.identicalApprovable` for `check`, `bulk.identicalOpen` for `uncheck` |

Response `200`: `{"id", "version", "changed", "needsReview"}`. `version` is the new version for the next write. `changed` is how many Clusters flipped approval. `needsReview` is how many Clusters a `check` left untouched because they need attention (lossy, oversized, keeps smaller); those are not marked reviewed. Every other targeted Cluster is marked reviewed (frozen), as a single act does. `uncheck` never skips.

Errors: `400` neither or both of `clusterIds`/`allIdentical`, bad `action`, `allIdentical` without `expectedCount`. `409` stale `version`, `expectedCount` mismatch, any named Cluster not open (skipped, stale, applied are reopened or settled one at a time), Reduction still computing. An unknown id, or one the viewer may not fully see, refuses the whole request with the same error as one unknown Cluster in `POST /v1/reduction/cluster`. Nothing is written on any error. The visibility check is repeated inside the write (members selected through the viewer's scope with a share lock, Postgres), so a Resource leaving their subtree mid-request fails the same way; `POST /v1/reduction/cluster` has the same guarantee.

### 3.2 `POST /v1/reduction` (create): new field

`all` (bool, JSON or form `all=true`): whole-library Reduction. `resourceIds`/`groupIds` may then be empty. Stored as `Extent.All`. With `id` set, `all=true` widens an existing Reduction to the whole library. Without `all`, an empty selection is still 400. `matchingMode`, `winnerRule`, `excludeExternalResources` and the keep-as-version flags were already accepted.

### 3.3 `POST /v1/reduction/apply`: behavior change

Request: `{id, version, expectedClusters?, expectedLosers?}` (JSON or form). `expectedClusters` and `expectedLosers` are the figures the apply dialog confirmed: `applyFigures.identical.clusters + near.clusters` and `identical.losers + near.losers` as of the page the dialog read. When present, the apply is refused with `409` (`what this apply would merge changed since you confirmed it; reload and review again`) and nothing is written unless the open, approved, fully visible Clusters the caller can see right now add up to exactly those numbers. Each is checked only when sent; omit both only for a caller that confirmed nothing. The set the apply acts on is fixed once, before the claim, so a scope change during the batch cannot widen it.

The response is unchanged (`applied`, `stale`, `destroyed`) except that a Cluster the caller may not fully see (any member, ejected included, outside their access) is left completely untouched and does not appear in either list; the old `withheld` outcome flag is gone. The batch runs on a context detached from the request (30 minute ceiling), so a closed tab does not strand a Cluster; once the ceiling passes no new Cluster starts, the one in flight is settled on its own short context (a failed merge is demoted to stale), and Clusters not reached stay approved and open. It writes `lastApply` running at the claim and finished at the end (section 1.2). A second apply is refused with `409` while `lastApply` is running and younger than the ceiling (including one that read a version after the first claim), and allowed once it is `interrupted`.

### 3.4 `POST /v1/reduction/cluster` (single override)

Promote now sets `decidedBy = "manual"`. Nothing else changed.

## 4. Plan document additions (`raw.Plan` is not exposed; for reference)

`plan.settings {matchingMode, winnerRule, excludeExternal}` written at compute. `plan.lastApply {startedAt, finishedAt, applied, removed, stale}` written by apply, preserved by compute and every override. Cluster `keepsSmaller`. Extent `all`.

## 5. Redaction audit of the new keys

Withheld Clusters are in no count: `tabCounts`, `applyFigures`, `bulk`, list `Progress`, bulk `changed`/`needsReview`/`expectedCount`, the apply result and its `expectedClusters`/`expectedLosers` check. The new per-Cluster fields are zero on a redacted stub. Test: `server/api_tests/resource_reduction_context_keys_test.go`.

`lastApply` counts what the last apply did. The record stores the ids of the Clusters it counted; a scoped viewer is shown the numbers only while every one of them is still fully visible to them, otherwise `countsHidden` is true and the numbers are zero. A Cluster withheld from the applier is not in the apply result at all, so it is in no stored count.

## 6. Things the frontend should know

- The existing `Status=open` link semantic changed: it now includes acted-on Clusters. `status=all` restores the old unfiltered list.
- The default view hides withheld stubs, so a scope-confined reviewer sees `Open (n)` with n over visible Clusters only.
- `Approve page` should send `clusterIds` from the rendered `clusters[].id`, skipping withheld stubs (their `id` is empty).
- Apply is still synchronous; reload after a lost response and read `lastApply`.
- Whole-library clustering walks the library through the scoped handle in id pages. Identical-tier compute on a very large library is slower than a group-scoped one (no `GROUP BY hash` shortcut yet).

## 7. Addendum: `reviewSelection` (D8)

Top-level review-page key (also in `.json`): `reviewSelection = {groups: [{id, name}], resources: [{id, name}], moreGroups, moreResources}`. First 20 of each by id, only what the viewer can see; invisible ones are in neither the lists nor `more*`. Empty lists for a whole-library Reduction (`extentAll`). Cost: one visibility read plus one name read per kind, through the scoped handle.
