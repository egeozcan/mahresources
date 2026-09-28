# Sidebar popups: Custom Thumbnail, then Image Actions (2026-09-28)

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
