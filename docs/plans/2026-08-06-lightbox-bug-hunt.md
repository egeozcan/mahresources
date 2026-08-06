# Lightbox bug hunt, 2026-08-06

Reported symptom: "sometimes the lightbox loses the image after it loads new items, and the only way
to restore it is to close and reopen it. It happens sporadically."

Method: six independent finder passes over `src/components/lightbox*`, `templates/partials/lightbox.tpl`
and every external caller, then one adversarial verifier per finding (refute-by-default). 35 raw
findings, 34 after dedupe, 24 confirmed, 8 refuted, 2 folded into their root cause.

## Part 1: the reported bug

### Root cause

`initFromDOM()` (`src/components/lightbox/navigation.js:65-97`) reassigns `this.items` wholesale
from the DOM:

```js
this.items = allItems;                 // navigation.js:74
this.currentPage = parseInt(urlParams.get('page'), 10) || 1;   // :77
this.baseUrl = window.location.pathname + window.location.search;  // :78
this.loadedPages.add(this.currentPage);                        // :89
```

It has no `isOpen` guard, never touches `currentIndex`, and never clamps it. Nothing else in the
store clamps either: `currentIndex` is bounded only inside `open()` (`navigation.js:175`).

### The four unguarded callers

| Caller | Fires when |
| --- | --- |
| `src/main.js:294` | `download-completed`, dispatched from the SSE stream at `src/components/downloadCockpit.js:265`. No user action required. |
| `src/components/mrqlEditor.js:702` | `execute()`, which the popstate handler at `mrqlEditor.js:389` re-runs on browser Back. |
| `src/components/pasteUpload.js:569` | Post-upload page refresh. |
| `src/components/timeline.js:322` | Timeline bucket preview load. |

The download cockpit is included from `templates/layouts/base.tpl:50` on every page and connects to
SSE unconditionally (`downloadCockpit.js:82`), so the first row fires on any page, at any time,
while the viewer is open. That is the sporadic trigger.

### Failure mode A: index out of range, silent black viewer

`getCurrentItem()` is an unguarded index read (`navigation.js:581`). Once `currentIndex >=
items.length` it returns `undefined`, and all three media branches evaluate falsy:

```
templates/partials/lightbox.tpl:133   x-if="isImage(getCurrentItem()?.contentType)"
templates/partials/lightbox.tpl:149   x-if="isSvg(...)"
templates/partials/lightbox.tpl:177   x-if="isVideo(...)"
```

There is no fallback branch, so every media element unmounts. The dialog root is `x-show` on
`isOpen` (`lightbox.tpl:38`), so the backdrop, toolbar, arrows and counter stay up. `loading` is
already `false` (cleared by the previous `@load`) and nothing sets it, so not even the spinner
shows. The counter renders an impossible ratio such as "73 / 50".

Two routes make `items` shorter than `currentIndex`:

1. The user paged forward past the DOM page. `loadNextPage` grows `items` to 100+
   (`navigation.js:465`) while the DOM still holds 50, so the rebuild drops it back to 50.
2. No paging at all. `data-lightbox-item` is emitted on every resource card regardless of content
   type (`templates/partials/resource.tpl:10`), but `_extractItemsFromLinks` filters to media
   (`navigation.js:59-62`). Resource lists are newest-first
   (`models/database_scopes/resource_scope.go:15`), so a completed non-media download prepends a
   card and pushes a media card off page 1. `items` shrinks by one and the last index is orphaned.

### Failure mode B: index in range, wrong image

The prepend shifts every index by one, so `getCurrentItem()` resolves to a neighbour. The picture
changes with no user input. This is the common case and it is silent.

### Failure mode C: stale zoom transform clips the image away

Distinct mechanism, same trigger. The image's position is entirely
`transform: scale(zoomLevel) translate(panX, panY)` (`lightbox.tpl:140`), clipped by the
`overflow-hidden` ancestor at `lightbox.tpl:100`. `constrainPan` derives its bounds from the live
media element (`zoom.js:326-339`), so a clamp legal for image A can put image B entirely outside the
box. Every other path that swaps the displayed image resets zoom (`next`/`prev` at
`navigation.js:385,413`; `refreshCurrentItem` at `cropPanel.js:171`), and `cropPanel.js:169` carries
the comment "The image is a different size now, drop any stale zoom/pan". The `initFromDOM` path
does neither, and `onMediaLoaded` (`navigation.js:584`) only sets `loading = false`.

### Why only close and reopen recovers it

`open()` clamps (`navigation.js:175`) and `close()` calls `resetZoom()` (`navigation.js:330`).
Paging out of it does not work: `initFromDOM` adds to `loadedPages` without clearing it
(`navigation.js:89`) while resetting `currentPage` from the URL, so the first N presses of Next hit
the `loadedPages.has(nextPage)` early return (`navigation.js:448`) and do nothing at all, then the
first real fetch jumps to an unrelated page.

### Fix plan

1. **Guard the destructive rebuild.** `initFromDOM()` returns early, or reconciles instead of
   rebuilding, when `this.isOpen`. This is the single change that closes failure modes A, B and C
   and removes the precondition for two lower findings below.
2. **Re-anchor rather than clamp.** Wherever `items` is reassigned while open, relocate
   `currentIndex` by the current item's `id` and fall back to a clamp. A clamp alone silently moves
   the user to a different image.
3. **Reset the pagination cursor.** `initFromDOM` must install a fresh
   `loadedPages = new Set([currentPage])`, matching what `_openFromSourceContainer`
   (`navigation.js:157`) and the standalone branch (`navigation.js:135`) already do.
4. **Reset or re-clamp zoom on any item swap**, and call `constrainPan()` from the media-load path
   so a pan can never outlive the bitmap it was clamped against.
5. **Add a fallback media branch** in `lightbox.tpl` so an unrenderable item announces itself
   instead of presenting as a silent black rectangle. Defense in depth for any future path that
   orphans the index.

### Regression test

Playwright, against an ephemeral server, in `e2e/tests/lightbox/`: seed two pages of media, open the
viewer, page past the boundary, then `page.evaluate` a `download-completed` CustomEvent (or a direct
`Alpine.store('lightbox').initFromDOM()`), and assert the `<img>` is still present with the same
`src`. Red before the fix, green after.

## Part 2: other confirmed defects

Ordered by verified severity, not by claimed severity.

### High

1. **`refreshPageContent` morph pins stale Alpine state.**
   `src/components/lightbox/editPanel.js:129-135` copies `_x_dataStack` across in `updating()` but
   omits the destroy/re-init pass that `src/main.js:275-292` documents as required, so cards whose
   `x-data` attribute changed keep their old component state after an in-viewer edit.

### Medium

2. **Drag release closes the viewer.** `lightbox.tpl:110` has `@click.self="close()"` on the same
   element that carries the mouse drag handlers, so releasing a pan over the letterbox background
   dismisses the lightbox. `handleMouseUp` (`gestures.js:385`) already computes the drag distance.
3. **`toggleTabTag` reads slot state after the await.** `quickTagPanel.js:493` recomputes
   `slotMatchState(index)` from the current tab, so a tab switch mid-flight flips add into remove.
4. **Source-container paging skips 45 resources.** `_openFromSourceContainer` treats the 5-item
   server preview as page 1 and then requests page 2 of a 50-per-page endpoint
   (`navigation.js:158,162`), so items 6 to 50 are never reachable.
5. **`Cmd/Ctrl+R` and `Ctrl+U` write tags.** `lightbox.tpl:78-79` lack the
   `!$event.metaKey && !$event.ctrlKey` guard that line 73 has, so reloading the page first applies
   the previous image's tags, and view-source first undoes a tag change.
6. **Long-press timer has no slot identity.** `quickTagPanel.js:957` shares one
   `_longPressTimer`/`_longPressSlotIdx` pair, so overlapping presses expand the wrong slot.

### Low

7. `Cmd/Ctrl + X/C/V` switch the quick-tag tab (`lightbox.tpl:74-77`), same missing guard as item 5.
8. `loadPrevPage` moves `currentPage` backwards (`navigation.js:507`), so the next forward press
   hits the already-loaded early return and does nothing.
9. A page whose resources are all non-media ends pagination permanently
   (`navigation.js:459-463`) instead of continuing to the following page.
10. `fetchResourceDetails` commits on resource id alone (`editPanel.js:213`), so a read started
    before a write can land after it and revert the edit visually.
11. `_preloadDetailsUpcoming` writes `detailsCache` unfenced (`navigation.js:274`), so a prefetch
    started before a tag write can restore pre-write tags.
12. `download-completed` morphs only the first list container (`main.js:256`).
13. SVG `<object>` reuse lets `checkIfMediaLoaded` clear the spinner against the previous document
    (`navigation.js:606`).
14. `_advanceFlow` leaves `_pendingFlowPrefix` dangling when `next()` cannot advance
    (`quickTagPanel.js:690`), so a stale announcement attaches to a later navigation.
15. `close()` restores `_itemsBeforeStandalone` unconditionally (`navigation.js:322`). Fixed for
    free by item 1 of the root-cause plan. The adjacent and larger gap the finder missed: `close()`
    restores `items` but not `baseUrl`, `currentPage`, `loadedPages`, `hasNextPage` or
    `hasPrevPage`, which `openFromClick` clobbered at `navigation.js:133-137`, so after any
    standalone open the page's gallery stops paginating. Deterministic, no race required.

## Implementation, 2026-08-06

All 24 confirmed findings are fixed. Regression test first: `e2e/tests/lightbox/items-refresh.spec.ts`
failed with "element(s) not found" for the viewer's `<img>` before the change, passes after.

### The root cause

`initFromDOM()` now splits into three parts (`src/components/lightbox/navigation.js`):

- When the viewer is closed it behaves as before, except `_readPaginationFromDOM` installs a
  fresh `loadedPages` Set instead of adding to the old one.
- When the viewer is open and is paging the page's own gallery, `_reconcileOpenItems` merges
  the scan in without ever shortening the list: present entries are patched, new cards are
  inserted at their DOM position, and entries the DOM no longer shows are kept because
  "scrolled off page 1" and "deleted" are indistinguishable here. `_reanchorIndex` then puts
  `currentIndex` back on the resource that was actually displayed, by id, clamping only when
  that resource is genuinely gone.
- When the open viewer is *not* the page's gallery, the scan is ignored entirely. A standalone
  item (`openFromClick`'s fallback), a source-container preview (`_openFromSourceContainer`)
  and a note block's gallery (`src/components/blocks/blockGallery.js`) each hold a list from a
  different endpoint, so merging page cards in would grow a deliberately narrow viewer.
  `_ownsPageGallery` tracks this.

Zoom is dropped when the merge changes which resource (or which version) sits under the
viewer, and `onMediaLoaded` now calls `constrainPan()` so a pan can never outlive the bitmap
it was clamped against. `lightbox.tpl` gained a fourth media branch that says the item is
unavailable, so no future path can present as a silent black rectangle.

### Everything else

| Finding | Fix |
| --- | --- |
| Morph pins stale Alpine state | `morphAndReinitChangedComponents` in `src/utils/shortcodeElementMorph.js`, used by both `main.js` and `refreshPageContent` |
| Drag release closes the viewer | `consumeDragClick()` in `gestures.js`, gating both `@click.self` handlers |
| `toggleTabTag` reads the current tab post-await | decides from the tags the press captured |
| Source-container paging skips 45 | `_previewSeeded`: the first fetch asks for page 1 and replaces the preview |
| `Cmd/Ctrl+R` and `Ctrl+U` write tags | `canPanelShortcut($event)` rejects modifier chords; the two bindings that *are* chords pass no event |
| `Cmd/Ctrl+X/C/V` switch tabs | same guard |
| Long-press timer has no slot identity | new presses cancel the pending one, releases must match `_longPressSlotIdx` |
| Backward paging stalls Next | `_nextPageNumber`/`_prevPageNumber` derive from the loaded range, not `currentPage` |
| All-non-media page ends pagination | `loadNextPage` walks up to 20 pages while the server reports more |
| Stale read reverts an edit | `_detailsGen` per resource; `_invalidateDetailsReads` on every write path |
| Prefetch clobbers a tag write | same generation, checked before the cache write |
| Only the first list container refreshes | `main.js` pairs every `LIST_CONTAINER_SELECTOR` match |
| SVG spinner clears off the old document | `checkIfMediaLoaded` compares `contentDocument.URL` to the current `viewUrl` |
| Dangling flow-mode prefix | `_advanceFlow` awaits `next()` and announces if nothing consumed it |
| Standalone close loses pagination | `_capturePaginationContext`/`_restorePaginationContext` |

Duplicate appends are also filtered in both page loaders, since a page boundary that shifts
while the viewer is open can return the same resource twice.

## Part 3: refuted

Reported by a finder, then refuted against the source. Recorded so they are not re-found.

- `scheduleMediaCheck`'s document-global selector (`navigation.js:618`). Every earlier
  `role="dialog"` in document order holds no img/video/object, and the lightbox is the first child
  of `.overlays` (`templates/layouts/base.tpl:163`).
- Dead `:key` on the `<video>` (`lightbox.tpl:180`). Genuinely inert, but the consequence does not
  follow.
- `===` in the Next/Prev `disabled` expressions (`lightbox.tpl:196,208`). Real, but the
  out-of-range precondition is unreachable through the lightbox's own writers. It becomes live only
  under the root-cause bug above, so fixing that also retires this.
- `updateItemsFromDOM` selector divergence (`editPanel.js:146`).
  `templates/listResourcesDetails.tpl:21` carries `.gallery` inside the wrapper, so it resolves.
- `onCropSuccess` reading `getCurrentItem()` late (`cropPanel.js:68`). Real asymmetry with
  `rotateCurrent`, but navigation during the in-flight crop is blocked.
- Zoom-preset popover missing from the wheel exemption list (`lightbox.js:45`).
- `initFromDOM` concatenating multiple containers (`navigation.js:66`). Pagination is never armed in
  that configuration.
- Unguarded `fullscreenchange` handler (`lightbox.js:37`). No other element in the app requests
  fullscreen.
