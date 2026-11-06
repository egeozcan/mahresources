# Per-slot previews and in-editor slot history

## Context

The Category, Resource Category and Note Type editors have one **Live preview** pane below the long Templates fieldset. It shows one slot at a time, chosen from a dropdown, and can be docked under the slot being edited (94ff1e6c). Template history lives on a separate page (`/category/revisions`). Its "Load into editor" links navigate back to the edit form with `?loadRevision=`.

The user wants:
1. A preview right after **every** template slot group, always present.
2. On each slot group, a button that opens a **popup** to view, diff and apply that slot's previous versions, with a preview of the old version.

Decisions the user made:
- The per-slot previews **replace** the single pane and its dock option.
- The history button goes on **each slot group**.
- **Apply loads into the editor without saving**, like "Load into editor" today. Undo works, and nothing is saved until Save.

The three editors share all of these partials and components, so the change covers all three carriers, not just Category.

## Behavior

### Shared preview context (top of the Custom Templates fieldset)
- One **Preview against** entity picker. It keeps today's rules: scoped to the category, remembered per category in `localStorage`, an unscoped-sample fallback with its notice, and the "nothing to preview against" message.
- A **Refresh all** button, and the sandbox note ("interactive editors and API-backed widgets are non-functional"), shown once here instead of in each preview.
- It publishes `entityType`, `categoryId`, `entityId` and `generatePath` to `Alpine.store('templatePreview')`, as today. The per-slot Generate buttons (`templateGeneration.js`) keep reading that store unchanged.

### Per-slot preview (last child of each `[data-template-cluster]` fieldset)
- Heading "Preview", a Refresh button, a "Rendering…" status, an iframe titled `Preview of <Slot label>` (h-64), render errors and lint issues. All of this is the existing pane markup, scoped to one slot.
- It renders the cluster's markup slot with the full CSS cascade from the form. That is the same cascade code as today's `refresh()`, including the picker-slot exception.
- **Lazy:** it renders the first time it scrolls into view (IntersectionObserver with a rootMargin of about 200px). An edit to its own slot, or to any CSS field, refreshes it after a 700 ms debounce if it is visible. Off-screen, it is marked stale and renders when it comes into view.
- A new entity picked in the shared context refreshes the visible previews and marks the rest stale.
- Carrier slots (List Header, List Footer) render against the category. On the create form they show today's "Save this category first" message.
- Custom CSS gets no preview of its own: it has no markup. Every slot preview already includes it, so editing it restyles them all.
- Each preview keeps its own request sequence (`_refreshSeq`), so a late response cannot overwrite a newer one.

### Slot history popup (edit forms only: hidden on the create form)
- A **History** button in each cluster, next to its legend, plus one next to the Custom CSS editor. Label: `History of <Slot label>`.
- One dialog per form (a single `slotHistory` component, opened by a window event carrying `{slot, label}`). It is `role="dialog"` with `aria-modal`, a heading, and a Close button. Escape closes it and focus returns to the button that opened it. It follows the confirmDialog pattern: containFocus plus inert on the rest of the page, no `x-trap`. Before writing it, check `src/components/confirmDialog.js` and the memories on focus restore after trap teardown.
- **List:** revisions whose `changedSlots` includes the slot (the server already folds a slot's CSS companion into it). Newest first, with number, date, source and label. Empty state: "This slot has not changed since history began."
- **Selected revision:**
  - Fetch `GET {apiPrefix}/revisions/{n}?id=` and read the `editor` payload.
  - Diff the revision's slot, and its CSS companion, against the **current editor contents** (unsaved edits included). The diff runs on the client with the existing `diff` package (`diffLines`), converted to `{op, lines}` hunks. Those hunks feed the existing `unifiedRows` / `sideBySideRows` from `templateRevisions.js`, so the layout and markup match the History page. Unified or side-by-side, chosen with a toggle. When there is no difference: "Same as the editor."
  - Preview of the revision's version in a sandboxed iframe: the revision's slot markup, with the revision's companion CSS swapped in when "Include its CSS" is ticked, and the form's other CSS as it is now. It uses the shared entity. Custom CSS shows the diff only, plus one line saying CSS has no markup to preview.
- **Apply** (`Apply #N to the editor`):
  - Writes the slot, plus its CSS when ticked, through the same path as `templateBundle.loadRevision`: `editorsReady` → `setRevisionField` → `recordApplied('loaded:N:slot')`.
  - Closes the dialog and shows the existing revision notice ("Loaded CustomHeader from revision #N. Review the form, then save. Nothing is saved yet.").
  - The slot's preview refreshes through `template-slot-changed`.
  - No extra confirm: the diff on screen is exactly what gets replaced, and undo still works.
- The "View template history" page link stays.

### Removed
- The single pane (`templatePreviewPane.tpl`), the Slot dropdown, the dock checkbox and the `templatePreview:docked` key. A stale key in browser storage does no harm.

### Hardening (from pi plan review, round 1)
- **No async gap at Apply.** Opening the dialog first awaits `editorsReady([slot, slotCSS])`, then reads the current contents for the diff. Until that finishes the dialog shows "Loading…" and Apply is disabled. Apply then writes synchronously from an immutable selection `{session, slot, number, editor, includeCss}`. Each open or close increments `session`, so a result from a closed dialog commits nothing. While the dialog is open the page is inert, so the editors cannot change underneath the diff.
- **Freshness inside the dialog.** The revision list and revision detail requests each carry a request id, checked as in `templateRevisions.loadDiff`. Choosing another revision clears the diff, the preview and Apply until its own response arrives. A failed fetch shows an error and Apply stays disabled.
- **Placement and focus.** The dialog is teleported into the page's `.overlays` container (`x-if` wrapping `x-teleport`), so confirmDialog's `_applyInert` and `containFocus` behave as they do for the confirm dialog.
  - The preview iframe inside the dialog is `inert` and `tabindex="-1"`: it is a picture of the old version, not something to interact with. Its render errors and lint issues are listed as text outside the frame. That keeps `containFocus`'s plain-controls rule true.
  - Escape or Close returns focus to the opener. A successful Apply skips that, and focus goes to the revision notice instead.
- **Notice after Apply.** The dialog dispatches `template-revision-loaded` with `{text}` on the document. `templateBundle` handles it with its existing `announceRevision`, which owns `revisionNotice`. The dispatch happens one macrotask after the dialog closes, so focus restore and the notice do not compete.
- **Preview endpoint and readiness.** `previewPath` is a parameter of the `slotPreview` include, not read from the store.
  - The store gains `entityGen`, a counter bumped on every entity change. Each slot preview `$watch`es it and takes it as a signal to rerender or go stale.
  - Before the default lookup finishes, a slot preview shows "Choosing an entity to preview against…" rather than the "nothing to preview against" message.
- **Stale invalidation.** Each slot preview keeps a generation counter that goes up on every edit or entity change, even while it is off screen or still debouncing. It aborts the previous fetch with an `AbortController`, and a response commits only when its generation is still current.
- **The dialog's preview render has its own freshness check.** It keeps its own generation counter and `AbortController`. Changing the selection, toggling CSS, changing the entity or closing the dialog invalidates it. The dialog session and generation are checked again after every await. Tests delay the responses for revision A→B and for CSS on→off. (pi round 2)
- **Prepare and write are separate exports.** `templateBundle` exports `prepareRevisionFields(fields)` (async: `editorsReady`) and `writeRevisionFields(editor, fields)` (sync: `setRevisionField` for each field). `loadRevision` awaits the first, then calls the second. The popup awaits preparation before it enables Apply, then writes, records provenance and closes without awaiting anything. Tests cover a readiness failure (Apply stays disabled, with an error) and a close during preparation (nothing is written). (pi round 2)
- **The opener travels with the event.** The History button's handler captures `$event.currentTarget` synchronously and sends it as `opener`. Focus is restored to that element. A test opens the dialog with a pointer click on a button that does not have focus. (pi round 2)
- **Picker freshness.** A default lookup that finishes after the user has already picked an entity is discarded. Search responses older than the current query are dropped.
  - Out of scope, and reported to the user: the picker's existing combobox lacks arrow-key navigation and `aria-activedescendant`. Moving it onto `src/selector/` is its own change.

## Implementation

**Plumbing to reuse rather than rewrite**
- `src/components/templatePreview.js`: split it. A pure module `templatePreviewRender.js` gets `SLOTS`, `slotsForEntityType`, `renderErrorsFrom`, the CSS-cascade builder, the request-body builder and `frameDocument(html, css, entity, slot)`. Then two components:
  - `templatePreviewContext`: the entity picker, default lookup, store and storage, from today's init and search code.
  - `slotPreview({ slot })`: the observer, refresh and frame code.

  The docking code is deleted.
- `src/components/templateBundle.js`: move `editorsReady`, `setRevisionField` and the field-selection logic in `slotFields` behind one exported helper, `applyRevisionFields(editor, fields)`. Today's `loadRevision` and the new popup then share it, and `recordApplied` stays reachable through `announceApplied` (`revisionApplied.js`). Keep the diff small: extract, don't rewrite.
- `src/components/templateRevisions.js`: export `unifiedRows`, `sideBySideRows` and the row helpers (`marker`, `srText`, `rowClass`, `gapText`). They are already exported or easy to lift.
- New `src/components/slotHistory.js` and `templates/partials/form/slotHistoryDialog.tpl`. The diff markup is copied from `templateRevisions.tpl`; if it is copied twice, extract it into `partials/form/revisionDiffField.tpl` and include it in both.
- New partials: `templates/partials/form/templatePreviewContext.tpl` and `templates/partials/form/slotPreview.tpl` (with `slot`, `label`), plus the History button inside it or beside the legend.
- `templates/createCategory.tpl`, `createResourceCategory.tpl`, `createNoteType.tpl`:
  - include the context at the top of the Custom Templates fieldset;
  - add `slotPreview` at the end of each cluster;
  - add a History button beside Custom CSS;
  - include the dialog once;
  - drop the old pane include.
- `src/main.js`: register the new Alpine data components. Then `npm run build-js` and commit `public/dist` with explicit paths.

**Docs**
- `docs-site/docs/features/custom-templates.md` § Live preview: rewrite it for per-slot previews and add the history popup. Also fix the "live preview pane" mention near the list header (line ~371).
- `docs/plans/2026-11-06-per-slot-preview-and-history.md`: a copy of this plan, with `./docs/plans/generate-index.sh`. Add a line in the 2026-11-01 docking plan that per-slot previews superseded it.

## Tests (red first, E2E)

Rewrite `e2e/tests/template-preview.spec.ts` (`/Users/egecan/Code/mahresources/e2e/tests/template-preview.spec.ts`), keeping every behavior that still applies:
- Every cluster on all three editors has a preview frame right after its CSS editor, inside the fieldset.
- Editing Custom Header renders it in the Header preview, and the Summary preview does not rerender. Editing Custom CSS restyles a visible slot preview.
- Lazy: an off-screen preview has sent no request until it is scrolled into view (count requests with `page.on('request')`).
- These carry over to the per-slot frame: entity scoping, the Alpine `entity` scope, carrier mode for List Header plus its create-form message, picker wrapper and companion CSS, render errors listed under the frame.
- Picking another entity refreshes the visible previews.

New `e2e/tests/template-slot-history.spec.ts`:
- Seed three saves of Custom Header. History on the Header cluster lists only the revisions that changed it.
- The diff is against unsaved edits: type into the editor, open History, and the diff shows the typed line as the right side.
- The preview iframe renders the old version.
- Apply writes the slot (and its CSS only when ticked), closes the dialog, focuses the notice, leaves the form unsaved, and the save records `revisionApplied` `loaded:N:CustomHeader`.
- Escape and Close return focus to the History button. Tab stays inside the dialog.
- The create form has no History buttons.
- The Custom CSS history shows a diff and the no-markup note instead of a frame.

Update the other specs that use `#tp-entity-*`, `#tp-slot-*` or the `Template slot preview` iframe title:
- `accessibility/category-template-authoring-a11y.spec.ts`: add an axe run with the dialog open.
- `regressions/ws12-taxonomy-authoring.spec.ts`
- `shortcodes/comment-shortcode.spec.ts`
- `template-generate.spec.ts`

Vitest:
- Move any unit tests of `renderErrorsFrom` or slot lists to the new module.
- Add a test of the `diffLines` → hunk conversion feeding `unifiedRows`.

## Verification
- Run the suites before starting (baseline), then after: `go test --tags 'json1 fts5' ./...`, vitest, `cd e2e && npm run test:with-server:all` (browser + CLI), and the Postgres Go and E2E runs. Read results from `.last-run.json` and redirected logs, never through a pipe.
- Rebuild `./mahresources` before E2E, since the binary goes stale.
- Manual check with playwright-cli on an ephemeral server with seeded revisions: all three editors, the popup at 390px width, and keyboard-only use of the popup.
- A pi review of this plan before ExitPlanMode, and of each implementation batch, stopping at one round with no P0/P1.

## Risks
- **Request load:** about 13 iframes, each loading `/public/dist/main.js`. Lazy rendering, plus refreshing only visible previews, keeps a normal page to 2–4 renders at a time.
- **Unsaved-input loss** (the class every earlier revision-UI blocker came from): Apply only writes after `editorsReady`. The dialog never navigates. An entity switch never touches editors.
