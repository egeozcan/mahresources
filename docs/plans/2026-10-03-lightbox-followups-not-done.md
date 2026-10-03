# Lightbox follow-ups: the not-done list

What `2026-10-02-lightbox-history-and-error-followups.md` left open, fixed on
master after `e4544be0`.

## 1. A tag error named tags already saved

Adding X and Y failed, then X saved: the message stayed (Y was still unsaved)
but still read "Could not add tags X, Y". The failing write now passes
`describe(tagIds)` to `_setWriteError` (`quickTagPanel.js`), and
`_clearWriteError` (`editPanel.js`) restates the message from the tags still
unsaved when a success narrows it, so it reads "Could not add tag Y".

Test: `staleness.test.ts`, "names only the tags still unsaved once some of them
are saved" (red before, green after).

## 2. close()'s focus return did not check where focus was

`_returnFocus` now leaves focus alone when a painted element other than the
trigger has it and that element did not already have focus when close() ran
(`visibleFocus()` in `utils/focus.js`).

Measured in Chromium: x-trap holds focus inside the viewer through the first
frame after close(), so the reader can move focus onto the page only in the one
frame between the trap letting go and the return. Playwright cannot hit that
frame reliably, so the test is a unit test (`staleness.test.ts`, "focus after
close, reader moved"). The "had focus before close()" exception came from a
full-run flake: a viewer that Forward reopened can leave focus on the previous
thumbnail, and without the exception the guard read that as the reader moving.

## 3. Forward across MRQL queries

`history.go(2)` from query A onto the viewer's entry for query B ran the
viewer's popstate handler first, found none of B's cards, and stepped back off.
MRQL's popstate handler now registers its re-run with `noteListRender`
(`utils/listContainer.js`). `_reopenFromHistory` waits one task for a page
listener to start, waits for a registered render, and looks again. It gives up
if the reader opened the viewer or moved off the entry meanwhile.

Tests: E2E `mrql-lightbox.spec.ts` "Forward from another query reopens the
viewer once that query has rendered" (red before, green after); unit
"waits for a list the traversal renders again before looking for the image".

## 4. The two E2E flakes from the full run

- `job-drawer-live-updates` "moves with its progress frames": the recorded
  failure read 100% as the first value. The six-second transfer had finished
  before the page showed a frame. The test now uses a `/held/` route that
  pauses a sixth of the way in until the test has read its first value.
- `auth/job-accounts` "lets go of a failure another account retried": the
  failure was `loginAs`'s `waitForURL` timing out after 15 s. Two causes fit
  it, and the helper now handles both. It waited for the dashboard's load
  event, which includes every thumbnail; it now waits for
  `domcontentloaded`. A login the server could not judge because the
  database was busy redirects to `/login?error=busy`, which the old URL
  predicate never matched; the helper now retries `busy`/`unavailable` up to
  three times and fails with the error code on anything else.

## Review

pi (gpt-6.1-sol high), round 1: one P1 and four P2, all confirmed and fixed.

- P1: with the Info panel open, Escape let `closeEditPanel()`'s own two-frame
  callback focus the Info toggle. The viewer stays painted through the panel's
  leave transition, so the guard read that as the reader moving, and focus fell
  to `<body>`. Focus inside any dialog no longer counts as a move.
- P2: Forward from a query that also lists the image opened among that query's
  results. `_reopenFromHistory` now always yields one task and waits for a
  registered render before it looks (E2E "Forward from a query that also lists
  the image ...", red before).
- P2: a Run started while Forward waited replaced the awaited render, which
  settled early and bounced the entry. Every `execute()` now registers itself,
  and the viewer waits until no render is current.
- P2: leaving the entry and coming back to it during a render let the first
  waiter bounce the second. Each reopen takes a generation (`_historyReopens`)
  and gives up when a newer one started.
- P2: the progress test could pass with live frames broken, since the lifecycle
  refetch at success also reaches 100%. The download now stops again halfway,
  and the test requires movement below 100% with no refetch.
