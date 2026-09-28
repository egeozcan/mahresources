# Sidebar popups: Custom Thumbnail, Image Actions, Video Actions (2026-09-28)

## Goal

The **Custom Thumbnail** sidebar group on the resource detail page becomes a single
button that opens a popup. **Upload Image** and **Regenerate from Source** live
inside the popup. An image pasted anywhere on the page is no longer accepted; a
paste is only taken while the popup is open.

## Why

- The group was three controls and a paragraph of help text for two actions,
  competing with the preview and the rest of the sidebar for space.
- `@paste.window` was live in every state of the page, so an image pasted anywhere
  on the resource page silently overwrote the thumbnail with nothing on screen
  saying so. A page-wide side effect needs a visible owner.

## What changed

- `src/components/customThumbnail.js` — `isOpen`, `open()` / `close()`, the opener
  captured at open and focus restored on close (`src/utils/focus.js`), and the
  shared `blockingModal` guard so a second `aria-modal` dialog is never mounted.
  The paste moved to a **capture-phase** `window` listener that acts only while
  `isOpen` and stops the event when it takes it.
- `templates/displayResource.tpl` — the group is a `Custom Thumbnail…` button plus
  an `x-if` popup on the app's existing `plugin-action-overlay` /
  `plugin-action-modal` + `x-trap` pattern (`pluginActionModal.tpl`,
  `massEditModal.tpl`).
- `src/components/customThumbnail.test.ts` — the popup and the paste rules.
- `e2e/tests/custom-thumbnail.spec.ts` — opens the popup; a closed popup ignores a
  paste, an open one takes it.
- `docs-site/docs/{user-guide/managing-resources,features/thumbnail-generation}.md`.
- `public/dist` rebuilt.

## The defect this found on the way

The paste path uploaded the image **twice**.

`@paste.window` and the global `setupPasteListener()` are both `window` paste
listeners. On a resource page, in order:

1. `onPaste` → `upload(file)`.
2. `setupPasteListener` guard 2 — the page has a file input and the clipboard has
   files — merges the clipboard into **our** hidden input and dispatches `change`,
   so `onFileChosen` → `upload(file)` again.

Which one runs first is decided by registration order (Alpine binds the directive
inside `Alpine.start()`, which runs before `setupPasteListener()`), which is not a
reason to leave the double upload in place. The popup now owns its paste
exclusively: a capture-phase listener reaches `window` before any bubble-phase
listener on it, and `stopImmediatePropagation` ends the event there. A clipboard
with no image is left alone entirely, stop propagation included.

The same reasoning explains why gating the handler alone would not have been
enough. With the file input moved inside the `x-if`, a paste while the popup is
*closed* no longer matches guard 2 — there is no file input on the page — so the
global handler falls through to its own toast ("paste from a group or note detail
page"), which is the honest answer for a page that does not take pastes.

## Verification

- `npx vitest run src/components/customThumbnail.test.ts` — 10 passed. Removing
  the `isOpen` guard fails `is ignored while the popup is closed`; removing the
  `blockingModal` guard fails `declines to open on top of another dialog`.
- `e2e`: `tests/custom-thumbnail.spec.ts` — 4 passed.
- `npm run test:unit` — 1692 passed. `go test --tags 'json1 fts5' ./...` — clean.

## Notes / limits

- Closing mid-request is allowed. `refreshPreviewImages` is module-level, so the
  preview still refreshes under the reader; what is lost by dismissing early is
  the status line, not the image.
- The outcome stays in the popup after a success rather than the popup closing
  under the reader, which is what `cropModal.tpl` does.

## Follow-up: Image Actions (same day)

The same shape was then applied to the other three image operations, which had
been a sidebar group of three section headings and three buttons
(**Update Dimensions** / **Recalculate Dimensions**, **Rotate 90 Degrees** /
**Rotate**, **Crop** / **Crop…**). It is now one **Image Actions…** button and a
popup holding **Recalculate Dimensions**, **Rotate 90°** and **Crop…**, each with
a one-line statement of what it does — every line taken from the user guide,
since the popup drops the three headings that used to carry the distinction.

Two things this surfaced:

- **The shared rules were about to be written twice.** `customThumbnail` had
  open/close with the opener capture, the `modality.js` guard and the deferred
  focus restore. Those three are easy to get subtly wrong and impossible to
  notice when wrong, so they are now `src/components/sidebarPopup.js` and both
  components spread it; `customThumbnail` overrides the `onOpen` hook to drop the
  previous run's outcome. `open()` also now takes the opener from
  `captureTrigger($event)` rather than from focus, which a mouse press does not
  always set.
- **Crop is a second dialog, and it is a native one.** It lives outside the
  popup, so `modality.js` cannot be the mechanism that keeps the two apart — the
  popup is closed and the crop dialog opened, a tick apart, because `x-trap` is
  still armed at the end of that call and `showModal()` over an armed trap pulls
  focus back into the popup behind it. The hand-off focuses the Image Actions
  button first, so `<dialog>`'s own close-restore has somewhere real to return
  to: the Crop… button it would otherwise restore to is inside the popup that
  has just been removed.

Moving the Crop… button inside an `x-if` also broke every spec that clicked
`#crop-open-<id>` — six call sites, and the failure reads as a timeout rather
than as the change that caused it. They go through
`e2e/helpers/image-actions.ts::openCropDialog` now.

### Verification (follow-up)

- `src/components/imageActions.test.ts` — 10 tests. Removing the `focusOn`
  before `showModal` fails two of them.
- `e2e/tests/accessibility/18-a11y-image-actions-modal.spec.ts` — 6 passed:
  closed sidebar shows one button, axe over the open popup, Escape + focus
  return, the trap in both directions, the hand-off not stacking two dialogs,
  and the crop dialog handing focus back to the Image Actions button.
- `resource-crop` + `crop-zero-dims-banner` + `16-a11y-crop-modal` — 11 passed.
- `tests/accessibility/` + `tests/lightbox/` + `custom-thumbnail` — 352 passed.
  `npm run test:unit` 1869 · `go test --tags 'json1 fts5' ./...` clean ·
  `./scripts/css-scan-test.sh` clean.
- Measured the two sidebar buttons at 1400/1100/900/768/500/390px: no clipping
  and no overflow at any of them, so the `…` in the label is the ellipsis
  character and not truncation.

## Follow-up: Video Actions (same day)

Trim Video got the same treatment — one **Video Actions…** button, a popup
holding the whole trimmer. The third caller needed no new behaviour, which is
what the `sidebarPopup.js` extraction bought: the diff in `videoTrimmer.js` is
one spread, and the open/close/focus/modality coverage transferred from the other
two components unchanged.

Two things decided the shape:

- **The `x-data` root stays on the always-mounted sidebar group; only the UI is
  behind the `x-if`.** `submit()` posts and then reloads, so a component that
  lived and died with the dialog would orphan an in-flight request, and the
  times the reader had dialled in would be gone on reopen. `data-trim-section`
  is unchanged for the same reason. The slider's `x-ref` is read at pointer
  time rather than at init, so it does not mind the element being absent while
  the popup is shut. An e2e case pins the state surviving a close/reopen.
- **The popup is 480px and the trimmer is a form with a slider in it.** Checked
  by forcing a duration and screenshotting: the track, both thumbs, the selected
  range and the time labels all fit.

`e2e/helpers/image-actions.ts` became `e2e/helpers/sidebar-popups.ts` with an
`openVideoActions` and an `openCustomThumbnail` beside `openImageActions` — the
same shape three times over, so one file is where the next one goes.

**Observed, not changed:** the trimmer's `validationError` and `errorMessage`
are `x-show` paragraphs carrying `role="alert"` — a live region that is
`display:none` until its text is set, which is the unreliably-announced shape
`cropModal.tpl` already works around. Pre-existing on this markup and out of
scope for a move; worth a separate pass.

### Verification (video actions)

- `e2e/tests/accessibility/19-a11y-video-actions-modal.spec.ts` — 6 passed:
  closed sidebar shows one button, axe over the open popup, the slider and all
  three inputs inside the dialog, Escape + focus return, the trap both ways,
  and state surviving a close/reopen.
- `resource-trim` — 3 passed (the two that drive the form now open the popup).
- `tests/accessibility/` + `tests/lightbox/` + `custom-thumbnail` +
  `entities/resource` — 365 passed. `npm run test:unit` 1869 · `go test` clean ·
  `./scripts/css-scan-test.sh` clean.

## Follow-up: a preview in the trim dialog, and the slider that was never really there (same day)

**"The slider is gone" was half right, and the half that was wrong is the
interesting half.** The markup was untouched and still in the popup, gated on
`x-show="duration > 0"`. But `duration` comes from `ProbeVideoDuration`, which
requires a local filesystem ("video duration probing requires a local
filesystem") and a working ffprobe. So it is 0 for every memory-fs deployment —
which is every `-ephemeral` run, including the whole e2e harness and every demo —
and for any non-local storage location. The slider silently did not render, and
`End (s)` sat empty with "End time must be a positive number" on open. That is
what was being looked at, and it predates the popups.

**The preview is the fix, because the media element knows its own duration.**
`loadedmetadata` on a `<video>` gives the same number ffprobe would (verified:
both report 6.083333 for `sample-video.mp4`) for free, on any filesystem, with no
server round trip. So the duration is adopted from the element when the server
could not supply it, and the slider appears — in the popup, on the demo, and in
e2e. A range already dialled in is never clobbered: only an untouched `end` is
completed from what we now know.

**What "preview" means here.** A `<video controls preload="metadata">` over the
resource's own bytes (`/v1/resource/view`, which 302s to the static file server
and answers `Range`, so seeking works), plus a **Preview Range** button that
plays from `start` and stops at `end`. A preview that stops where the trim will
is the one question a trimmer cannot answer by looking at it. The native
controls stay, so free scrubbing is still available and the media element is one
tab stop rather than a wall of custom controls in a 480px dialog.

**The preview follows the selection, and yields to the reader.** Moving a handle
or typing a time seeks the player to the new `start` — but only while it is
paused, and only on drag *end* rather than on every `pointermove`, which would
queue a seek per mouse event. While the reader is watching something, the
preview is left alone; a preview that fights the person using it is worse than
one that lags. The stop-at-`end` is gated on the same flag, so watching the whole
video by hand is never cut short at the trim point.

### Verification

- `src/components/videoTrimmer.test.ts` (new) — duration adoption, the range
  stopping where the trim will, the paused-only auto-seek, and that a range
  already dialled in survives the metadata arriving late.
- `19-a11y-video-actions-modal.spec.ts` — the slider is there (two thumbs, a
  known duration), the preview plays and stops at the end, and the video is one
  tab stop inside the dialog rather than a trap problem.
- Confirmed before building: Playwright's Chromium here reports
  `canPlayType('video/mp4; codecs="avc1.42E01E") === "probably"`, `loadedmetadata`
  fires with duration 6.083333, and `currentTime = 3` produces a `seeked` event —
  so this is e2e-testable here rather than only in theory.

### Two defects the work turned up, both from the slider becoming visible

1. **`adoptMetadata` seeded `end` and left `start` null.** Nothing about that
   looks wrong on screen — `6.08 > null` coerces, so the button enabled, the
   thumb sat at 0% and the label read `0.0s` — but `role="slider"` with no
   `aria-valuenow` is a critical `aria-required-attr` violation, and axe had
   never reported it because the slider had never rendered anywhere it could
   look. Both ends are seeded now, and the unit test says why.

2. **`hasTimes()` preferred the slider state while `submit()` posts the text
   fields.** Clearing End left a live button under "End time must be a positive
   number", and preferring a known duration turned that from a rare state into
   the normal one — it is what the existing `resource-trim` "invalid times"
   case was actually reporting. The text fields win now, because the slider is a
   view of them (`syncFromSlider` writes them, `syncFromText` syncs back) and
   they are what the request carries.

The visible control is what makes its neighbours testable. Neither of these was
reachable in any test environment before, which is exactly why neither had been
found.

## Follow-up: Set Start / Set End off the playhead (same day)

Two buttons beside the preview that mark the range from wherever the playhead is,
which is the way you actually mark a cut: play it, mark it, keep going.

They write through `startText` / `endText` and `syncFromText()`, not by
assigning `this.start`, so the ordering rule and the clamping stay one
implementation each and the text fields — which `submit()` posts — cannot drift
from the slider that mirrors them. Marking a start past the end pushes the end
along, exactly as typing one does; the end of a video is clamped, because it is
not a place a trim can cut at.

**One defect only e2e could see.** `canMarkTime` read `$refs.player.duration`,
and `$refs` is not reactive: Alpine evaluated `:disabled="!canMarkTime"` on the
first render and never again, so a dialog opened before the metadata landed kept
the buttons disabled forever. The unit test called the getter directly and passed
the whole time. It now reads `this.duration`, which `adoptMetadata` writes, so it
recomputes exactly when it should — and the e2e that clicks the buttons is what
pins it.
