# Category template preview: dock under the slot being edited

Request: on the Category, Resource Category and Note Type editors, let the author show the live preview right after the template slot editor, instead of only at the bottom of the long Templates fieldset.

## Behavior

- The preview pane header gains a checkbox, **Show under the slot being edited** (`#tp-dock-<entityType>`), with a hint tied to it by `aria-describedby`: moving focus into a slot's controls selects that slot and shows the preview under it; the Custom CSS editor keeps the current slot. Off by default, so nothing changes for anyone who does not opt in.
- On: the pane moves to sit directly after the anchor of the selected slot.
  - Slot `X` or `XCSS` with a `[data-template-cluster="X"]` fieldset: after that fieldset.
  - `CustomCSS`, which has no cluster: after the Custom CSS editor row.
  - No anchor found (a slot the form does not carry): the pane stays where it is.
- On, focus entering a cluster (any focusable inside `[data-template-cluster="X"]`, the CodeMirror editors included) selects slot `X` and moves the pane under it. Focus inside the pane itself, or in the Custom CSS editor, changes nothing: the CSS slot has no markup of its own to show, so following it would blank the frame.
  - Focus within the cluster the pane already follows does nothing: the pane only moves when its anchor changes, because every move reloads the frame.
- On, choosing a slot in the Slot dropdown moves the pane under that slot.
- Every move keeps the focused element where the author is looking. Before the move it records `document.activeElement` and its viewport top; after it, it scrolls by the measured difference (zero when browser scroll anchoring already corrected it), and when that element moved with the pane (the Slot select, the checkbox) it refocuses it with `preventScroll`, since reparenting blurs a focused descendant.
- Off: the pane returns to its original place (a placeholder comment left where the template put it).
- The choice persists per browser in `localStorage` (`templatePreview:docked`), like the pane's remembered entity. A layout convenience, not a setting that has to follow the user across devices.

The pane is moved, not cloned. Alpine 3.15 treats a node removed and re-added in one mutation batch as moved and does not re-run or tear down its component (`onMutate` in `alpinejs/dist/module.esm.js`). Re-inserting the iframe reloads its `srcdoc`: the frame shows the same render, but its scroll and any hydrated widget state reset. That is acceptable for a preview, and moving only when the anchor changes keeps it rare.

No template changes beyond the pane partial; the three editors already mark every cluster with `data-template-cluster`.

## Steps

- [x] E2E first (red): `e2e/tests/template-preview.spec.ts`
  - Docking puts the pane right after the selected slot's fieldset; picking another slot in the dropdown moves it there.
  - Docked, focusing the Custom Summary editor selects the Summary slot, moves the pane after the Summary cluster, and the frame renders that slot.
  - Undocking returns the pane after the Templates fieldset.
  - The choice survives a reload.
  - Docked, keyboard: changing the Slot select with the keyboard leaves focus on the select after the move.
  - Docked, focusing the editor inside the cluster already followed does not reload the frame; focusing Custom CSS keeps the slot.
  - Note type: docking follows `[data-template-cluster="CustomSidebar"]`.
  - Resource category: docked with the Preview slot, the pane follows `[data-template-cluster="CustomPreview"]` (resource-only slot).
- [x] `src/components/templatePreview.js`: `docked` state, placeholder, `_dock()` placement, `focusin` follow, scroll compensation, persistence.
- [x] `templates/partials/form/templatePreviewPane.tpl`: the checkbox.
- [x] `npm run build-js`, commit `public/dist`.
- [x] Docs: the docs-site page that describes the live preview gets one paragraph.
- [x] Gates: Go unit, vitest, browser + CLI E2E, Postgres.

## Review

- Done as planned. Two things the plan did not anticipate:
  - Alpine's initial walk continues from the pane's next sibling, so a restored dock is deferred with `queueMicrotask`; moving the pane inside `init()` would have walked already-initialized clusters a second time.
  - pi code review round 1 (P2): the `focusin` listener was registered after `init()` awaited the default-entity lookup, so focus moving during a slow lookup was not followed. It now registers first; regression test "focus is followed while the preview entity is still loading".
- The keyboard test uses type-to-select: ArrowDown opens the popup of a closed select on macOS instead of changing its value.
- Removing the refocus line turns both focus assertions red, so the refocus is needed and tested.
- pi reviews: plan 2 rounds (1 P1 + 3 P2, then clean); code 2 rounds (1 P2, then clean).
- Gates: vitest 2441 passed; Go unit passed; browser + CLI E2E 2732 passed; Postgres Go passed; Postgres E2E 2732 passed, with 1 flaky test (`lightbox.spec.ts:198`, page scroll on open/close) that passed on retry and does not touch the editors.
