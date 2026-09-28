# Custom Thumbnail: one button, and a popup (2026-09-28)

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
