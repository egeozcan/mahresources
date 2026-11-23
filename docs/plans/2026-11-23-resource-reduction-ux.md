# Resource Reduction: review workflow and UX overhaul

Status: approved for implementation (evaluation rounds: v2 6.5, v3 7.5, v4 8.0; round-3 notes folded in)

## Problem

Resource Reduction (#57, `docs/plans/2026-08-26-resource-reduction.md`) is safe but slow
and confusing to use. Its safety design stays as it is; this plan is about the person
reviewing two hundred Clusters in one sitting. Sources: code reading of the review, list,
create and bulk templates and `src/components/reductionReview.js`, plus a live audit
(ephemeral server, 24 images, desktop 1440 and mobile 390, screenshots in the session
scratchpad).

1. **The work is below the fold.** Warning banner, Details, Extent and Coverage come first;
   the first Cluster starts at y≈745 of 900. Apply and the checked count sit at the top of a
   non-sticky sidebar and scroll away; on mobile they hide in the collapsed sidebar panel.
2. **One click per Cluster.** No bulk approval, no keyboard flow. The first Cluster checkbox
   is tab stop 50 of 115; each Cluster adds about 10 stops.
3. **The default view mixes everything.** The status filter defaults to all statuses, so
   applied Clusters ("Merged and deleted." placeholders) stay above open ones forever.
4. **Computing is a dead end in the create flow.** Creating never computes, so every
   Reduction needs a second trip; before compute the page shows "0 of 0 resources have a
   content hash", "0 Resources and 2 Groups selected" and live Apply and filter controls
   that do nothing.
5. **Promoting a worse copy is silent (P0).** `Make Winner` on an 800x533 copy turned the
   1600x1066 original into "Will be deleted", "Chosen by" went blank, and the apply dialog
   said nothing about it.
6. **Disk space is misrepresented (P0).** The dialog says "delete 6 Resources". Applying
   frees no disk space: storage is content-addressed (byte-identical copies already share
   one file), every Loser's versions are re-parented to the Winner, and any file a delete
   does release is copied to the `/deleted/` backup before removal
   (`runFileCleanupActions`, `resource_bulk_context.go`). A user deduplicating "to save
   space" is misled at every step. (The docs page's claim that "an upload always creates"
   a version row is also wrong: a never-versioned Resource has only a virtual v1, so with
   keep-as-version off its file goes to the `/deleted/` backup rather than staying as a
   version. Both end on disk.)
7. **The card makes the user squint.** 120px thumbnails, facts not compared across members,
   internal vocabulary ("Include in reduction", "Eject", "distance 2", "Extent",
   "external resources").
8. **The list page is a poor "come back later" surface.** No progress, no next action; the
   empty state explains nothing.
9. **Entry points don't say what they do** ("Resource Reduction", "Reduce"), and there is no
   way to say "my whole library".
10. **Settings are expert-only.** Six ordered selects; changing the rule silently does
    nothing until a recompute, and the rule is shown twice.

## Goals and non-goals

Goal: a reviewer goes from "I have duplicates" to "applied" in one sitting with few clicks,
always knowing what is kept, what is removed, and what happens to disk space.

Unchanged from #57: no undo, no CLI surface, no automatic recompute, no change to
clustering, revalidation, freezing, version CAS, visibility or redaction. Every new surface
obeys the redaction rules: a withheld Cluster is never counted, named or acted on,
including in `.json` and in every count this plan adds.

Explicitly not done:
- **Freeing disk space.** Content-addressed storage and the `/deleted/` backup are
  system-wide design, not this feature's to change. The plan states the truth instead (D9,
  D11): a Reduction tidies the library (one entry, merged tags, notes and groups); it does
  not reclaim space.
- **Replacing the full-page morph refresh.** Each action re-renders the page server-side and
  morphs the Clusters in place (`refresh()`); the audit found it fast and scroll-preserving
  at 20 Clusters a page. Rewriting it as per-card patches would duplicate the promote/eject
  reasoning in JavaScript, which #57 rejected for good reason. Bulk actions (D2) are what
  remove the per-click cost.
- Growing the Extent from the review page, removing from it, a Winner Rule beyond four
  presets.

## Design

### D1. Review page layout

Main column, top to bottom:

1. `Skip to Clusters` link (first focusable element of the main column).
2. **Status header**, one row: status pill, last computed time, and the one contextual
   action: `Find duplicates` (not computed, primary), progress bar (computing, D4),
   `Recompute` (ready, secondary), error text + `Try again` (failed). A `Settings` link
   moves focus to the Settings form (on mobile it opens the sidebar panel first).
3. **Notices**, only when relevant: Extent drift ("3 Resources added since the last run.
   Recompute"); poor coverage; settings changed since the last run (D7).
4. **Tabs** (a `nav` of links with `aria-current="page"`, not an ARIA tablist, since each
   is a navigation): `Open (n)` default, `Skipped (n)`, `Stale (n)`, `Applied (n)`. Counts
   are over visible Clusters only. The current `Reviewed` status folds into Open.
5. **Filter and sort row**: chips Tier (Identical / Near-Identical), Approval (Approved /
   Not approved), Needs attention; sort select `Most copies first` (default), `Largest
   files first`, `Computed order`. Chips are links that toggle a query param; the sort
   select submits on change, with a visually hidden `Sort` button for no-JS.
6. **Clusters** (D3), 20 per page by default, with a `Per page` select (20 / 50 / 100),
   so approving a long Near-Identical review takes a few `Approve page` clicks, not ten.
7. **About this Reduction**, a collapsed `<details>`: the selection by name (D8), coverage
   figures, the Winner Rule in words, created date. The read-only Winner Rule panel and the
   Details/Extent/Coverage panels are removed.

The undo warning becomes one line in the apply bar plus the dialog, not a 200px banner.

Before the first compute the main column shows only items 1-2 and a sentence: `Finds
copies of the same file and near-identical images in this selection. Nothing changes until
you review and apply.` No tabs, chips, Clusters, apply bar or coverage figures.

The sidebar keeps Settings and Delete. The sidebar filter form is removed.

### D2. Apply bar and bulk approval

A bar `position: sticky; bottom: 0` inside the main column, shown while there are open
Clusters, with a top border and the page background so cards scroll under it. The Clusters
list gets bottom padding equal to the bar's height so the last card is never covered. All
bar controls are at least 44px tall.

- Left: `18 approved · 34 copies to remove` (whole plan, server counts, as today).
- Right: `Approve page`, `Clear page`, `Approve all Identical (n)`, and the primary
  `Apply 18 Clusters…` (red, disabled at 0) with the line `Applying cannot be undone.`
- Below 640px: counts and `Apply` stay; the three bulk buttons move into a `More` menu
  button.

Bulk rules, enforced on the server (B1):

- **Never bulk-approve a Cluster that needs attention.** `Approve page` and `Approve all
  Identical` skip every Cluster where `clusterNeedsAttention` is true (Lossy, Oversized,
  and the new KeepsSmaller, D3) and report it: `Approved 16. 2 need individual review.`
  with a link to the Needs attention filter. Single approval of such a Cluster works as
  today (an oversized one still needs expanding first).
- `Approve page` takes the Cluster ids the page rendered; any tier, because the reviewer
  has them on screen.
- `Approve all Identical` runs over the whole plan, Identical tier only, so Near-Identical
  approval across unseen pages stays impossible (#57's friction where the risk is). It
  carries `expectedCount` (the number on the button); a mismatch is a 409 and the page
  refreshes the number.
- `Clear page` unchecks the page's ids.
- Bulk acts mark Clusters `Reviewed` exactly as a single act does, so they freeze.

### D3. Cluster card

- Header: `Identical · 3 copies` / `Near-Identical · 2 copies`; the approval control is a
  toggle button (`aria-pressed`, text `Approve` / `Approved ✓`, 44px), then `Skip` or
  `Reopen`.
- One sentence: `Keeping "IMG_0412.jpg": highest resolution, 4x the pixels of the next.`
  After a manual promotion: `Keeping "x.jpg": you chose it.` (promote sets
  `DecidedBy = "manual"`). Tie: `All copies tie on every criterion; keeping the one added
  first.`
- Members in a row, kept copy first, thumbnails 160px (from 120), each with a role label:
  `Keep` (green border), `Remove` (red), `Not included` (ejected, dimmed).
- Under each thumbnail: dimensions, file size, format, date added, and tags / notes /
  groups counts. A value that differs from the kept copy's is bold with a `≠` prefix
  (non-colour cue) and the accessible text `differs from the kept copy`.
- **Keeps-smaller warning (P0).** When the kept copy has fewer pixels (width × height)
  than any copy being removed, the card shows in the attention style: `The kept copy has a
  lower resolution than a copy being removed (800×533 vs 1600×1066).` with a `Keep the
  larger copy instead` button (promotes the largest). Pixels only: a smaller *file* at the
  same pixels is usually the better copy, and comparing size would flag every Cluster under
  the `Smallest file` preset. Suppressed when the rule's first criterion is `pixels_asc`
  (the user asked for the smaller copy); not suppressed after a manual promotion, because a
  mis-click there is exactly the audit's P0. Stored as `KeepsSmaller` on the Cluster and
  part of Needs attention.
- Member actions: `Keep this instead` (promote), `Don't remove` (eject), `Include again`
  (restore), `Compare` (unchanged).
- Footnote: `Removed copies' tags, notes and groups move to the kept copy.` plus the
  existing lossy warning.
- When a manual promotion also triggers the keeps-smaller warning, the two read as one
  message: `Keeping "small.jpg": you chose it.` followed by the warning and its button,
  which is the intended reading (you chose it; are you sure?).
- Near-Identical distance badge: `Very close` (0-2), `Close` (3-6), `Similar` (7+), with
  `perceptual distance N` in the accessible name and `title`.
- Applied Clusters (Applied tab only) render compact: one line and the kept copy.

UI vocabulary: Keep / Remove / Approve / selection. The API, data model and docs keep
Winner / Loser / Eject / Extent; the docs page maps the two.

### D4. Live compute

- Compute becomes a fetch from the store (no native form post). The page renders
  `ComputeJobID`; while computing the store subscribes to `/v1/jobs/events` (as
  `adminExport.js` does), shows `Comparing… 1,200 of 8,000` from the job's phase progress
  (`SetPhaseProgress` in `runReductionCompute`), and reloads the page on the terminal
  event. Before relying on it, verify that the stream's job id equals `ComputeJobID` after
  `claimReductionComputeJob` swaps the generation nonce for the job id; if the page loaded
  during the `pending:` window, it polls `GET /v1/reduction.json?id=N` until the id
  appears.
- On stream error the store falls back to polling that route every 3s.
- Failure shows the stored `ComputeError` and `Try again`.

### D5. Create and add flows compute immediately

- `/reduction/new` and the bulk panel get `Find duplicates now` (checked by default).
  After create or add, the client calls compute with the returned version and navigates to
  the review page, which shows D4 progress.
- If create succeeds and the compute call fails, the client stores the error message in
  `sessionStorage` (keyed by Reduction id, try/catch) and navigates to the review page,
  which shows `The Reduction was saved, but finding duplicates could not start: <message>.`
  and `Try again`, or, when storage is unavailable, the same sentence without the reason.
  No free text goes in the URL. Nothing is lost because the Reduction exists.
- `/reduction/new` gains an `Options` `<details>`: matching mode and Winner Rule preset
  (D7). `POST /v1/reduction` already accepts both.

### D6. Keyboard review

Shortcuts are active **only while focus is inside the Clusters region** (WCAG 2.1.4: a
single-key shortcut is active only when its component has focus), and never with a
modifier key, in a form field, or while a dialog or the lightbox is open. The shortcuts
dialog also has a `Turn off keyboard shortcuts` switch, remembered in `localStorage`
(wrapped in try/catch).

| Key | Action |
|-----|--------|
| `j` / `k` | Focus next / previous Cluster card (cards get `tabindex="-1"`; `scrollIntoView({block: "nearest"})`, instant under reduced motion) |
| `a` | Toggle approval of the focused Cluster |
| `s` | Skip / reopen it |
| `x` | Expand an oversized Cluster |
| `?` | Open the shortcuts dialog |

When `a` or `s` moves the Cluster out of the current view, focus goes to the card that took
its place, or to the previous one at the end of the page. A visible `Keyboard shortcuts`
button in the filter row opens the dialog, and the controls carry `aria-keyshortcuts`
only while shortcuts are on; turning them off removes the attribute and the key hints.

### D7. Settings

- Winner Rule preset select: `Best quality` (pixels, size, oldest; today's default),
  `Smallest file`, `Oldest copy`, `Custom`. `Custom` reveals the existing ordered selects.
  Presets are a client-side mapping to `winnerRule`; the server stores the list as today
  and the page shows whichever preset matches it.
- **Settings-changed notice without a new column.** The compute stores the settings it ran
  with in the plan (`plan.Settings`: the matching mode actually used, the winner rule
  after normalisation, exclude-external).
  When the current row's settings differ, D1 shows `Settings changed since the last run.
  Recompute to apply them to Clusters you have not acted on.` with `Recompute`. Plans
  computed before this change have no `Settings` and show no notice.
- Keep-as-version flags are not compared (they act at apply, not compute). Their labels
  become `Add removed copies as versions of the kept copy`, with the hint `Off: their
  files move to the deleted-files backup instead. They stay on disk either way.` (Versions a
  removed copy already had always move to the kept copy.)
- Settings save via fetch so the page keeps its state; a version conflict reloads.

### D8. Selection, read-only

About lists the selected Groups and Resources by name with links, first 20 of each, then
`and N more`. Groups and Resources the viewer cannot see are omitted and not counted. A
whole-library selection reads `Everything you can access`.

### D9. Apply

- **Dialog**, built from server figures (B6):
  - Per tier: `Identical: 12 Clusters, 15 copies removed.` / `Near-Identical: 6
    Clusters, 8 copies removed.` followed by what happens to their files, chosen from
    what the merge actually does, hedged where the outcome depends on reference counts
    (`countHashReferences`): flag on, `Their files are added as versions of the kept
    copy.`; flag off, `Their files move to the deleted-files backup unless a version
    still uses them.` The Identical line says `They have the same content as the kept
    copy.` (no claim about a shared file: two storage locations hold two files). Test: a
    Loser with real version history, flag off, keeps its file as a re-parented version.
  - When non-zero: `2 approved Clusters keep a lower-resolution copy than one they remove.`
    with a link to them.
  - `This does not free disk space.` and the one pointer to mechanisms that exist: `To
    release space, delete versions you no longer need from the kept copy's Versions panel.
    The deleted-files backup is emptied only by whoever runs the server.`
  - `There is no undo.`
- **Synchronous, and safe to abandon.** Apply stays one request, run on a context
  detached from the request (B7), so closing the tab or a dropped connection cannot cancel
  a claim-then-merge halfway and leave a Cluster marked applied but never merged, or a
  failed one never demoted to stale. That this happens today is first proved with a test
  that cancels the request context after the claim (TDD). The page shows a blocking
  `Applying 18 Clusters… You can leave this page; the server finishes the batch.` state.
- **The last apply is recorded in the plan** (`plan.LastApply`: started at, finished at,
  applied, copies removed, stale count). It is written as `running` at the claim and
  completed at the end of the loop. The page renders it, so:
  - a lost response (closed tab, proxy timeout) is answered on reload by the same banner,
    built from `LastApply`;
  - while `running`, every tab shows `An apply started at 16:40 is running. Reload to see
    the result.` with Apply disabled; a second tab or a double-click that still reaches
    the server is refused by the claim's CAS with `This Reduction changed since you loaded
    it, possibly by an apply in another tab. Reload.`;
  - a `running` record older than the apply ceiling (B7) reads `The last apply was
    interrupted. Applied Clusters are on the Applied tab; the rest are still approved.`
  - stale names stay on the Stale tab as today; `LastApply` holds counts only, and its
    stale count excludes `Withheld` outcomes (stored and displayed), so a scope-restricted
    owner cannot learn how many Clusters were withheld. Test for it.
  - `LastApply` is written as `running` once, by the batch-level claim at the start of
    `ApplyResourceReduction` (not by each Cluster's claim), and completed once at the end.
    Compute's plan rebuild (`storeReductionPlan`) and `mutateCluster`'s read-modify-write
    both carry `LastApply` over from the plan they read; the write-sites guard test covers
    all plan writes.
- **Result banner** at the top of the main column: applied count, copies removed, the
  disk-space sentence, and the stale list with links into the Stale tab. The page then
  shows the Open tab.

### D10. List page

Each card: name, status pill, `140 Clusters · 20 approved · 60 applied · 8 skipped · 2
stale` with a thin bar (applied + skipped over total, the "done" share), dates, and one
contextual button: `Find duplicates` (not computed), `Continue review` (open Clusters
remain), `View` (none open), `Try again` (failed). Default sort: last updated first.

Counts are computed on read from the plan (B5), not stored: Reductions are few per user,
and computing them per viewer is what keeps withheld Clusters out of the counts for a
scope-restricted owner, whose access can change after the plan was written.

Empty state: `A Resource Reduction finds copies of the same file and near-identical
images, lets you choose which copy to keep, and merges the rest into it. Nothing is
removed until you review and apply.` with `Find duplicates in everything` and `New`. That button creates a whole-library
Reduction and starts it through the D5 flow, landing on the D4 progress view.

### D11. Plain words, entry points, whole library

- Bulk bar and group page buttons: `Find duplicates`. Panel heading `Find duplicates in 12
  selected Resources` and `Nothing is removed until you review and apply.`
- The exclude checkbox becomes `Only keep copies from this selection`, hint `Otherwise a
  better copy elsewhere in your library may be kept instead.`
- Create form: a radio `Everything you can access` / `Choose groups and resources`, and the
  line `A Reduction tidies your library: one entry per image, with the others' tags, notes
  and groups merged in. It does not free disk space.`
- Whole library is stored as `Extent.All = true` (JSON field, no migration). The resolver
  iterates every Resource the principal can see with `Pluck`/`Find` only (so the subtree
  scope callback applies), in id-keyset pages. With `All` set the `Only keep copies from
  this selection` option is hidden (everything visible is in the selection); the drift
  notice still works (`extentArrivalsSince` counts Resources added since the last run).

## Backend changes

- **B1** `POST /v1/reduction/clusters`: `{id, version, action: check|uncheck, clusterIds?:
  [], allIdentical?: true, expectedCount?}`; exactly one of `clusterIds` or
  `allIdentical`. Unknown or withheld ids refuse the request with the same error as one
  unknown Cluster. Attention Clusters are skipped on `check`. Same CAS and retry path as
  `OverrideReductionCluster`. Returns `{version, changed, needsReview}`. MCP safety class
  `mutating`; OpenAPI route registered.
- **B2** Review query gains `approval`, `sort` and `pageSize` (20/50/100, anything else is
  20); the context gains per-tab counts over visible Clusters. `Largest files first` sorts
  by the Cluster's total member file size, loaded with one `Pluck` over the visible member
  ids (already gathered for the visibility check), ties broken by Cluster id so pages are
  stable across refreshes. `Most copies first` ties likewise.
- **B3** `KeepsSmaller` on the Cluster (plan JSON, no migration), set at compute and by
  every recompute of `Lossy` (`refreshLossy`); part of `clusterNeedsAttention`.
  `promoteMember` sets `DecidedBy = "manual"`, `Margin = ""`; `WinnerCriterionLabel` maps
  `manual`.
- **B4** `plan.Settings` written at compute; settings-differ check in the review context.
- **B5** List context computes per-Reduction counts from each row's plan, for the page of
  rows only (the list is already paginated). Unrestricted principals decode and count; the
  visibility pass runs only for scope-restricted principals, with one `visibleMemberIDs`
  call over the union of member ids of the page's Reductions (not one per Cluster). Perf
  test: a page of 20 Reductions with 5,000 Clusters each renders under a budget measured
  first on master's review page for one such plan, for both an admin and a restricted
  user.
- **B6** Review context adds apply-dialog figures: per-tier approved Clusters and Losers,
  approved KeepsSmaller count.
- **B7** `ApplyResourceReduction` detaches itself: at entry it rebases on
  `ctx.WithParentContext(context.WithTimeout(context.WithoutCancel(ctx.db.Statement.Context),
  ReductionApplyCeiling))` (30 minutes; `WithParentContext`, `context.go`, carries the
  scope filter and acting user; `removeIfUnreferenced` is the `WithoutCancel` precedent).
  Every read and write in the batch, the claim and the per-Cluster loop alike, goes
  through that handle. Not in the handler: the handler's `ResourceReductionContext` has no
  way to rebase the handle. The ceiling bounds a wedged batch; Clusters not reached stay
  approved. Writes `plan.LastApply`.
- **B8** `Extent.All` in create, resolver, `containsResources` and `countExtentResources`.

## Verification

- TDD for the P0 and safety items: KeepsSmaller set/cleared across promote, eject, restore;
  manual reason; bulk check skips attention Clusters; `allIdentical` never touches the Near
  tier; withheld ids refused like unknown ones and absent from every new count (review tabs,
  list counts, dialog figures, `.json`); `expectedCount` and version 409s; cancelled-request
  apply; `LastApply` running/finished/interrupted rendering and second-apply refusal;
  whole-library scope for a subtree-confined user on SQLite and Postgres.
- Go unit + API tests, Postgres run of the reduction tests.
- E2E: create with `Find duplicates now` goes from progress to Clusters; tabs, chips, sort;
  `Approve page` with an attention Cluster skipped; `Approve all Identical`; keyboard
  `j/k/a/s` only inside the Clusters region and off-switch; keeps-smaller warning after a
  bad promote and its fix button; apply dialog figures; result banner; list card progress
  and button; settings preset and changed notice; whole-library create; mobile 390px apply
  bar; page size 100 renders and acts without a perceptible stall (measured in the E2E
  run). Existing `resource-reduction.spec.ts` updated to the new labels.
- a11y spec `09-a11y-resource-reduction.spec.ts` covers not-computed, computing, review
  with bar, apply dialog, shortcuts dialog; axe clean.
- Docs `docs-site/docs/features/resource-reduction.md` rewritten (including the corrected
  keep-as-version paragraph); screenshots retaken.
- Full suites: Go unit, E2E browser + CLI, Postgres.

## Implementation slices

1. Backend B1-B8 with tests.
2. Review page D1, D2, D3, D9.
3. D4, D5, D7, D8, D11.
4. D10 list page, D6 keyboard (first to drop if scope must shrink).
5. Docs, screenshots, full suites, review loop.
