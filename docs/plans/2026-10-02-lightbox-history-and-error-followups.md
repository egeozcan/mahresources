# Lightbox follow-ups after #62: history, MRQL refresh, write errors

Six items left open by PR #62 (Back closes the viewer, visible write errors).
Five are frontend; the sixth is the intermittent Go test
`TestJobsWaitingForAURLHoldNoCapacity`, investigated separately.

Decisions taken with the user (2026-10-02):

- Forward onto the entry a closed viewer left behind **reopens the viewer** on
  the image it was closed on.
- Opening the viewer discarding Forward history is **accepted and documented**.
  It is what every `pushState` does, MRQL's own query runs included.

## 1. Forward lands on a dead history entry

**Cause.** `close()` from inside the viewer calls `history.back()`
(`navigation.js` `_popHistoryEntry`). The marker entry stays ahead of the page
entry, so Forward lands on it, `_onHistoryPop` sees a closed viewer and returns,
and nothing happens. The next Back then needs two presses to leave.

**Fix.**
- `_popHistoryEntry` stamps the current item's id into the marker entry
  (`replaceState({...state, mahLightboxItem: id})`) before it goes back.
- `_onHistoryPop`, with the viewer closed, no close-traversal pending, and
  `history.state.mahLightbox` set: find that item's thumbnail on the page
  (`[data-lightbox-item][data-resource-id=id]`, list container first) and open it
  through `openFromClick` with a synthetic event, so the gallery context (scope,
  source container, standalone) is rebuilt as a click would. `open()` adopts the
  entry it is on instead of pushing (`_historyToken = state.mahLightbox`,
  `_historyReused = false`), so the next Back closes it as usual.
- No id, or no such thumbnail on the page (deleted, filtered out, paged past
  inside the viewer): step back off the entry (`_historyBackPending = true;
  history.back()`), so Forward is skipped rather than left dead.

**Done when.** E2E: open, Escape, Forward reopens the viewer on the same image
with no extra entry (Back closes it, Back again leaves the page); open, step to
the second image, close, Forward reopens on the second image; with the image
removed from the page, Forward leaves the viewer closed and the URL and
`history.length` unchanged and the next Back leaves the page. Unit tests for the
stamp, the adopt path and the bounce.

**Docs.** `navigation.md` and `managing-resources.md` shortcut tables: Forward
reopens the viewer; opening the viewer starts a new Forward history, as any
in-page navigation does.

## 2. MRQL: a save that lands after close clears a bulk selection

**Cause.** A write that lands after `close()` calls `refreshPageContent()`, which
on MRQL runs the selection store's `refresh`, i.e. `execute({snapshot})`.
`execute` → `clearResult` → `resetSelections()`, and the result cards are torn
down, so a selection the reader made after closing is gone.

**Fix.** The selection `refresh` that `connectSelections` installs (used by the
lightbox and by `submitEditorForm`, which deselects before calling it) re-runs
with `preserveSelection: true`. `execute` records each store's selected ids
before `clearResult`, and once the new cards have registered (the existing
`$nextTick` that calls `connectSelections`), re-selects the ids still listed.
A new `restoreSelection(ids)` on the bulk selection store adds ids that have a
registered option and syncs their checkboxes without announcing each one.
Paging, Run, and `refreshForCompletedAction` keep resetting, as today.

**Done when.** E2E on `/mrql`: open a result, rename, close, select two cards
while the save is held, release it; the card shows the new name and both cards
are still selected (bulk bar count 2). Unit test: an id no longer in the result
is not re-selected.

## 3. MRQL: a fast Back-save leaves focus on `<main>`

**Cause.** Back → `close()` blurs Name (the save starts) and schedules its focus
return two frames later. On a fast server the save lands first: the refresh
starts while focus is on `<body>`, so it does not plan a restore, and
`execute()` clears the result immediately, detaching the trigger thumbnail. The
deferred return then finds the trigger detached and parks on the list or `<main>`.

**Fix.** The deferred return in `close()`: if a page refresh is in flight, wait
for it to settle and one frame. Then, unless the reader has moved focus
somewhere real, restore to the trigger if still connected, else to the same
resource's thumbnail in the current list (the lookup `refreshPageContent`
already does, extracted to one helper both use), else the list, else `<main>`.

**Done when.** A unit test drives the failing order deterministically (save
lands, refresh starts, then the deferred return runs) and asserts focus on the
new thumbnail. E2E: a copy of "a Name edit saved by Back keeps focus on the
refreshed thumbnail" with no delay on `editName`; it is timing-dependent, so it
backs the unit test rather than replacing it.

## 4. One error per field: image B's failure replaces image A's

**Cause.** `writeErrors[field]` holds one entry.

**Fix.** `writeErrors[field]` becomes an object keyed by the target resource id.
`_setWriteError` keeps its rules per key (skip if superseded, skip if the key
holds a newer failure). `_clearWriteError` deletes only its key.
`writeError(field)` returns the newest-seq entry whose `targetId` or
`resourceId` is the current image. `close()` resets to empty objects.

**Done when.** Unit: fail on A, fail on B, A's message still shows on A, B's on
B; a later success on A clears only A's. E2E: name save fails on A, navigate,
fails on B, navigate back to A, A's message is visible.

## 5. A tag error has no dismiss control

**Fix.** A "Dismiss" button beside both tag error lines (`#lightbox-tag-write-error`
in the panel, `#lightbox-tag-viewer-error` in the viewer) calls
`dismissWriteError('tags')`, which drops the entries shown on the current image.
Focus moves to the tag search input (panel) or the "Edit tags" button (viewer),
so it does not fall to `<body>` when the button disappears. Accessible name
"Dismiss tag error"; same focus styling as neighbouring controls.

**Done when.** E2E for each location: dismiss hides the message, removes
`aria-describedby` from Repeat/Undo/Edit tags, and focus lands where stated.
axe scan of the open panel with the error shown passes.

## 6. `TestJobsWaitingForAURLHoldNoCapacity` flake

Investigated by a separate agent (read-only). If it is a real double claim, the
fix gets its own failing test and goes in this branch as a separate commit;
otherwise the test is fixed. Decision recorded in the review section below.

## Verification

- Before: `npm run test:unit` (vitest), Go unit, E2E browser + CLI on master.
- After: vitest, `go test --tags 'json1 fts5' ./...`, `cd e2e && npm run
  test:with-server:all`, Postgres (`go test ... postgres` + E2E postgres),
  `npm run build-js` output committed.
- pi review of each batch.

## Review

Plan review (pi, gpt-6.1-sol high) changed the design in five places:

- Item 1: the image is stamped into the entry on open and on every step, not in
  `_popHistoryEntry`. Back leaves the entry before `close()` runs, so a stamp
  there missed Back then Forward.
- Item 2: a refresh that replaces one whose cards have not registered yet
  carries its ids on (`_keptSelection`), and the restore runs only for the
  current request (`_executeRequestId`), so a newer Run or page is never given
  an old selection. `clearResult()` for a new query drops the carried ids.
- Item 3: the focus return is fenced by the closing session, so a viewer
  reopened and closed during the refresh owns focus.
- Item 4 (found on the way): a tag failure clears only once successes have
  covered every tag it names. Saving X after X and Y failed used to hide Y's
  failure. The message text still names both.
- Item 5: the panel's Dismiss falls back to the panel itself, not its Close
  button, when the tag search is absent. #61 moved focus off that button
  because Space, the tagging flow's "next" key, would close the panel.

Known limit: Forward across MRQL queries (`history.go(2)` onto another
query's marker) runs before the destination query renders its cards, finds no
thumbnail, and steps back onto that query's page entry with the viewer
closed.

Item 6 was a real double claim, not a test-timing problem. The dispatch loop
read the pass-over list before selecting the next waiting Job, so a duplicate
that went back to the queue between the two reads was claimed a second time.
No transfer ran twice (the start re-checks the URL under the registry lock).
Fixed by reading the list again after the select (`ClaimRequest.PassOver`),
with the first page kept at one row when nothing is being skipped.
`TestAWaitingDuplicateIsNotClaimedAgainByTheSamePass` forces the interleaving
with a GORM callback: red 3/3 before, green 20/20 after; the original test
passed 600/600 (`-count=200 -cpu 1,2,8`) after the fix.

Code review (pi, three rounds, no P0 or P1 in any): round 1 found that Forward
reopened in the wrong gallery when a resource is listed twice (fixed: the
session's scoped or sourced gallery is searched first) and a data race in the
new Go test; round 2 found that asking `PassOver` per page made a claim
quadratic in the wait list (now asked once a candidate is about to be answered)
and that the race test could pass with the bug restored; round 3 tightened both
tests until a mutation moving `PassOver` before the read fails them. Declined:
the focus return in close()'s first two frames does not check where focus is,
which is master's behavior there.

Red proof for the frontend: all seven new E2E tests failed with master's
`src/` and template and passed with the branch's.
