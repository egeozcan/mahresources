---
sidebar_position: 3
---

# Resource Reduction

A **Resource Reduction** is a named, durable workspace for collapsing repeated Resources in batches. You choose what to search, a background job groups the repeats into **Clusters**, you review each Cluster's proposal, and only then is anything merged and deleted.

![Resource Reduction workspace with clusters](/img/reduction-overview.png)

:::warning
Applying a Cluster merges its removed copies into the kept copy and deletes them. A Resource Reduction cannot be undone, and there is no restore path. Review each Cluster before you approve it.
:::

## Vocabulary

This page uses the terms the data model and the API use. The interface uses plainer words for the same things.

| Term | In the interface | Meaning |
|------|------------------|---------|
| Extent | selection | The Resources a Reduction searches |
| Winner | **Keep** | The copy that survives and absorbs the others |
| Loser | **Remove** | A copy that is merged into the Winner and deleted |
| Eject | **Don't remove** | Takes one copy out of the Cluster and leaves it untouched |
| Checked | **Approve** / **Approved** | A Cluster that the next apply will merge |

## Applying does not free disk space

A Reduction tidies the library: one entry per image, with the other copies' tags, notes and groups merged into it. It does not reclaim disk space.

- Files are stored by content hash. A file is removed only when no Resource and no version references that hash on its storage location (`removeIfUnreferenced`).
- When a file is removed, a copy goes to the `/deleted/` backup first (`backUpAndRemove`). The backup is emptied only by whoever runs the server.
- Every version a removed copy already had moves to the kept copy, so those files stay referenced.

The one existing way to release space is to delete versions you no longer need from the kept copy's **Versions** panel. See [Resource Versioning](./versioning.md).

## Starting a Reduction

### Find duplicates in everything

On `/reductions`, when you have no Reductions yet, click **Find duplicates in everything**. It creates a Reduction over everything you can access and starts finding duplicates. The review page opens and shows the progress.

### From the New form

From `/reductions`, click **New**. The form has these fields:

| Field | Effect |
|-------|--------|
| **Name** | Defaults to "Resource Reduction" |
| **Where to look** | **Choose groups and resources**, or **Everything you can access** |
| **Groups**, **Resources** | Shown for **Choose groups and resources**. Groups include the Resources of all their subgroups |
| **Only keep copies from this selection** | Shown for **Choose groups and resources**. Otherwise a better copy elsewhere in your library may be kept instead |
| **Options** | **Matching**, and **Which copy to keep** (see [The Winner Rule](#the-winner-rule)) |
| **Find duplicates now** | Checked by default. Starts the clustering job right after the Reduction is saved |

Click **Create**. With **Find duplicates now** on, the review page opens and shows the progress. With it off, the review page opens with a **Find duplicates** button and nothing is computed.

If the Reduction is saved but the job cannot start, the review page shows "The Reduction was saved, but finding duplicates could not start" with the reason when the browser kept it, and a **Try again** button.

### From a list selection

1. Select Resources on `/resources`, or Groups on `/groups`
2. In the bulk editor, click **Find duplicates**
3. Choose **New Resource Reduction** and give it a name, or **Add to existing reduction** and pick one from the list
4. For a new Reduction, optionally check **Only keep copies from this selection**
5. Leave **Find duplicates now** checked to start the job, or uncheck it
6. Click **Create** (or **Add**)

The panel heading names what you are about to search, for example "Find duplicates in 12 selected Resources". The panel also says "Nothing is removed until you review and apply."

The same button is on a Group's detail page, in the owned Resources panel. It searches that group's Resources. Check **Include resources from all subgroups** to add the Resources of every subgroup.

![Bulk bar with the Find duplicates panel open](/img/reduction-bulk-action.png)

**Only keep copies from this selection** stops matching Resources outside the selection from being kept in place of the copies you chose. It applies to both Identical and Near-Identical matching. The choice is saved at creation and kept when you add selections or recompute. A whole-library Reduction has no such option, because everything you can access is already in the selection.

### The list

Reductions are listed at `/reductions`, most recently updated first. They never expire. Each card shows:

- The name, and a status pill
- A line of counts, for example "140 Clusters · 20 approved · 60 applied · 8 skipped · 2 stale". It shows only the non-zero counts after the Cluster total
- A thin bar for the share of Clusters that are applied or skipped
- Dates created, updated and last computed, and the matching mode
- One button

![Resource Reductions list](/img/reduction-list.png)

| Status | Card text | Button |
|--------|-----------|--------|
| **Not computed** | Not computed yet. Nothing is removed until you review and apply. | **Find duplicates** |
| **Computing** | Finding duplicates… | **View** |
| **Ready** with open Clusters | The counts line | **Continue review** |
| **Ready** with none open | The counts line, or "No duplicates found." | **View** |
| **Failed** | Finding duplicates did not finish. | **Try again** |

While a Reduction made before the plan moved to the current storage is being upgraded in the background, its card reads "Being upgraded to the new storage. Its Clusters appear here when that finishes.", shows no progress bar and has a **Being upgraded** label in place of a button. The review page and the review endpoints answer such a Reduction with `503` ("this Resource Reduction is being upgraded; try again shortly") until the upgrade finishes, which needs no action. A page that was open when this happened shows "This Reduction is being upgraded, try again shortly." instead of refreshing.

**Find duplicates** and **Try again** start the job from the card. The counts are computed from the plan each time the page renders, over the Clusters you can see.

## The selection (Extent)

The set of Resources a Reduction considers is its **Extent**: the Resources you named, plus every Resource owned by or related to the Groups you named and all of their descendants. A whole-library Reduction has every Resource you can access.

Group IDs are stored rather than their expansion, so a Group's contents are resolved fresh every time the Reduction is computed. Files added afterwards enter the Extent on the next **Recompute**. The review page tells you how many Resources were added since the last run.

**About this Reduction**, a collapsed section at the bottom of the review page, lists the selected Groups and Resources by name, the first 20 of each followed by "and N more". Items you cannot see are left out and not counted. A whole-library Reduction reads "Everything you can access."

## How It Works

### Finding duplicates

On the review page, **Find duplicates** (first run) or **Recompute** (later runs) starts the clustering job. It groups the Extent into Clusters, each proposing one Winner to keep and one or more Losers to remove. **Try again** appears after a failure.

While the job runs, the page shows a progress bar and text such as "Comparing… 1,200 of 8,000". The page follows the job through `/v1/jobs/events` and falls back to polling `/reduction.json?id=N` if the stream fails. When the job ends the page reloads. You can leave the page: the server keeps going and the result is there when you come back. A failed job shows its error text.

| Status | Meaning |
|--------|---------|
| **Not computed** | Created, but the Clusters have never been computed |
| **Computing** | A clustering job is in flight |
| **Ready** | The Clusters are computed and waiting for review |
| **Failed** | The last compute failed, or its lease ran out while it was still running |

A compute holds a lease on the Reduction. The running job renews it every 30 seconds and each renewal pushes it 10 minutes ahead, so a large compute is not cut off after any fixed time such as an hour. A run that stops renewing (the process died) loses the lease within 10 minutes and the Reduction reads as failed. No run holds a lease for more than 24 hours from its start: that is the ceiling, and its lease ends there (`leaseCeiling`, `leaseWindow` and `leaseRenewEvery` in `application_context/reduction_store_lease.go`).

While a compute is in flight, the Reduction refuses review decisions, bulk approval, apply, settings changes and changes to the selection with `409` ("this Resource Reduction is still computing"), because the compute will replace the plan (`reductionEditable` in `application_context/reduction_ops_store.go`).

Before the first compute the page shows only the status and one sentence: "Finds copies of the same file and near-identical images in this selection. Nothing changes until you review and apply."

### The Two Tiers

| Tier | What it means | Applies to | Arrives |
|------|---------------|------------|---------|
| **Identical Resources** | The same content hash, and therefore the same bytes | Every content type | Approved |
| **Near-Identical Resources** | Within the perceptual distance threshold and the aHash guard | JPEG, PNG, GIF and WebP only | Not approved |

Identical Clusters arrive approved because byte-identity is certain. Near-Identical Clusters arrive not approved because perceptual matching is a judgement that can be wrong, so the friction sits where the risk is.

A Near-Identical match is a stored pair that passes the same distance predicate and aHash guard used by the similar-resources list and MRQL `SIMILAR TO`. Their effective distance thresholds differ when the live `hash_similarity_threshold` is zero: the list and MRQL honor zero, while Resource Reduction falls back to the configured positive threshold, or 10. The hashes are computed from brightness alone, so two images that differ only in colour but share their pattern of light and dark (the same gradient in green and in blue, a photograph and a recoloured copy of it) can match. Check the thumbnails of each Cluster before approving it.

The **matching mode** chooses which tiers a Reduction looks for:

| Mode | Effect |
|------|--------|
| `both` (default) | Both tiers |
| `identical` | The Identical tier only. This is an index-supported grouping over the content hash, it covers video, PDF and audio, and it is the mode that stays tractable on a very large library |

The two tiers are disjoint. Byte-identical images also produce a distance-zero perceptual pair, so every Resource an Identical Cluster claims leaves the pool before the perceptual tier runs.

A Resource with no content hash is never clustered. It is reported in the coverage line instead, because grouping on the empty hash would collapse every unhashed Resource into a single Cluster.

### How Near-Identical Clusters Are Formed

Candidates are sorted best-first by the Winner Rule and walked. The first unclaimed Resource becomes a Winner and claims every unclaimed Resource within threshold **of itself**.

Perceptual similarity is not transitive, so this is deliberately not a connected-components walk over the pair table. Every proposed deletion rests on a stored pair between that exact Loser and that exact Winner, and no transitive step ever deletes anything.

The cost of that guarantee is fragmentation: a genuine run of near-identicals splits across two Clusters when no single member is within threshold of all the others.

See [Image Similarity Detection](./image-similarity.md) for how perceptual hashes and the distance threshold work.

### Coverage

**About this Reduction** reports how much of the Extent could actually be examined, so "no repeats found" stays distinguishable from "nothing was hashed":

- How many Resources carry a content hash
- How many Resources of a perceptually hashable type carry a perceptual hash

When eligible Resources are unhashed, the page says so and links to **Recompute similarity** on `/admin/overview`.

Coverage measures the **whole** Extent, so it is shown only to an account that can see the whole Extent. A group-limited reviewer is told that instead of being given figures that describe more than they can reach.

## The Winner Rule

Each Cluster has exactly one **Winner**: the Resource that survives and absorbs every **Loser's** tags, notes and group memberships. The Winner Rule is an ordered list of criteria, each breaking the ties the one before it left.

| Criterion | Chooses |
|-----------|---------|
| `pixels_desc` / `pixels_asc` | Highest / lowest resolution |
| `size_desc` / `size_asc` | Largest / smallest file |
| `created_asc` / `created_desc` | Created first / last |
| `updated_asc` / `updated_desc` | Row edited least / most recently |
| `name_asc` / `name_desc` | Name first / last alphabetically |
| `content_type_asc` / `content_type_desc` | Content type first / last alphabetically |
| `has_description` | The copy that carries a description |
| `associations_desc` / `associations_asc` | Most / fewest tags, notes and groups |

The default is **highest resolution, then largest file, then created first** (`pixels_desc`, `size_desc`, `created_asc`). Choose it under **Which copy to keep**, on the New form (under **Options**) or in **Settings** on the review page:

| Preset | Order |
|--------|-------|
| **Best quality** | Highest resolution, then largest file, then added first (the default) |
| **Smallest file** | Smallest file, then highest resolution, then added first |
| **Oldest copy** | Added first, then highest resolution, then largest file |
| **Custom** | Choose criteria in priority order. Leave unused fields empty |

The server stores the ordered list. The form shows whichever preset matches it, or **Custom**. A rule change applies on the next **Recompute**.

A criterion that cannot tell a Cluster's members apart falls through to the next, so a rule mentioning resolution still behaves sensibly for content types that have none. If every criterion ties, the Winner falls to the lowest ID and the Cluster says so rather than presenting a tiebreaker of last resort as a decision.

Each Cluster shows which criterion decided it and by how much (see [The Cluster card](#the-cluster-card)), because "highest resolution, by 4x the pixels" reads differently from "by a hair's breadth in the pixels".

:::note
There is no filesystem modification time anywhere in this system. `updated_at` is when the row was last edited.
:::

## Reviewing Clusters

![Two Clusters under review](/img/reduction-review.png)

The review page is one column, top to bottom:

1. **Skip to Clusters**, a link that appears on keyboard focus
2. The status row: status pill, "Last computed", the **Find duplicates**, **Recompute** or **Try again** button, and a **Settings** link that moves focus to the settings form
3. Notices, only when relevant (see [Notices](#notices))
4. Tabs, filter chips, **Sort**, **Per page** and **Keyboard shortcuts**
5. The Clusters
6. The sticky apply bar
7. **About this Reduction**

### Tabs

Each tab is a link with a count over the Clusters you can see. The counts ignore filters and chips.

| Tab | Shows |
|-----|-------|
| **Open** (default) | Every open Cluster, approved or not, acted on or not |
| **Skipped** | Clusters you skipped |
| **Stale** | Clusters an apply refused (see [Stale Clusters](#stale-clusters)) |
| **Applied** | Clusters already merged. They render as one line and the kept copy |

### Chips, sort and page size

| Control | Options |
|---------|---------|
| Chips | **Identical**, **Near-Identical**, **Approved**, **Not approved**, **Needs attention**, and **Clear filters** while any is on |
| **Sort** | **Most copies first** (default), **Largest files first** (total member file size as recorded when the plan was computed), **Computed order** |
| **Per page** | 20 (default), 50, 100 |

**Needs attention** matches Clusters whose merge would discard fields, Clusters that are oversized, and Clusters that keep a lower-resolution copy. Sort ties break by Cluster id, so pages stay stable across refreshes. Everything is a query parameter on `/reduction?id=N`, so a filtered view is a link you can bookmark.

### The Cluster card

The header reads "Identical · 3 copies" or "Near-Identical · 2 copies", with a state label, **Approve** (a toggle whose label stays **Approve**; a check mark and its pressed state show that it is on) and **Skip**. A skipped Cluster shows **Reopen** instead.

One sentence says why the kept copy was chosen:

- `Keeping "IMG_0412.jpg": highest resolution, 4x the pixels of the next.`
- After you promote a copy: `Keeping "x.jpg": you chose it.`
- When every criterion ties: "All copies tie on every criterion; keeping the one added first."

Each copy shows a 160 pixel thumbnail, a role label and these facts: dimensions, file size, content type, date added, and counts of tags, notes and groups. The kept copy comes first. The role is **Keep**, **Remove**, or **Not included** for a copy you took out of the Cluster. A copy that lives outside the selection carries **Outside the selection**.

On every Remove copy, a fact that differs from the kept copy is bold, starts with a not-equal sign and has the accessible text "differs from the kept copy". A footnote reminds you that "Removed copies' tags, notes and groups move to the kept copy."

On the Near-Identical tier a badge shows how close the match is: **Very close** (perceptual distance 0 to 2), **Close** (3 to 6) or **Similar** (7 and up). The distance is in the badge's accessible text and `title`.

| Action | Effect |
|--------|--------|
| **Keep this instead** | Promotes a copy to Winner. Your judgement beats the rule |
| **Don't remove** | Ejects one copy from the Cluster. The Resource is left completely untouched |
| **Include again** | Restores an ejected copy. Refused on the Near-Identical tier when that Resource has no stored match to the current Winner |
| **Compare** | Opens the two-up comparison of a Remove copy against the Keep copy |
| **Skip** / **Reopen** | Moves past a Cluster without deciding about it, or brings it back |
| **Approve** | Approves or un-approves it for the next apply |

Promoting on the Near-Identical tier re-checks every Loser against the new Winner and ejects any that have no stored pair to it, showing them as **Not included**. Promotion never quietly widens what gets deleted. Promoting sets the reason to "you chose it" and clears the margin.

Two actions are refused:

- **Demoting a Winner that sits outside the selection.** Such a Resource may win, so you can keep the better copy that lives elsewhere, but a Reduction that was not asked to consider it may never merge it away
- **Ejecting the Winner**, which would leave the Cluster with nothing to merge into. Promote another copy first

### Large Clusters

A new Cluster never holds more than 200 copies (`reductionMemberCap` in `application_context/reduction_store.go`); a compute that finds more eligible copies leaves the rest out (see below). Clusters of 12 copies or fewer always render in full, and larger ones share a budget of 2,000 copies per page. A Cluster beyond that budget shows its kept copy and a **Show more members** button until you ask for the rest. The button loads the next copies from `GET /v1/reduction/members` and adds them to the card, focus moves to the first copy added, and a screen reader hears "Showed N more copies." The request carries the Reduction `version` and the `winner` (the kept copy's id) the card was rendered with, and the server answers `409` if either no longer matches, so copies fetched for one render of a card are never added to another. The page also drops a response that arrives after the card was re-rendered, and aborts the request. Every copy in the response carries `inExtent`, `false` included. Only a Cluster stored before the cap existed can hold more than 200 copies: it shows the first 200 and a line "Showing 200 of 340 copies." (it is read-only, see below).

When the compute found more eligible copies than the cap, the card says so and nothing outside the shown copies is merged:

- Identical: "Showing 200 copies; 5 more were left out. Apply this Cluster, then compute again for the rest."
- Near-Identical: "Showing 200 copies; there are more. Apply this Cluster, then compute again for the rest." The card gives no count, because the number of further matches is only a lower bound.

A viewer confined to a Group's subtree never sees the omitted count or this note, because the omitted copies may lie outside their subtree (`Truncated` is zero for them).

A Near-Identical Cluster is a full star when the search for the seed's matches inside the selection reaches 199 copies (`insiderCap` in `buildStar`, `application_context/reduction_compute_near_store.go`). The other copies were each matched to one copy, the seed, and the Cluster cannot grow past it: no copy outside the selection may join, and the seed is the kept copy for good. The count is of matches inside the selection, not the size of the Cluster. A seed with 198 matches inside the selection and a better copy outside it is not full, because that copy may win. Its justification reads `Keeping "name" (kept: the copy the others were matched to).` The kept copy is fixed. **Keep this instead** and **Keep the larger copy instead** are disabled, with the note "This Cluster is at the size limit; its kept copy is fixed, so another copy cannot be kept instead." The server refuses the same promotion with `409` ("this Cluster is at the size limit; its kept copy is fixed").

A Cluster stored before the cap existed and holding more than 200 copies is read-only. If it was already applied, it stays on the Applied tab as history. Otherwise it is stale with the reason "too large: compute again", and its card says "This Cluster is read-only" with no actions at all. Any change to it is refused with `409` ("this Cluster is too large to act on; compute again"). Compute again to review those copies in Clusters of up to 200.

### Keeping a lower-resolution copy

When the kept copy has fewer pixels (width times height) than a copy being removed, the card shows a note: "The kept copy has a lower resolution than a copy being removed (800×533 vs 1600×1066)." The dimensions are the kept copy and the largest copy being removed. The **Keep the larger copy instead** button promotes that largest copy.

The check compares pixels only. A smaller file at the same pixels is often the better copy, and comparing size would flag every Cluster under the **Smallest file** preset. The note is suppressed when the rule's first criterion is `pixels_asc`, because you asked for the smaller copy. It is not suppressed after you promote a copy by hand, because a mis-click is the case it exists for.

A Cluster with this note needs attention, so bulk approval skips it. The apply dialog also counts these Clusters.

### Frozen Clusters

A Cluster you have explicitly acted on **freezes**. Its members are held out of re-clustering, so growing the selection and recomputing can add Clusters but can never rearrange a judgement you already made. Approving through a bulk action counts as acting on a Cluster, as a single approval does.

A Cluster that merely arrived approved by default has not been acted on.

### Oversized Clusters

An unusually large Near-Identical Cluster arrives not approved whatever the tier default says. Its **Approve** and **Skip** buttons stay disabled until you click **Show N copies**. Approving it carries an explicit acknowledgement that the server requires, so the guard is not only in the browser.

A large Identical Cluster is not treated as suspicious: fifty copies of one file are fifty copies of one file.

### Persistence

Clusters paginate; members within a Cluster load in pages of up to 200 (see Large Clusters). Every decision is stored with the Reduction's plan, so it survives paging, a reload, closing the tab and a server restart.

### Notices

These appear above the tabs when they apply:

| Notice | When |
|--------|------|
| "N Resources added since the last run. Recompute to include them." | Resources entered the selection after the last compute |
| "Settings changed since the last run. Recompute to apply them to Clusters you have not acted on." with **Recompute** | The matching mode, the Winner Rule or **Only keep copies from this selection** differs from what the last compute used. Keep-as-version flags are not compared, because they act at apply. A plan computed before this notice existed shows nothing |
| "N resources are missing a perceptual hash, so Near-Identical results may be incomplete." with a **Recompute similarity** link | Coverage is poor and the matching mode includes the Near-Identical tier |

## Approving in bulk

A sticky bar at the bottom of the review page shows "18 approved · 34 copies to remove", the bulk buttons and **Apply 18 Clusters…** (disabled at 0), with the line "Applying cannot be undone." The counts are for the whole Reduction, not only the page. Below 640 pixels wide the bulk buttons move into a **More** menu.

| Button | Scope | Notes |
|--------|-------|-------|
| **Approve page** | The open Clusters on the page, any tier | On the **Open** tab only |
| **Clear page** | The same Clusters | On the **Open** tab only. Never skips |
| **Approve all Identical (n)** | Every open Identical Cluster in the whole Reduction | Never touches the Near-Identical tier. The disabled state means n is 0 |

Rules, enforced by the server:

- **Attention Clusters are skipped.** Approving never touches a Cluster that is lossy, oversized or keeps a lower-resolution copy. The bar reports it: "Approved 16. 2 need individual review." with a **Review them** link to the **Needs attention** filter. You can still approve such a Cluster one at a time (an oversized one needs expanding first)
- **Approve all Identical** sends the number on the button as `expectedCount`. If the real number has moved, the request is refused with `409` and the page refreshes the number
- Every Cluster a bulk approval touches is marked reviewed, so it freezes
- Nothing is written if any named Cluster is unknown, not visible to you or not open

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| `j` / `k` | Focus the next / previous Cluster |
| `a` | Approve or un-approve the focused Cluster |
| `s` | Skip it, or reopen a skipped one |
| `x` | Show the copies of an oversized Cluster |
| `?` | Open the **Keyboard shortcuts** dialog |

Shortcuts work only while focus is inside the Clusters region. They stand down for a modifier key, a form field, an open dialog and an open media viewer. The **Keyboard shortcuts** button opens the dialog. Its **Turn off keyboard shortcuts** checkbox switches them off, and the choice is kept in the browser's local storage.

## Settings

**Settings** is in the sidebar of the review page. It saves without reloading the page.

| Setting | Options |
|---------|---------|
| **Name** | Free text |
| **Matching mode** | **Identical and Near-Identical Resources** (`both`), or **Identical Resources only** (`identical`) |
| **Which copy to keep** | A preset, or **Custom** |
| **Add removed copies as versions of the kept copy** | Two checkboxes: **For Near-Identical Clusters** and **For Identical Clusters** |

Changing a setting does not recluster anything. Click **Recompute** to apply matching mode and Winner Rule changes to Clusters you have not acted on. Keep-as-version flags take effect at the next apply.

The **matching mode** chooses which tiers a Reduction looks for. `identical` is an index-supported grouping over the content hash, it covers video, PDF and audio, and it stays tractable on a very large library.

## Applying

Click **Apply N Clusters…** in the bar to open the dialog.

:::warning
Applying is permanent. Verify that each approved Cluster's kept copy is the one you intend to keep.
:::

The dialog shows figures built from the server's counts:

- Per tier: "Identical: 12 Clusters, 15 copies removed." or "Near-Identical: 6 Clusters, 8 copies removed."
- What happens to the removed copies' files. With the tier's keep-as-version flag on: "Their files are added as versions of the kept copy." With it off: "Their files move to the deleted-files backup unless a version still uses them." The Identical line adds "They have the same content as the kept copy."
- When non-zero: "2 approved Clusters keep a lower-resolution copy than one they remove." with a **Review them** link
- "This does not free disk space. To release space, delete versions you no longer need from the kept copy's Versions panel. The deleted-files backup is emptied only by whoever runs the server."
- "There is no undo."

Click **Cancel** to close it, or **Apply N Clusters** to run. Clicking outside the dialog does not dismiss it.

Applying is partial and repeatable. Unapproved Clusters stay open, and applied ones move to the **Applied** tab. A Winner that survived an applied Cluster goes back into the pool as an ordinary candidate, so a duplicate arriving next month can still be caught.

### While it runs

Apply is one request. The dialog switches to "Applying N Clusters…" and "You can leave this page; the server finishes the batch." The batch runs on a context detached from the request, with a 30 minute ceiling (`ReductionApplyCeiling`), so a closed tab or a dropped connection cannot cancel a merge halfway. Clusters the batch did not reach before the ceiling stay approved.

You may eject a member while a batch runs and it will be spared. A Cluster that was recomputed, or whose Winner was promoted, after you pressed **Apply** is refused rather than merged, because it is no longer the proposal you approved. A second apply on the same version loses the claim and is refused with `409`.

### The result

When the response arrives, a banner at the top reads "Applied N Cluster(s), removed N cop(ies)." and "This does not free disk space." The numbers come from the response's totals: `appliedCount`, `staleCount` and `destroyed`. The `applied` and `stale` lists in the response stop at a cap, and `more` is `true` when they did; the banner then adds "This report names only some Clusters. The full list is in the Applied and Stale tabs." If any Cluster could not be applied, it adds a link to the **Stale** tab and lists each listed one by its kept copy's name (the entry's `winnerName`, cut to a bounded length) with its reason. The page does not fetch the Resources to name them. A Cluster refused because a copy it would remove left the selection after the compute reads "left the selection". Approved Clusters that another decision moved before the batch reached them (unchecked, skipped or changed in another tab) are not stale; they are counted as changed (`changed` in the response) and are not on the Stale tab. A Cluster the batch could not even read because of a database failure is counted in `errors` and stays approved.

### The last apply record

The Reduction stores a record of its last apply: when it started and finished, how many Clusters were applied, how many copies were removed and how many went stale. The page shows it, so a lost response is answered on reload.

| State | Message |
|-------|---------|
| Finished | "Last apply, 2026-11-23 16:40: applied 18 Clusters and removed 34 copies." plus "N could not be applied; they are on the Stale tab." when there are any |
| Running | "An apply started at 16:40 is running. Reload to see the result." **Apply** is disabled |
| Interrupted | "The last apply was interrupted. Applied Clusters are on the Applied tab; the rest are still approved." Shown when the record has no finish time and the apply lease has run out |

An apply holds a lease on the Reduction, renewed every 30 seconds and pushed 10 minutes ahead each time, including while it settles claims an earlier apply left unfinished. A batch whose server stopped therefore reads as running for up to 10 minutes after its last renewal, and as interrupted once the lease has run out. A batch that loses its lease before it starts merging records no finish time and is shown as interrupted, instead of as a finished apply that applied nothing.

A Cluster withheld from you is left untouched by your apply and does not appear in its result, so you cannot learn how many existed. The record names its apply with a batch id, and each Cluster that apply decided carries that id. The counts (applied, removed and stale) are shown only when exactly as many Clusters carry the id as the counts add up to (applied plus stale) and none of them is withheld from the viewer. An account whose access has since narrowed therefore loses the numbers once it can no longer see one of those Clusters, and the record shows its times and state without them (`countsHidden`). An administrator loses them too when the Clusters cannot be accounted for: a record that counted Clusters but holds no batch id (an apply recorded before the ids existed, which a migrated Reduction can carry, even when it recorded merges), or one whose id is carried by a different number of Clusters than the counts. A record that counted nothing and holds no batch id has nothing to hide.

The apply request can carry `applyToken`, the digest the review page exposes over the exact approved proposals (each Cluster id, Winner, Losers and their reviewed hashes). When it is sent and the proposals visible at apply time differ in any way, the apply is refused with 409 and nothing is claimed. It can also carry `requestId`, an opaque id of up to 64 characters that is stored in the record so a client that lost the response can tell its own apply from another tab's.

A Cluster an apply has claimed but not yet merged is shown as **Merging**, not Applied: its copies still exist. The merge and the mark that it happened commit in one database transaction, so a Cluster that is claimed and not marked merged was not merged. Each claim carries its own token, and the merge refuses to run unless the claim it started under is still the Cluster's current one. If the process dies in between, the next apply settles the Cluster (viewing the page never does): all its copies still there means it goes back to open and approved, and a copy gone means it becomes stale, because a Resource in it no longer exists. The review page shows **Finish the interrupted apply** whenever it can see a stranded claim that nothing is running on; it sends `settleOnly: true` to the apply route, which settles those Clusters and merges nothing else.

An applied Cluster stays visible to an account confined to a Group subtree only while that account could see everything in it. The removed copies no longer exist, so the merge records which Group owned each one. The Cluster is shown only if the kept copy is visible now and every recorded owner is inside the account's current subtree. A Cluster applied before this record existed is hidden from confined accounts and stays visible to unrestricted ones. So an administrator's apply never reveals to a confined owner a Cluster that reached outside their subtree.

### Recovering a stale Cluster

Recompute leaves a stale Cluster as it is, so it does not rebuild it. The card marks the copy whose bytes changed, and says when it is the kept copy itself. Use **Don't remove** on a marked copy, or one that was removed, to reopen the Cluster with the rest, then approve it again. If the kept copy itself changed or was deleted, start a new Reduction over the same selection.

### Stale Clusters

Immediately before each merge, the Cluster is revalidated. It is refused, marked **stale**, named in the result and kept in the Reduction when:

- A member no longer exists. For an account confined to a Group subtree, a member that is gone and one that is out of its reach look the same, so such a Cluster is withheld instead: it is left untouched and not reported
- A member's content hash no longer matches what the plan recorded. A version upload changes the bytes without touching the similarity pairs, which is why the plan snapshots each member's hash
- On the Near-Identical tier, the stored pair between the Winner and a Loser no longer holds

An approved Cluster that another decision moved before the batch reached it (unchecked, skipped, ejected from, or recomputed) is not stale. It is counted as changed in the result and does not appear on the Stale tab.

One stale Cluster never refuses the whole batch. Ejected members are exempt from all of these checks, because an ejected Resource is not part of the Cluster and is left untouched.

The checks that matter run **inside the transaction that performs the deletion**, behind a row lock, not merely before it.

### Keep as Version

`keepAsVersion` is two flags, one per tier. They are the checkboxes under **Add removed copies as versions of the kept copy**.

| Tier | Default | Effect when on |
|------|---------|----------------|
| **Near-Identical** | On | Each Loser's file becomes a further version of the Winner, so there is a way back to pixels you decided against |
| **Identical** | Off | A byte-identical Loser has nothing to preserve |

With a flag off, the Loser's file is not added as a version. What happens to it depends on references:

- A Resource that was never versioned has only a virtual v1 (`GetVersions` builds it from the Resource row), not a stored version row. If nothing else references its file, the file goes to the `/deleted/` backup and is removed from its location
- If another version or another Resource still references the same hash on that storage location, the file stays where it is (`removeIfUnreferenced`)
- Versions the Loser already had always move to the Winner, whatever the flag says

Either way the bytes remain on disk, in a version or in the backup. See [Applying does not free disk space](#applying-does-not-free-disk-space).

## Visibility

A Reduction is visible to the person who created it and to administrators. A Reduction whose owner has been deleted belongs to nobody, because a pending destructive decision is not a shared artifact.

Membership is re-checked against your current access both when the page renders and again when you apply, because a scope-limited account's subtree can change after a Reduction is created.

A Cluster reaching a Resource you may not see is not shown, not counted, not named in an apply result, and answers every action exactly as a Cluster that does not exist, including on the `.json` form of the page. Its identifier is derived from its members' IDs, so anything that confirmed such a Cluster existed would give those IDs away.

## Limitations

### Stale Perceptual Pairs

Perceptual pairs are precomputed and are not removed when a Resource's file changes. A version upload deletes that Resource's perceptual hash and re-queues the work, but the pairs it took part in survive, describing content it no longer holds.

A Reduction refuses to cluster on a pair whose endpoint has no current perceptual hash, which covers the window before the hash worker catches up. Once it has re-hashed, the old pairs are still there.

The plan snapshot means an apply never deletes bytes that changed **since** you reviewed them, and looking at the images is what catches a match that was never true.

### Not Supported

- **Undo or restore.** There is no restore path in this system, and this feature does not build one
- **A CLI surface.** A Reduction's value is the person looking at Clusters before deletion. [`mr resources merge`](../cli/resources/merge.md) already serves scripted merging by someone who knows their Winner
- **Perceptual matching for video, PDF or audio.** Those content types are reachable through the Identical tier only
- **Automatic recompute.** Re-scanning is always something you ask for

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/reductions` | List the Reductions the caller may see (`name`, `status`, `sortBy`, `page`) |
| `POST` | `/v1/reduction` | Create a Reduction, or add a selection to an existing one (`id`, `name`, `resourceIds`, `groupIds`, `ownerId`, `includeDescendants`, `all`). New Reductions also accept `excludeExternalResources` (default `false`), `matchingMode`, `winnerRule` and the keep-as-version flags. Responds with `id`, `version` and `url` |
| `POST` | `/v1/reduction/edit` | Rename, or change the matching mode, Winner Rule or keep-as-version flags |
| `POST` | `/v1/reduction/delete` | Delete a Reduction. The Resources it named are untouched |
| `POST` | `/v1/reduction/compute` | Start the background clustering job. Refused with `409` while a run is in flight |
| `POST` | `/v1/reduction/cluster` | Record one review decision (`clusterId`, `action`, `resourceId`) |
| `POST` | `/v1/reduction/clusters` | Approve or clear many Clusters at once (see below) |
| `GET` | `/v1/reduction/members` | The next copies of a Cluster the review page cut short (`id`, `cluster`, `after`). Responds with `members` and `after`, the cursor for the next call (empty at the end). Start with no `after` |
| `POST` | `/v1/reduction/apply` | Merge and delete every approved Cluster's Losers. Responds with `applied`, `stale`, `changed`, `errors`, `appliedCount`, `staleCount`, `destroyed` and `more` |

Every write carries a `version` field: the optimistic-concurrency counter the caller last saw. A request carrying a stale one is refused with **409 Conflict** rather than merged, because merging two judgements about which files to delete cannot be done safely. Read the current value from the Reduction before each write. Create, edit and bulk responses carry the new `version`.

### Request caps

| Request | Cap | Over the cap |
|---------|-----|--------------|
| `clusterIds` in a bulk approval | 500 Clusters (`reductionBulkIDCap`) | `400` |
| `resourceIds` or `groupIds` when creating or extending a Reduction | 10,000 each (`reductionSelectionCap`) | `400` |
| Resource ids in one bulk delete (`POST /v1/resources/delete`) | 10,000 (`MaxBulkDeleteResources`) | `400` |

### Create: whole library

`all` (boolean, JSON or form `all=true`) creates a whole-library Reduction. `resourceIds` and `groupIds` may then be empty. With `id` set, `all=true` widens an existing Reduction to the whole library. Without `all`, an empty selection is `400`.

### Single decisions

`POST /v1/reduction/cluster` accepts these `action` values: `promote`, `eject`, `restore`, `skip`, `reopen`, `check` and `uncheck`. Approving an unusually large Near-Identical Cluster additionally requires `acknowledgeOversized`. The review page also sends `membersSeen`, the number of copies it had rendered: when the field is sent, `0` included, and its value is not equal to the copy count of an unusually large Cluster, the approval is refused with `400`. Leaving the field out lets the acknowledgement alone stand. The page also sends `winnerId`, the kept copy it showed; when it is sent and the Cluster keeps a different copy, the approval is refused with `409`.

### Bulk approval

`POST /v1/reduction/clusters` takes JSON or form fields:

| Field | Type | Notes |
|-------|------|-------|
| `id` | number | The Reduction |
| `version` | number | The version the caller last saw |
| `action` | string | `check` or `uncheck` |
| `clusterIds` | string array | Cluster ids, any tier. Give exactly one of `clusterIds` or `allIdentical` |
| `allIdentical` | boolean | Every open Identical Cluster in the whole Reduction. Never the Near-Identical tier |
| `expectedCount` | number | Required with `allIdentical`: the count the caller saw |

The response is `{"id", "version", "changed", "needsReview"}`: the new version, how many Clusters changed, and how many a `check` left alone because they need individual review. Every other targeted Cluster is marked reviewed. `uncheck` never skips.

Errors:

- `400`: neither or both of `clusterIds` and `allIdentical`, more than 500 `clusterIds`, a bad `action`, or `allIdentical` without `expectedCount`
- `409`: a stale `version`, an `expectedCount` mismatch, a named Cluster that is not open, or a Reduction that is still computing

An unknown id, or one you may not fully see, refuses the whole request with the same error as one unknown Cluster. Nothing is written on any error.

### Review page parameters

`GET /reduction?id=N` (and its `.json` twin) accepts these query parameters. Unknown values fall back to the default and never error.

| Parameter | Values | Default |
|-----------|--------|---------|
| `status` (repeatable) | `open`, `reviewed`, `skipped`, `stale`, `applied`, `all` | `open` |
| `tier` (repeatable) | `identical`, `near` | none |
| `attention` | `true` | off |
| `approval` | `approved`, `unapproved` | none |
| `sort` | `copies`, `size`, `computed` | `copies` |
| `pageSize` | `20`, `50`, `100` | `20` |
| `page` | 1-based | 1 |

`status=open` covers every open Cluster, whether you acted on it or not. `reviewed` still works and means open Clusters you acted on. `status=all` applies no state filter. A Cluster you may not fully see is left out of every view, `status=all` included, in the page and in its `.json` twin: no stub, no count, no position.

## Related Pages

- [Image Similarity Detection](./image-similarity.md) -- perceptual hashing, distance thresholds, and the pair table the Near-Identical tier reads
- [Resource Versioning](./versioning.md) -- how keep-as-version stores a Loser's file on the Winner
- [Bulk Operations](../user-guide/bulk-operations.md) -- the bulk bar a Reduction is started from
- [Authentication & RBAC](./authentication.md) -- group scoping, and what a confined reviewer can see
