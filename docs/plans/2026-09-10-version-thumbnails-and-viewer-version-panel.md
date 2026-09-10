# Version thumbnails and the viewer's version panel

## Context

A Resource's version history is reachable today only as text. `templates/partials/versionPanel.tpl`
lists `v3 / Jan 02, 2026 / 4.2 MB` rows with Download, Restore, Delete and Compare, and nothing on
that page shows what any of those versions actually look like. Picking the right one means
downloading candidates, or sending two of them to `/resource/compare` and reading the comparator.
For a library of images, which is what most of these Resources are, the one question a reader
actually has ("which of these is the crop I want?") is the one the panel cannot answer.

The viewer has the opposite shape of the same gap. It renders one Resource's current content
beautifully, with zoom, pan, fullscreen, an Info panel and a tagging panel, and it has no idea that
the thing on screen has a history at all.

This adds two surfaces and one endpoint:

1. A thumbnail on every row of the resource page's version panel, which opens that Resource Version
   in the viewer when the Version's content type is one the viewer can display.
2. A version strip inside the viewer, opening above the media, which selects which Resource Version
   the viewer shows.
3. `GET /v1/resource/version/preview`, the downscaled render of one Resource Version, which does not
   exist today. `templates/partials/compareBinary.tpl:1` already records its absence: "No per-version
   thumbnail exists, /v1/resource/preview is keyed on the resource".

Related reading: [2026-01-24-resource-versioning-design.md](archive/2026-01-24-resource-versioning-design.md),
[2026-01-26-version-compare-ui-design.md](archive/2026-01-26-version-compare-ui-design.md),
[2026-08-06-lightbox-bug-hunt.md](2026-08-06-lightbox-bug-hunt.md).

## Vocabulary

Added to [CONTEXT.md](../../CONTEXT.md) as part of this work: **Resource Version**, **Current
Version**, **Historical Version**, **Displayed Version**. Two of them carry a warning worth
repeating here.

"Previous version" names no particular Resource Version, because version numbers are not upload
order once a merge has transferred versions between Resources (`versionPanel.tpl` already says so in
a comment). Use **Historical Version**.

Restoring does not reinstate an old Resource Version. `RestoreVersion` writes a **new** row at
`MAX(version_number)+1` carrying the source's `Hash`, `Location` and `StorageLocation`
(`application_context/resource_version_context.go:344-364`), so two Resource Versions legitimately
hold the same bytes at the same path.

## Model: the Displayed Version

The viewer's `items` array holds one entry per **Resource**, and that stays true. A Resource Version
is not an item. Instead an item gains a **Displayed Version**: when set, the item's `viewUrl`,
`contentType`, `width` and `height` come from that Resource Version, and everything else about the
item keeps describing the Resource.

This is the decision the rest of the plan hangs off, so the reasoning matters. The alternative,
pushing Resource Versions into `items` as entries of their own, buys free prev/next navigation and
pays for it by making `item.id` mean two things. Every site keyed on it would have to learn the
difference: the Info panel's `/resource.json` fetch, the tagging panel, rotate, crop, the bulk
selection bridge (`selectionForElement`), the preloader, and the `/resource?id=` link in the bottom
bar. A Resource Version has no name, no tags, no owner and no related entities. It is not a
Resource, and the counter reading "3 / 7" when the list holds one Resource would be the first of
many small lies.

Consequences that the implementation has to honour, each one a real failure mode rather than a
preference:

- The Displayed Version resets to the Current Version on **every** navigation and on close.
  `onResourceChange()` (`navigation.js`, called from every `next`/`prev` path) is the single hook.
  Sticky history means a reader arrowing back to a Resource sees pixels that disagree with every
  thumbnail on the page behind the viewer.
- `_syncItemFromDetails` (`editPanel.js:435`) must not clobber it. See Hazards below.
- Rotate and Crop transform the Resource's **current** file. They must not run against a Historical
  Version that is on screen.
- A badge says which Resource Version is displayed whenever it is not the Current Version, whether
  or not the panel is open.

## Layer 1: `GET /v1/resource/version/preview`

- [x] `contracts/version_interfaces.go`: add `VersionThumbnailLoader` (embeds `VersionReader`, adds
      `LoadVersionThumbnail(versionID, width, height uint, ctx context.Context) ([]byte, error)`).
      It returns bytes rather than `*models.Preview`, unlike `ResourceThumbnailLoader`, because
      nothing is persisted and a `Preview` value that is never saved invites someone to save it. The
      output is always JPEG (`imaging.Encode(..., imaging.JPEG, ...)`), so the content type is a
      constant at the handler.
- [x] `application_context/resource_media_context.go`: `LoadVersionThumbnail`. Clamp width and height
      to `constants.MaxThumbWidth` / `MaxThumbHeight` (both 600), resolve the filesystem with the
      existing `GetFsForStorageLocation(version.StorageLocation)`, and generate with the existing
      `generateImageThumbnailFromFile(fs, version.Location, w, h, ctx)` (line 709), which already
      carries the HEIC/AVIF ImageMagick fallback and the derive-the-missing-axis fix. Take
      `ctx.locks.ThumbnailGenerationLock.AcquireContext(httpCtx, version.ResourceID)` so a cold page
      with twenty versions does not start twenty concurrent decodes. Key the lock on the **Resource**
      id, not the version id, which is what makes that true.
- [x] `application_context/contract_checks.go`: add the compile-time assertion.
- [x] `server/api_handlers/version_api_handlers.go`: `GetVersionThumbnailHandler`, mirroring
      `GetResourceThumbnailHandler` (`resource_api_handlers.go:584`) rather than inventing a second
      convention:
      - `ETag: "<version hash>-<w>-<h>"`, `Cache-Control: max-age=31536000, immutable`, and the
        `If-None-Match` 304 branch. `immutable` is correct here and would not be on the resource
        endpoint: a Resource Version's bytes never change.
      - A non-image or undecodable Resource Version redirects to `/public/placeholders/file.jpg` with
        `Cache-Control: no-cache`, exactly as the resource handler does. The `no-cache` is
        load-bearing: a format ImageMagick learns to decode later must not be permanently cached as
        a placeholder.
      - Decide on `version.ContentType` alone. Do not consult `Width`/`Height`, which are legitimately
        `0` for AVIF and HEIC.
- [x] `server/routes.go` (beside line 707): register under `scopedAPI`, like every other version
      route. A thumbnail of content is a disclosure of content.
- [x] `server/routes_openapi.go` (beside the `/v1/resource/version/file` entry at line 646): add the
      route. The published spec is generated from this file, so an endpoint missing here is missing
      from the API.
- [x] `templates/partials/compareBinary.tpl:1-4`: the comment's premise becomes false. Rewrite it to
      state the reason the type placeholder is still right (these content types have no renderable
      preview, per-version or otherwise). Change no comparator behaviour.

URL shape, following the convention in `displayResource.tpl:331` and `hovercard.tpl:13`:
`/v1/resource/version/preview?versionId=N&height=H&v=<version hash>`. Height only, width derived.
The `&v=` makes the URL content-addressed, which is what licenses `immutable` rather than leaning on
the ETag alone. Two Resource Versions that share a hash (a restore) get distinct URLs for identical
bytes; that is accepted, since `versionId` is how every other version route is addressed.

## Layer 2: the resource page version strip

- [x] `templates/partials/versionPanel.tpl`: a thumbnail cell at the left of each existing row, at
      `height=64`. Rows keep their Download / Restore / Delete / compare-checkbox layout unchanged;
      `e2e/tests/regressions/version-panel-layout.spec.ts` exists for a reason.
- [x] Include the Current Version's row, which already carries `bg-amber-50` and the `current` badge.
      A strip that omits what you are looking at forces the reader to reconstruct the gap.
- [x] Per content type, decided from `version.ContentType` only:
      - `image/*`: the real thumbnail, clickable, calling
        `$store.lightbox.openResourceAtVersion(resourceId, versionId, ...)`.
      - `video/*`: a film icon placeholder, still clickable. The viewer plays the Version file
        directly and `http.ServeContent` supports Range requests. Per-version **video** thumbnails
        need ffmpeg, a temp-file copy, the 30s timeout and the `-video-thumb-concurrency` gate; that
        is a separate change (see Out of scope).
      - anything else: no thumbnail, no click. The row renders as it does today.
- [x] The click must pass the Version's content type, width and height through to the store, so the
      viewer does not have to fetch before it can paint.

## Layer 3: the viewer's version panel

New module `src/components/lightbox/versionPanel.js` exporting `versionPanelState` and
`versionPanelMethods`, composed into the store in `src/components/lightbox.js` alongside the existing
five.

- [x] **Data comes from the fetch that already happens.** `/resource.json?id=N` already returns the
      full version list (`versions` is absent from `render_template.go`'s discard list), and
      `editPanel.js:347` throws it away with `data.resource || data`. Capture `data.versions` into a
      `versionsCache` beside `detailsCache`, under the same generation fence
      (`_mayCommitDetails` / `_settleDetailsCache`). No new endpoint, no second fetch, and the
      existing staleness machinery applies unchanged.
- [x] Opening the panel triggers `fetchResourceDetails(undefined, true)` the way `openEditPanel`
      does, so it revalidates on every open.
- [x] Add the panel to `_handleDetailsRevalidate`'s guard in `lightbox.js`
      (`if (!this.editPanelOpen && !this.quickTagPanelOpen) return;`), whose own comment cites "a
      background job re-versions it" as the motivating case.
- [x] **Layout:** a sibling **above** the media area, inside the existing `relative flex-1 flex
      flex-col` column, so the media shrinks for it exactly as it already does for the bottom bar.
      Not an overlay: the side panels need the `lg:ml-[400px]` / `lg:mr-[400px]` dance because they
      overlay, and a top overlay would additionally fight the close button at `top-4 right-4 z-20`.
      In flow at every width, shrinking on narrow (smaller thumbnails, horizontal scroll, label
      collapsing to `v3` alone) rather than becoming a full-screen overlay like the side panels,
      because seeing the media while picking a Version is the reason it goes at the top.
- [x] **Contents:** every Resource Version, newest first, as `GetVersions` already orders them.
      Each entry shows the thumbnail at `height=96` (`height=64` below `md`) or a type badge, `v<n>`,
      the date, the size, and the comment as a `title`. Non-displayable Versions are present but not
      selectable: a timeline that silently omits v4 reads as data loss, and "v4 is a PDF" is the
      honest answer. Header carries a link to the resource page.
- [x] **Viewing only.** No Restore, Compare or Download. Restore is a write behind a confirm, Compare
      navigates away and destroys the viewer session, and the resource page has all three.
- [x] Scroll the Displayed Version into view (`block: 'nearest'`, `inline: 'center'`) on open and on
      selection, or a Resource with forty Versions opens scrolled to v1 with v38 selected off-screen.
- [x] **Keyboard:** `h` toggles, guarded by the existing `canPanelShortcut($event)` helper. The panel
      does not intercept Escape. `handleEscape()` closes the viewer and only the crop overlay and an
      expanded state intercept it today; the side panels do not, and a third rule is not worth
      inventing.
- [x] **Semantics:** a labelled `role="group"` of plain buttons with `aria-current="true"` on the
      displayed entry. Not `tablist` (there is one media stage, not one panel per Version) and not
      `radiogroup` (it implies a form value and hijacks the arrow keys the viewer already owns).
      Announce through the existing `_liveRegion`: "Showing version 3 of 7, uploaded Jan 02 2026" on
      selection, "Showing current version" on reset.
- [x] With one Resource Version, the panel says so rather than the button hiding itself. The count is
      not known until the fetch lands, and a control that appears a beat after the viewer opens is
      worse than one that answers honestly.
- [x] Add `[data-version-panel]` to the wheel-handler exclusions in `lightbox.js`'s
      `_handleWheelEvent`, beside `[data-edit-panel]`, `[data-quick-tag-panel]` and
      `[data-crop-overlay]`. Without it, scrolling the strip horizontally zooms the image underneath.

## Layer 4: the viewer while a Historical Version is displayed

- [x] `openResourceAtVersion(resourceId, versionId, contentType, width, height)` in `navigation.js`:
      seed the one-item standalone session the same way `openFromClick`'s fallback branch does (the
      resource detail page's own preview already takes that branch, because the Resource is absent
      from `items` by construction: `GetSeriesSiblings` excludes it and the sidebar is not a scanned
      container), then set the Displayed Version and open the panel. A dedicated entry point rather
      than a sixth branch inside `openFromClick`, so the page strip and the in-viewer strip call one
      method.
- [x] Reset the Displayed Version in `onResourceChange()` and in `close()`.
- [x] **Badge in the bottom bar**, beside the existing resolution readout, whenever a Historical
      Version is displayed: `Version 3 of 7`, carrying a "Back to current" action. Shown whether or
      not the panel is open. This closes the one hole the model opens: `h` can close the panel while
      history stays on screen, and without the badge a reader is one keystroke from silently
      misreading what they are looking at. It is also what the disabled Rotate/Crop tooltip refers
      to.
- [x] The resolution readout already reads `item.width` / `item.height`, which now hold the
      **Version's** dimensions. No change needed there, but it is why the Displayed Version has to
      carry them.
- [x] **Rotate and Crop render disabled, with the reason attached**, not hidden. `templates/compare.tpl:272-273`
      makes this exact argument for its own Merge control: "Always rendered, disabled with the reason
      attached when merging is unavailable: a control that disappears when you pick an older version
      explains nothing."
- [x] **Info and Edit Tags stay fully live.** They describe the Resource, about which the Displayed
      Version says nothing.

## Hazards found while designing this

**`_syncItemFromDetails` silently reverts the Displayed Version.** `editPanel.js:435` unconditionally
rewrites the item's `viewUrl` to the Resource's current content, and three callers reach it: the
`/resource.json` fetch (line 361), the crop panel (182), and the navigation preloader (456). Because
Layer 3 wires the version panel into `_handleDetailsRevalidate`, switching browser tabs away and back
would yank the viewer from v2 to v5 with no explanation. Fix: while a Displayed Version is set, the
item's `viewUrl`, `width`, `height` and `contentType` come from that Version and sync leaves them
alone; `name` still syncs. If the refreshed version list no longer contains the Displayed Version,
someone deleted it elsewhere: fall back to the Current Version and announce that, rather than leaving
a 404'd image on screen. This is the regression most worth a test, and it is a state-machine property,
so it belongs in vitest rather than a Playwright tab-visibility dance.

**`updateItemsFromDOM` has the same shape.** `editPanel.js:267-296` rebuilds `viewUrl` from
`data-resource-hash` for every item it finds in the page's DOM. It is reached from
`refreshPageContent`, so the same preservation rule applies.

**`[data-lightbox-scope]` is the wrong mechanism here,** despite looking made for it. That branch
builds a gallery of **items** out of the links inside a container, and Resource Versions are not
items. Layer 4's `openResourceAtVersion` is the replacement.

**The viewer's displayability predicate is content type only.** `_extractItemsFromLinks` filters to
`image/*` and `video/*`. A Resource Version's content type can differ from its Resource's, so a PDF
Resource can hold an image Version and the reverse. Never decide from `Width`/`Height`, which are
`0` for AVIF and HEIC.

## Out of scope

- **Per-version video thumbnails.** Video versions are reachable in the viewer via the film-icon
  placeholder; generating a real frame needs `createThumbFromVideoFileAtTime`, a temp-file copy, the
  video-thumb lock and the `-video-thumb-concurrency` gate. A clean follow-up that changes nothing
  shipped here.
- **Real thumbnails in the binary comparator.** Mismatched content types land in `compareBinary.tpl`
  (`compare_template_context.go:140-146`), so after Layer 1 the image side of a PNG-versus-ZIP
  comparison could show a real per-version thumbnail. A genuine improvement, and its asymmetric
  layout deserves designing rather than falling into.
- **Rotate or crop a Historical Version into a new Resource Version.** A coherent feature ("recrop
  from this old version") with its own confirm, provenance comment and test surface. Nothing in the
  request asked for it.
- **A Displayed Version remembered per Resource for the session.** Rejected in design: see Model.
- **The resource page's main preview reflecting a selected Version.** That page is server-rendered;
  the viewer is where Version selection lives.

## Verification

TDD, failing test first, in three layers.

Go:

- [x] Handler tests in `server/api_tests/`: the size clamp against `MaxThumbWidth/Height`, the
      placeholder redirect plus `no-cache` for a PDF Version and for undecodable bytes, the ETag 304
      branch, and that the route is scoped (a group-limited principal cannot read a thumbnail of a
      Version outside its subtree).
- [x] `go test --tags 'json1 fts5' ./...`

vitest:

- [x] New `src/components/lightbox/versionPanel.test.ts`, following
      `src/components/lightbox/staleness.test.ts` and `quickTagPanel.test.ts`: a revalidation does
      **not** revert the Displayed Version; a Displayed Version missing from the refreshed list falls
      back to the Current Version and announces it; `onResourceChange()` resets it; the panel's
      entries list non-displayable Versions as non-selectable.
- [x] `npm run test:unit`

E2E:

- [x] Extend `e2e/tests/resource-versioning.spec.ts`: thumbnails render on image Version rows, a
      video Version row is clickable with no thumbnail, a PDF Version row has neither, and clicking
      an image Version opens the viewer showing that Version with the panel open.
- [x] New `e2e/tests/lightbox/version-panel.spec.ts`: `h` toggles the panel; selecting a Historical
      Version swaps the media and shows the badge; "Back to current" restores; Rotate and Crop are
      disabled with a reason while history is displayed; Info and Edit Tags still work; navigating
      away resets; a11y roles and the live-region announcements.
- [x] `npm run build` first. E2E reuses the prebuilt `./mahresources`, so a stale binary tests the
      old backend.
- [x] `cd e2e && npm run test:with-server:all` (browser and CLI in parallel)
- [x] `cd e2e && npm run test:with-server:a11y`

Postgres, per CLAUDE.md, when the feature is finished:

- [x] `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
- [x] `cd e2e && npm run test:with-server:postgres`

Docs and generated artifacts:

- [x] `docs-site/docs/features/versioning.md`: the thumbnails and the viewer panel.
- [x] `docs-site/docs/features/thumbnail-generation.md`: the new endpoint, its immutability and the
      fact that it persists nothing.
- [x] `go run ./cmd/openapi-gen` and commit the regenerated spec.
- [x] `npm run build-js`, and commit `public/dist/` alongside the source change that caused it.
- [x] `./scripts/css-scan-test.sh` if any new Tailwind class lands in a template.
- [x] `./docs/plans/generate-index.sh`


## Implementation notes

- Historical SVGs render through an image element: the version-file endpoint serves attachments, which browsers reject in an embedded document. Current SVGs retain the existing object viewer.
- Virtual v1 rows (ID zero, for Resources not yet migrated to stored versions) use the Resource preview and media URLs.
- Strip thumbnails mount only while the panel is open. Media loading and zoom measurements target the media stage explicitly.
- The shared details fetch also survives closing Info or Edit Tags while Versions remains open. The standalone version entry point avoids a duplicate details request when Edit Tags is already open.
- Generated OpenAPI, JavaScript and CSS artifacts are included alongside the source changes.
- The broad regression run exposed stale viewer-dialog selectors and bulk-toolbar assertions from the generic list migration. Those tests now address the viewer and shared controls directly. It also exposed the Select All row's initial zero-versus-zero registry comparison; the predicate now treats an empty selection on a populated page as selectable immediately.

- Hover previews are excluded from the version strip so its Resource page link cannot obscure version choices.


### Verification results

- Go: `go test --tags 'json1 fts5' ./...` passed.
- Postgres Go: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1` passed.
- Vitest: 82 files, 1,332 tests passed.
- SQLite browser and CLI: 2,198 passed, 5 skipped, no failures or retries on the final full run.
- Dedicated accessibility suite: 211 passed; the new version strip also passes its focused axe audit.
- Postgres browser and CLI: 2,196 passed, 3 passed on retry, 4 skipped. The intermittent cases were corrected; the final SQLite run and 72 repeated Postgres checks (three repetitions) passed without retries.
- Full build, OpenAPI generation, plan index generation and CSS source scan passed. Generated files are included in the working tree.

### Review loop

- Three independent Standards/Spec review rounds completed. Round 1 found focus loss when closing history and stale version state after crop/rotate; round 2 found retained details could discard a requested Historical Version after reopening the viewer. All three major findings were fixed, with regression coverage. Round 3 found no major issues on either axis.
- Closing the strip or using Back to current now returns focus to Versions when the disappearing control had focus. Accepted crop/rotate refreshes update the version list and Current Version marker under the existing generation fence. Closing the viewer clears retained Resource details.
- Final review validation: all 1,336 unit tests, 131 targeted viewer/version/accessibility browser tests, and 24 repeated PostgreSQL viewer/crop tests passed without retries. The JavaScript bundle was rebuilt, and source diff whitespace checks passed.
