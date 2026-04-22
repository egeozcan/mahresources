# Bug Hunt Log (Continuous Loop)

Started: 2026-04-21
Cadence: every 30 minutes, loop id `350a88ca`
Server: http://localhost:8181
Canonical log maintained by `/loop` orchestrator. Sub-hunters (Sonnet) append findings; orchestrator dedupes + verifies.

## Columns

- **ID**: `BH-<NNN>` assigned once, stable
- **Status**: `unverified` (fresh from hunter), `verified` (orchestrator reproduced), `fixed` (patched and re-checked), `wontfix`, `dupe`
- **Severity**: `critical | major | minor | cosmetic | feature-gap`
- **Evidence**: screenshot / HAR / curl / console log
- **Workflow**: the real-world flow in which it surfaced

---

## Prior known issues (from `tasks/bug-report-consolidated.md`, 2026-03-26) — now all resolved or promoted

| ID | Outcome |
|----|---------|
| BH-P01 | **fixed** (iter 7) — see Fixed table below |
| BH-P02 | **fixed** (iter 7) |
| BH-P03 | **fixed** (iter 7) |
| BH-P04 | **fixed** (iter 4) |
| BH-P05 | **fixed** (2026-04-22, c1-error-hygiene, merged 0aa5d39e) — see Fixed table below |
| BH-P06 | **fixed** (iter 4) |

---

## Active bug log

_(populated by iterations — newest first)_

### BH-039 · BH-011 image ingestion over-rejects valid SVG/ICO/WebP/AVIF/HEIC uploads with HTTP 400
- **Status:** verified (discovered during e2e-fixture-repair, 2026-04-22)
- **Severity:** major (regression) — breaks a previously-working feature (SVG upload, lightbox display) and the `mr resource from-url` CLI path against any `.ico`/non-Go-decodable source
- **Workflow:** `POST /v1/resource` (multipart) and `/v1/resource/remote` (URL fetch) for image MIME types Go's stdlib cannot decode natively.
- **Repro:**
  - `curl -X POST http://localhost:8181/v1/resource -F 'resource=@e2e/test-assets/sample-image.svg' -F 'Name=x'`
    → HTTP 400 `{"error":"following errors were encountered: uploaded file is not a valid image (failed to decode): image: unknown format"}`
  - Same for `.ico`, `.webp`, `.avif`, `.heic` (any file `mimetype.DetectFile` labels `image/*` that isn't PNG/JPEG/GIF/TIFF).
- **Root cause (code-verified):** `application_context/resource_upload_context.go:589-603` (BH-011 fix, commit 64b8005c) runs `image.Decode(tempFile)` on **every** `image/*` MIME and treats **any** decode error — including `image.ErrFormat` (= "image: unknown format") — as a 400. The BH-011 intent was to catch truncated/corrupt uploads, not to reject formats Go lacks a decoder for.
- **Impact:**
  - E2E test `tests/13-lightbox.spec.ts › Lightbox SVG Support` is skipped in `test/e2e-fixture-repair` pending this fix.
  - Any deployment whose users have `.svg` (icon libraries, logos), `.webp` (modern web image), or `.heic` (iPhone camera) in their import pipeline now sees 400s where pre-c3 they'd get a resource row (with Width=0/Height=0).
- **Fix (narrow, preserves BH-011):** distinguish `image.ErrFormat` from other decode errors. Either:
  1. `if errors.Is(decErr, image.ErrFormat) { preWidth, preHeight = 0, 0 /* keep the resource, skip dims */ } else { return &InvalidImageError{Cause: decErr} }`
  2. Or bound the check to the set of MIME types for which a decoder is registered via `image.RegisterFormat` (png/jpeg/gif/tiff for this codebase).
  Option 1 is a 2-line change. Either option keeps the truncated-PNG regression guard in `image_ingestion_rejects_truncated_test.go` green.
- **Side-finding source:** surfaced while repairing 41 e2e failures caused by a separate fixture bug (5 truncated PNGs on disk). The SVG lightbox test was the only e2e test that still failed after repairing fixtures — and it failed because the backend's image gate is too wide, not because the SVG fixture is bad.

### BH-038 · Notes-list page serializes `shareToken` into Alpine `x-data` — all share tokens readable from `/notes` HTML source
- **Status:** verified (iter 13, orchestrator re-confirmed via curl of `/notes?shared=true` — three share tokens in the page body)
- **Severity:** cosmetic today (private-network / no-auth per CLAUDE.md), **latent major** the moment any auth/multi-user layer is added
- **Iter:** 13 · **Workflow:** share management UX exploration
- **Repro:** `curl -s 'http://localhost:8181/notes?shared=true' | grep shareToken` returns the raw token values embedded in every note card's `x-data` attribute.
- **Root cause (hypothesis):** the note list context passes the full note JSON to the card template; the card template serializes all fields into `x-data` without stripping sensitive ones. The `shareToken` is only needed on the note's own sidebar (via the detail template) — not by any list-card logic.
- **Fix:** in the notes list context provider (or in the card partial's pre-serialization), omit `shareToken` from the payload. If a specific UI piece needs to know "is this note shared at all", expose a boolean `hasShare` instead.
- **Why this matters even today:** any log aggregator, proxy cache, or browser session viewer (devtools → view source) with access to `/notes` captures share tokens in plaintext. Once the operator shares a note's URL via email, the token is effectively the password; leaking all tokens in a single HTML response is a wider blast radius than necessary.

### BH-037 · Perceptual-hash values (AHash/DHash) never exposed in the resource UI — cannot debug similarity misses
- **Status:** verified (iter 13)
- **Severity:** cosmetic / observability gap (tightly related to BH-018's DHash-on-solid-color false-positive bug)
- **Iter:** 13 · **Workflow:** admin surfaces
- **Observed:** resource detail "Technical Details" section shows the SHA1 file-integrity hash. The **perceptual** hashes used by the similarity engine (stored in the separate `resource_hashes` table) are not surfaced anywhere. The admin overview shows aggregate totals (e.g. "85 hashed images, 64 similar pairs") but no per-resource visibility. The "Similar Resources" panel surfaces the *result* of the comparison but not the input hash values.
- **Why it matters now:** BH-018 is known — solid-color images collide at DHash=0. An operator trying to understand why two unrelated solids show as "similar" has no way to see "oh, both have DHash=0" without running a SQL query against `resource_hashes`.
- **Fix:** extend the resource-context fetch to include DHash/AHash when present, and add a row in the Technical Details collapsible like `Perceptual hash (DHash): 0x0000000000000000 (AHash: 0xabc…)`. Also a small admin-overview drill-down for "resources waiting to be hashed" / "resources where DHash=0" would help operators act on BH-018 proactively.

### BH-036 · Export UI does not disclose the 24 h (default) retention window — completed tars vanish with no prior warning
- **Status:** **FIXED** (2026-04-22, c10-jobs-ui-polish, PR #35 merged dd2c68b2 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 13)
- **Severity:** minor UX (compounds BH-025 / BH-026 into a genuinely frustrating recovery scenario)
- **Iter:** 13 · **Workflow:** admin-export surface
- **Observed:** `/admin/export` renders group picker + scope toggles + fidelity options + "Start export" button. Zero text anywhere referencing `-export-retention` (24 h default) or warning that completed tars expire. The Download Cockpit panel (see BH-028) also shows no per-job countdown or expiry timestamp.
- **Compound scenario:** an operator starts an export, reloads the page (BH-025 → page blanks), navigates away, returns a day later to finish downloading — the tar is gone, the cockpit (BH-026) doesn't even show the entry usefully, and there was never a warning.
- **Fix:** (cheap) add a static helper text "Completed exports are available for download for `{{ config.ExportRetention }}` after completion." on the export page; surface an expiry timestamp per job in the cockpit's completed-job row. (medium) add an admin-overview metric for "exports expiring in next 4 h".

### BH-035 · No centralized shared-notes management dashboard — revocation requires per-note navigation
- **Status:** verified (iter 13)
- **Severity:** minor (UX / feature gap; bites proportionally with number of active shares)
- **Iter:** 13 · **Workflow:** share management
- **Observed:** the only exposure of "which notes are shared" is a `?shared=true` filter checkbox on the notes list, and even there the list shows just note names — no share URL, no creation timestamp, no one-click revoke. To revoke a single share, the operator must: open the note → scroll to the Sharing sidebar section → click "Unshare". For 20 shared notes that's 20 separate navigations.
- **Also missing:** a `shareCreatedAt` timestamp on the note model, a `shareExpiresAt` (per BH-033 context — no expiry exists), a bulk-revoke affordance, an "audit trail" of past shares for a note.
- **Fix (layered):**
  1. Add `shareCreatedAt` to the note model; set it when a share token is minted.
  2. Add a dedicated `/admin/shares` view: table of every note with a token, columns `Name | Token URL | Created | Revoke`.
  3. Optional: bulk-revoke from that view, and "revoke-on-expiry" if BH-033's expiry-field fix lands.

### BH-034 · No request-body size limit on `/v1/resource` (multipart) and `/v1/resource/versions` upload paths
- **Status:** verified (iter 12 — 25 MB multipart uploaded successfully; code-confirmed)
- **Severity:** minor today, potentially major in `millions of resources` deployments (CLAUDE.md) — an unbounded POST can exhaust disk
- **Iter:** 12 · **Workflow:** body-size probe
- **Root cause (code-verified):**
  - `server/api_handlers/version_api_handlers.go:86` calls `r.ParseMultipartForm(100 << 20)` with **no `http.MaxBytesReader` preceding it**. The 100 MB limit only controls the in-memory buffer; the rest spills to disk unbounded.
  - `server/api_handlers/resource_api_handlers.go` (resource upload) similarly has no `MaxBytesReader` guard — empirically confirmed by uploading a 25 MB random binary that was accepted with HTTP 200 in 56ms.
  - **Contrast with the correct pattern at `import_api_handlers.go:41-45`:** that handler does `r.Body = http.MaxBytesReader(w, r.Body, maxSize)` before `ParseMultipartForm`. `MaxImportSize` is a documented flag (10 GB default per CLAUDE.md). The resource/version paths need the same discipline.
- **Fix:** add a `MAX_UPLOAD_SIZE` / `--max-upload-size` config (default e.g. 2 GB), and wrap `r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)` in both resource and version upload handlers before parsing multipart.

### BH-033 · `ShareBaseUrl` uses the server's bind address verbatim — loopback bind produces non-routable share URLs
- **Status:** verified (iter 12)
- **Severity:** minor (UX / config)
- **Iter:** 12 · **Workflow:** share server reconnaissance
- **Repro:** with `SHARE_BIND_ADDRESS=127.0.0.1 SHARE_PORT=8383`, the "Share Note" sidebar shows the user `http://127.0.0.1:8383/s/<token>`. Sending that URL to anyone resolves to THEIR own localhost — not the intended share recipient's destination.
- **Root cause:** `server/template_handlers/template_context_providers/note_template_context.go:253-258`:
  ```go
  shareBaseUrl = fmt.Sprintf("http://%s:%s", context.Config.ShareBindAddress, context.Config.SharePort)
  ```
  `ShareBindAddress` is the server-side listen address; safest security practice binds loopback. It's rarely the externally-routable hostname.
- **Fix:** add a `SHARE_PUBLIC_URL` / `--share-public-url` config (empty default). When set, use it as the base; when unset, fall back to the current construction only if bind ≠ `127.0.0.1` / `::1` / `0.0.0.0`. Document the distinction.

### BH-032 · Share server responses lack security headers (CSP, X-Frame-Options, Referrer-Policy, X-Content-Type-Options)
- **Status:** verified (iter 12)
- **Severity:** minor (the share server is the only externally-linked surface by design; missing headers compound other bugs)
- **Iter:** 12 · **Workflow:** share server reconnaissance
- **What's missing on `GET /s/<token>`:** `X-Frame-Options`, `Content-Security-Policy`, `Referrer-Policy`, `X-Content-Type-Options`, `Strict-Transport-Security`. The error path sets `X-Content-Type-Options: nosniff` (from Go's `http.Error`), but the success path doesn't.
- **Why it matters:**
  - **Clickjacking**: without `X-Frame-Options: DENY`, a shared page can be framed in a malicious site and used for UI-redressing.
  - **Referrer leakage**: without `Referrer-Policy: no-referrer`, the share token appears in the `Referer` header on every outbound request (Google Fonts, any external image). `templates/shared/base.tpl:5` loads Google Fonts — so tokens leak to Google by default.
  - **CSP**: `note.Description|markdown2|safe` template binding means a future XSS introduction has no CSP backstop. Not exploitable today (iter 8 confirmed block-editor escaping is solid), but defense-in-depth matters on an externally-facing surface.
- **Fix:** add a middleware to `ShareServer.Start()` (`server/share_server.go:59-84`) that sets these four headers on every response. Apply the same middleware to the primary server for consistency (secondary benefit).

### BH-031 · Share server block-state write endpoint accepts ANY block type — not just `todos`
- **Status:** **FIXED** (2026-04-22, c8-share-allowlist, PR #30 merged 3bed7dd8 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 12, code-confirmed at `share_server.go:131-173`)
- **Severity:** **medium** (genuine public-surface defect — any share token holder can persist arbitrary state changes to any block in the note)
- **Iter:** 12 · **Workflow:** share server security probes
- **Repro:**
  1. Create a note with a gallery block; share it; get the token.
  2. `POST http://localhost:8383/s/<token>/block/<galleryBlockId>/state` with JSON body `{"layout":"list"}` (or `{"layout":"grid","injected":true}`).
  3. HTTP 200. The gallery's state column is overwritten with the supplied JSON — visible to every subsequent viewer.
  4. Works for gallery, text, references, heading, divider. Only intended for `todos` (per line 129 comment: "e.g., todo checkbox").
- **Root cause (code-verified):** `server/share_server.go:131-173` verifies (a) the token resolves to a note and (b) the block belongs to the note. It does NOT check the block's type against an allowlist before calling `UpdateBlockStateFromRequest`. Most block types' `ValidateState` accept any JSON (they only reject structurally invalid input), so arbitrary state shapes land in the DB.
- **Impact:**
  - **Integrity:** anyone with the share link can vandalize state — flip gallery layouts, alter calendar views, or write garbage fields that the owner can't detect without looking at raw DB rows.
  - **No content modification** is possible — `/s/.../block/.../content` route doesn't exist, so block text/gallery selection is safe. This is strictly a state-column defect.
  - **No privilege escalation** — the token grants share-level access only; this doesn't break out of the share scope.
- **Fix:** in `handleBlockStateUpdate` (same function), add a block-type allowlist before the `UpdateBlockStateFromRequest` call:
  ```go
  allowedStateTypes := map[string]bool{"todos": true /*, "calendar": true if view-state persistence is desired*/}
  var targetBlock *models.NoteBlock
  for i := range note.Blocks { if note.Blocks[i].ID == blockId { targetBlock = &note.Blocks[i]; break } }
  if targetBlock == nil || !allowedStateTypes[targetBlock.Type] {
      http.Error(w, "Block type does not allow share-token state writes", http.StatusForbidden); return
  }
  ```
- **Evidence:** iter-12 hunter's live POSTs + orchestrator code confirmation; `share_server.go:131-173`.

### BH-030 · Resource compare view: diff cards convey change via color only, radiogroup lacks roving tabindex
- **Status:** verified (iter 11, a11y audit)
- **Severity:** minor (2 moderate WCAG issues)
- **Iter:** 11 · **Workflow:** compare-view a11y audit
- **Sub-issues:**
  1. **WCAG 1.4.1 (Level A)** — `compare-meta-card--diff` cards differ from same-value cards only in left-border color (`rgb(252,165,165)` vs none). Value text sometimes conveys the delta ("File Size: 176 B → 336 B") but many fields ("Hash: Different") don't include an explicit "(changed)" marker. Color-blind users and screen-reader users lose the at-a-glance "what changed" scan.
     - **Fix:** add `<span class="sr-only">(changed)</span>` inside each `compare-meta-card--diff`, or give the card `aria-label="Changed: <field>"`.
  2. **WCAG 2.1.1 (Level A)** — the image-compare mode switcher (`role="radiogroup"` with 4 `role="radio"` buttons) has no roving tabindex: every radio is independently tab-stoppable (adds 3 extra stops), and ArrowRight/Left don't advance selection.
     - **Fix:** `tabindex="0"` on the checked radio, `-1` on the others; `@keydown.arrow-right` / `@keydown.arrow-left` handlers in `compareView.js`.
- **Evidence:** iter-11 audit findings #9, #10; `tasks/bug-hunt-evidence/iter-2026-04-22-6/04-compare-view.png`.

### BH-029 · Group hierarchy tree missing ARIA tree semantics and WAI-ARIA keyboard pattern
- **Status:** verified (iter 11, a11y audit)
- **Severity:** minor (moderate WCAG 1.3.1 + 2.1.1, Level A; tree is tab-navigable but structure is invisible to AT)
- **Iter:** 11 · **Workflow:** group-tree a11y audit
- **Details:**
  - The tree renders as `<ul>/<li>` with expand/collapse `<button>` children. The expand button itself is done WELL — it has `aria-expanded` and a descriptive `aria-label` including the child count (e.g., `"Collapse [group], 3 children"`). That part is correct.
  - Missing on the container and items: `role="tree"` on the outer `<ul>`, `role="treeitem"` on each `<li>`, `aria-level`, `aria-setsize`, `aria-posinset`. Screen readers perceive the structure as a flat list of links and buttons.
  - Arrow-key navigation (ArrowDown/Up between items, ArrowRight/Left expand/collapse, Home/End to first/last) is not implemented. Navigation is Tab-only.
- **Fix location:** `src/components/groupTree.js` `render()` + `renderNode()`. Add ARIA attributes per the WAI-ARIA Tree View pattern and implement arrow-key roving focus.
- **Evidence:** iter-11 audit finding #8; `tasks/bug-hunt-evidence/iter-2026-04-22-6/03-group-tree.png`.

### BH-028 · Download Cockpit accessibility: panel not a dialog, no focus management, progress bars lack ARIA, connection status color-only (3 WCAG A/AA issues)
- **Status:** **FIXED** (2026-04-22, c5-jobs-ui-a11y, PR #27 merged f60bd9f3 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 11, a11y audit)
- **Severity:** **major** (composite — 1 Serious + 2 Moderate WCAG issues in a component that's the intended recovery surface for BH-025/BH-026)
- **Iter:** 11 · **Workflow:** downloadCockpit a11y audit
- **Sub-issues:**
  1. **WCAG 4.1.2 + 2.4.3 (Level A) — Serious.** The slide-in Jobs panel (`downloadCockpit.tpl:28`) has no `role="dialog"`, no `aria-modal="true"`, no `aria-labelledby` pointing at the "Jobs" heading. On open, `document.activeElement === BODY` — no focus moves into the panel. Keyboard users must tab through all remaining page content to reach the panel's close button.
     - **Fix:** add `role="dialog" aria-modal="true" aria-labelledby="jobs-panel-heading"` + a `$watch('isOpen', ...)` in `downloadCockpit.js` that moves focus to `$refs.panel.querySelector('button')` on open and returns focus to the trigger on close.
  2. **WCAG 4.1.2 (Level A) — Moderate.** Progress bars (`templates/partials/downloadCockpit.tpl:110-115`) are styled `<div>`s with `width: N%` and zero ARIA. No `role="progressbar"`, no `aria-valuenow/min/max`. Screen readers go silent between "Download queued" and "Download completed".
     - **Fix:** add `role="progressbar" :aria-valuenow="getProgressPercent(job).toFixed(0)" aria-valuemin="0" aria-valuemax="100" :aria-label="'Download progress: ' + formatProgress(job)"`.
  3. **WCAG 1.4.1 (Level A) — Moderate.** Connection status dot (`templates/partials/downloadCockpit.tpl:40-47`) communicates connection state (green/yellow/red) purely via color, with only `title=` as the text alternative — unreliable for AT.
     - **Fix:** either `:aria-label="'Connection status: ' + connectionStatus"` + `role="img"`, or add a visually-hidden `<span class="sr-only" x-text="connectionStatus"></span>` sibling.
- **Positive:** the `createLiveRegion()` helper IS used and does announce job lifecycle events (queued, completed, failed, paused). That part works well — the gap is focus/structure/progress.
- **Evidence:** iter-11 audit findings #5, #6, #7; eval results `panelRole: null, panelAriaModal: null, progressbars: 0, connectionDot.role: null`.

### BH-027 · Block editor accessibility: 4 WCAG A violations (2 Critical axe-flagged, 2 Serious)
- **Status:** **FIXED** (2026-04-22, c6-block-editor-a11y, PR #28 merged 5460bdae — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 11, a11y audit — 2 of 4 confirmed by axe-core automated scan as Critical)
- **Severity:** **major** (composite — 2 critical + 2 serious WCAG Level A issues in a primary authoring surface)
- **Iter:** 11 · **Workflow:** block-editor a11y audit
- **Sub-issues (in fix-location order):**
  1. **WCAG 1.1.1 (Level A) — Critical, axe-flagged `image-alt`.** Gallery block images (`templates/partials/blockEditor.tpl:181-183, 197`) render as `<img :src="..." loading="lazy">` with no `alt`. Screen reader users have no information about what's in the gallery.
     - **Fix:** `:alt="getResourceName(resId) || 'Resource ' + resId"`. The `blockGallery` component already fetches resource metadata, so the name is locally available.
  2. **WCAG 4.1.2 (Level A) — Critical, axe-flagged `select-name`.** Heading-block level select (`templates/partials/blockEditor.tpl:112`) has no `id`, no `<label>`, no `aria-label`. Announced as "select, H1, H2, H3" with no purpose context.
     - **Fix:** `<select aria-label="Heading level" x-model.number="level" @change="save()">`.
  3. **WCAG 4.1.2 (Level A) — Serious.** Move-up / Move-down / Delete-block icon buttons (`templates/partials/blockEditor.tpl:37-65`) rely solely on `title=` for name. VoiceOver doesn't announce `title` on focusable elements by default. All 12 control buttons showed `ariaLabel: null` in a live eval.
     - **Fix:** `:aria-label="'Move block ' + (index+1) + ' up'"` etc. Ideally also fire a live-region announcement on reorder — block-editor does NOT use the existing `createLiveRegion` helper (that bulkSelection uses successfully).
  4. **WCAG 4.1.2 (Level A) — Serious.** Add-Block picker (`templates/partials/blockEditor.tpl:862-888`) trigger button has `aria-expanded: null, aria-haspopup: null`. Dropdown container has `role: null`. Screen reader users have no disclosure signal when the picker opens.
     - **Fix:** `:aria-expanded="addBlockPickerOpen.toString()" aria-haspopup="listbox" aria-controls="add-block-listbox"` on the trigger; `role="listbox" aria-label="Block types"` + roving-tabindex + Arrow-key navigation on the container.
- **Non-bug caveat:** the heading block renders all three `<h1>/<h2>/<h3>` DOM elements and hides two via Alpine `x-show` (i.e., `display:none`). Screen readers respect `display:none` and exclude those from the a11y tree, so this is CORRECT in current behavior. Flagged as a fragility risk (if a future refactor adds an `x-transition`, the hidden headings would leak into AT), but NOT an active bug.
- **Evidence:** iter-11 audit findings #1–#4; axe-core scan output `{image-alt: critical, select-name: critical}`; `tasks/bug-hunt-evidence/iter-2026-04-22-6/01-block-editor-edit-mode.png`.

### BH-026 · Download Cockpit panel shows blank title and no download link for completed group-export jobs
- **Status:** **FIXED** (2026-04-22, c5-jobs-ui-a11y, PR #27 merged f60bd9f3 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 10, live-SSE evidence for 5 completed export jobs)
- **Severity:** medium (makes the cockpit — the intended post-reload recovery surface — useless for group exports; user has to know the job ID to download the tar)
- **Iter:** 10 · **Workflow:** operator-migration scenario (paired with BH-025)
- **Observation:** all 5 completed `source=group-export` jobs in the live SSE `init` payload have `url: ""` and the cockpit panel therefore renders them with:
  - a **blank name line** (because `getJobTitle(job)` → `getFilename(job.url)` → `getFilename("")` → `""`, `downloadCockpit.js:338-360`)
  - a green "Completed" badge
  - **no download link** — the template (`templates/partials/downloadCockpit.tpl:156-169`) has two completion-time link branches: `job.resourceId` (resource downloads) and `job._isAction && job.result?.redirect` (plugin actions). Neither matches `source=group-export`.
- **Key data point:** the `resultPath` field (e.g. `_exports/fc399493.tar`) IS present in the SSE event — the info is there, the UI just doesn't use it. A third template branch wiring `source=group-export` + `resultPath` to a `/v1/exports/{jobId}/download` link would close the gap.
- **Fix:**
  1. Extend `getJobTitle()` to fall back to e.g. `job.name`, `job.groupName`, or a "Group export" default when `url === ""`.
  2. Add a template branch in `downloadCockpit.tpl` for `source === 'group-export' && resultPath`: render a download anchor pointing at `/v1/exports/{jobId}/download`.
- **Paired with BH-025:** together these two bugs form a complete UX hole for the export flow — BH-025 blanks the export page on reload, BH-026 blanks the cockpit panel's export entries. Either fix helps; both together close the scenario.

### BH-025 · `adminExport` loses all job tracking on page reload — in-flight exports become invisible, completed exports lose their download link
- **Status:** **FIXED** (2026-04-22, c5-jobs-ui-a11y, PR #27 merged f60bd9f3 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 10, code-confirmed)
- **Severity:** **medium** (real-world operator scenario: start export → switch tabs / come back → page is blank → no way to find the tar from the same UI they kicked it off in)
- **Iter:** 10 · **Workflow:** operator-migration scenario — page reload during / after a long export
- **Root cause (code-verified):**
  - `src/components/adminExport.js:31-42` — `init()` restores only `selectedGroups` from `preselectedIds`. It does NOT subscribe to the jobs SSE stream and does NOT restore `this.job`.
  - `src/components/adminExport.js:113` — `subscribeProgress(jobId)` is only reachable from `submit()` at line 110. Post-submit, `this.job` is set and the progress panel is revealed by `x-show="job"`.
  - After reload, `this.job === null` → progress panel hidden, download link not rendered. The job continues on the server (SSE stream carries it), but the adminExport page doesn't look at the stream.
- **Contrast with `downloadCockpit`:** `downloadCockpit.connect()` IS called unconditionally in its `init()` and the SSE `init` event rehydrates `this.jobs` across the whole panel. The export page's scoped tracker missed this pattern.
- **Fix:** make `adminExport.init()` subscribe to the jobs SSE stream and match any `source=group-export` job against the current user's recent submissions (by job ID in localStorage, or by matching `requestBody.groupIds`) to repopulate `this.job` and show the progress/download panel.
- **Evidence:** confirmed via iter-10 hunter + code citations above.

### BH-024 · `GET /v1/note/block/table/query?blockId=<existing-block-with-dangling-queryId>` returns HTTP 500 (should be 404)
- **Status:** **FIXED** (2026-04-22, c4-deletion-cascade, PR #26 merged 7abe0e77 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 9), **scope narrowed in iter 10** (this is specifically about existing blocks whose `content.queryId` points at a deleted saved query — NOT about missing blocks)
- **Severity:** minor
- **Iter:** 9 discovered · 10 scope corrected
- **Repro:**
  1. Create a table block with `{"queryId":<valid-query-id>}`.
  2. Delete that saved query via `POST /v1/mrql/saved/delete?id=<queryId>`.
  3. `curl -isS 'http://localhost:8181/v1/note/block/table/query?blockId=<EXISTING blockId>'` → `HTTP 500 {"error":"record not found"}`.
- **Not about missing blocks:** iter 10 audit confirmed that `blockId=9999999` (nonexistent block) correctly returns 404 via the global `statusCodeForError` helper. The 500 only appears when the block exists but its referenced query has been deleted.
- **Root cause (refined hypothesis):** the table-block handler fetches the block successfully (block row exists), then does a second fetch for the query via `content.queryId`. That second fetch raises `gorm.ErrRecordNotFound`, which is NOT routed through the app's standard error translator — it falls through to the default 500 branch.
- **Positive: iter 10 audit found ZERO other endpoints with this pattern.** `/v1/resource`, `/v1/note`, `/v1/group`, `/v1/tag`, `/v1/category`, `/v1/query`, `/v1/noteType`, `/v1/mrql/saved/run`, `/v1/resource/version`, `/v1/group/export`, and `/v1/note/block/table/query` (on a missing blockId) all correctly return 404. The standard `statusCodeForError()` helper at `server/api_handlers/error_status.go` maps "record not found" to 404 consistently.
- **Fix:** in the table-block handler, wrap the inner query fetch with `statusCodeForError`, or translate the inner `ErrRecordNotFound` to a 410 Gone / 404 with a clearer message like "referenced query has been deleted". Two-line fix.

### BH-023 · Alternative filesystem feature (`FILE_ALT_*`) is half-implemented — unreachable from UI, silently ignored via multipart API, and stripped by export/import
- **Status:** **FIXED** (2026-04-22, c7-alt-fs, PR #29 merged 8467c32f — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 9, code-confirmed in three layers)
- **Severity:** medium (real data-integrity risk on top of a dead feature — any user relying on alt-fs for archival resources would silently lose the storage binding on re-import)
- **Iter:** 9 · **Workflow:** alt-fs "archival NAS" scenario
- **Three separate defects, one composite bug (because they share a root cause: `StorageLocation` isn't threaded through the stack):**
  1. **UI gap:** `templates/createResource.tpl` has no filesystem/target selector. The only place `altFileSystems` appears user-facing is the read-only `adminOverview.tpl:145-148` panel. Users can't choose where to write.
  2. **Multipart API silently discards the hint:** `POST /v1/resource` with multipart `PathName=some_key` returns HTTP 200 but the stored resource has `StorageLocation: null`. Code: `ResourceCreator` struct (`models/query_models/resource_query.go:22-24`) embeds only `ResourceQueryBase` and has NO `PathName` field. Gorilla/schema decodes only the fields that exist, so `PathName=some_key` is quietly dropped. (The local-upload variant `ResourceFromLocalCreator:26-30` DOES have `PathName`, and `POST /v1/resource/local` with `PathName=some_key` DOES reach the lookup — and then fails because `/some/folder` doesn't exist on this host, which is a separate operational issue, not a bug.)
  3. **Export/import strips the binding:** `archive/manifest.go`'s `ResourcePayload` has no `storage_location` (or `pathName`) field — confirmed by grep. An alt-fs resource exported and re-imported lands on the default filesystem. Users who exported an archival-tier group and re-imported it would silently migrate their archives back to the hot filesystem.
- **Evidence:** `curl /v1/admin/data-stats` confirms `config.altFileSystems: ["some_key"]`; `curl /v1/resource?id=<id>` after a multipart create with `PathName=some_key` shows `StorageLocation: null`; grep of `manifest.go` returns zero hits for `StorageLocation`/`storage_location`/`PathName`.
- **Fix (ordered, each independently shippable):**
  1. **Manifest contract** (fix #3 first — data loss is the worst symptom): add `storage_location` to `ResourcePayload` v1 manifest. Since unknown top-level keys are already silently ignored per contract, this is forward-compatible. Exporter sets it; importer restores it when present; absence → default fs (current behaviour).
  2. **Multipart field:** add `PathName` to `ResourceQueryBase` (or to `ResourceCreator`). Wire it through `AddResource` so the server writes to the target fs.
  3. **UI:** add a "Storage" dropdown on `createResource.tpl` populated from `config.altFileSystems`.
- **Scope:** if alt-fs is actually considered a documented feature, all three should land together. If it's considered aspirational, at minimum the docs and admin-overview panel should stop advertising it.

### BH-022 · OpenAPI spec (`cmd/openapi-gen`) omits 11 live routes — the entire MRQL subsystem, `editMeta` endpoints, and part of plugins
- **Status:** verified (iter 8)
- **Severity:** minor / docs (but high-impact for any integrator consuming the spec — they'd have no idea MRQL or the per-entity `editMeta` endpoints exist)
- **Iter:** 8 · **Workflow:** OpenAPI spec drift probe
- **Counts:** 167 live routes in `server/routes.go`; 156 in generated `openapi.yaml`. Diff of in-code minus in-spec (routes that exist but aren't documented):
  1. `/v1/mrql` (query endpoint)
  2. `/v1/mrql/complete` (autocomplete)
  3. `/v1/mrql/saved` (list/create/update)
  4. `/v1/mrql/saved/delete`
  5. `/v1/mrql/saved/run`
  6. `/v1/mrql/validate`
  7. `/v1/group/editMeta`
  8. `/v1/note/editMeta`
  9. `/v1/resource/editMeta`
  10. `/v1/plugins/`
  11. `/v1/plugins/{pluginName}/display/render`
- **Positive:** the reverse diff is **empty** — no phantom routes documented that don't exist. Generator is conservative, not hallucinatory.
- **Spec generator status:** passes the bundled `cmd/openapi-gen/validate.go` as a valid OpenAPI 3.0 spec (156 paths, 80 schemas, 20 tags).
- **Root cause (hypothesis):** `cmd/openapi-gen` likely walks a route registry that was wired up entity-by-entity; the MRQL subsystem and the per-entity `editMeta` shortcut endpoints were added later and never registered with the generator.
- **Fix:** wire up the missing route handlers in whatever registry `cmd/openapi-gen` reads. The MRQL routes at `server/routes.go:495-501` are the obvious omission — 6 of 11 missing routes are in that block.

### BH-021 · Block-editor text rendering only recognizes `**bold**` and `*italic*` — `_italic_` and backtick inline code render as literal characters
- **Status:** verified (iter 8 observation, code trivially confirmable via `renderMarkdown` in `src/components/blockEditor.js` or adjacent)
- **Severity:** minor (expectation mismatch: both `_italic_` and `` `code` `` are ubiquitous in Markdown / GFM; users who type them will be confused when they render as `_italic_` / `` `code` ``)
- **Iter:** 8 · **Workflow:** daily-journal block editor
- **Fix:** either expand `renderMarkdown` to cover the full "safe subset" of common Markdown (`_italic_`, `` `code` ``, `~~strike~~`, headings, unordered list) OR add a small help tooltip/placeholder documenting exactly which syntax is supported, so users don't type what won't render.
- **Note:** this is NOT about adding a full Markdown parser — just matching user expectations for the tokens the app claims to support (the NoteText handler already uses `markdown2` per `displayNoteText.tpl:10`, so there's a precedent for real Markdown elsewhere).

### BH-020 · Deleting any referenced entity does not scrub block content — 4 block types keep dead pointers (systemic)
- **Status:** **FIXED** (2026-04-22, c4-deletion-cascade, PR #26 merged 7abe0e77 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 8 gallery; iter 9 scope expanded to references / calendar / table)
- **Severity:** minor individually; medium in aggregate (data-integrity issue that compounds silently across the entire block-editor surface)
- **Iter:** 8 (gallery) + 9 (systemic audit)
- **Affected block types and what they embed:**
  | Block type | Embedded IDs | Status after target deletion |
  |---|---|---|
  | `gallery` | `resourceIds[]` | stale IDs remain; view renders broken `<img>` + 2 console 404s |
  | `references` | `groupIds[]` | stale IDs remain; view would render dead group links |
  | `calendar` | `calendars[].source.resourceId` (type=resource) | stale IDs remain; endpoint returns per-calendar error "failed to get resource: record not found" — graceful at runtime, permanently broken in storage |
  | `table` | `queryId` | stale IDs remain; endpoint returns HTTP 500 (should be 404) — see BH-024 |
  | `text` / `heading` / `divider` / `todos` | none | not affected |
  | `plugin` | delegates to plugin renderer; not audited |
- **Orchestrator re-confirmed (iter 9):** `curl '/v1/note/blocks?noteId=276'` shows all 4 dangling blocks after the respective target deletions — block 21 (gallery→r112), 22 (references→g1013), 23 (calendar→r112), 24 (table→q239).
- **Root cause:** none of the DELETE handlers (`/v1/resource/delete`, `/v1/group/delete`, `/v1/mrql/saved/delete`) cascade into `note_blocks.content`. The JSON blob is opaque to the ORM; no DB-level FK; no explicit cleanup step anywhere.
- **Fix (layered — all three together form the clean story):**
  1. **Storage-side:** on entity delete, walk `note_blocks` and scrub matching IDs from `content.resourceIds` / `content.groupIds` / `content.calendars[].source.resourceId` / `content.queryId`. SQLite: `json_each` + `json_remove`; Postgres: `jsonb_array_elements`. Add a one-shot migration to clean existing orphans.
  2. **UI-side:** each block component should gracefully render "Resource unavailable" (or equivalent) on 404 from its metadata fetch. `calendar` already does this per-calendar; `gallery`/`references`/`table` don't.
  3. **Detection:** surface a "dangling references" counter in `/admin/data-stats` so operators can see the scale.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-22-3/dangling-resource-gallery-broken.png`; iter-9 full block-type audit above.

### BH-P05 · `.json` error responses leak full server config (`Config.DbDsn`, `FfmpegPath`, paths, timeouts, worker config, `_requestContext`)
- **Status:** **FIXED** (2026-04-22, PR #23, merged 0aa5d39e — `_appContext` + `_requestContext` added to JSON discard list in `render_template.go`)
- **Severity:** **major** (unauthenticated info-disclosure on any error path that a template router renders as `.json`; 1214-byte payload per response)
- **Iter:** legacy (3+ weeks) → confirmed still present iter 7
- **Repro:** `curl -isS 'http://localhost:8181/resource.json?id=abc'`. HTTP 400 body contains:
  - `_appContext.Config.*`: `DbType`, `DbDsn: "test.db"`, `DbReadOnlyDsn`, `FileSavePath: "./files"`, `FfmpegPath: "/opt/homebrew/bin/ffmpeg"`, `LibreOfficePath`, `BindAddress: "localhost:8181"`, `SharePort`, `ShareBindAddress`, `AltFileSystems: {"some_key":"/some/folder"}`, all remote timeouts, all hash worker config, `MaxImportSize`, `PluginPath`, `DefaultResourceCategoryID`.
  - `_requestContext.Context.Context.…` (7 levels of nested Go context — structurally harmless but noise).
  - The actual user-facing error is there too: `"errorMessage":"invalid value for \"id\": must be a valid number"`.
- **Mitigating context:** CLAUDE.md explicitly scopes Mahresources to "private networks only — no authentication". Genuine threat model is narrow. But: (a) the config nonetheless accelerates post-compromise recon; (b) `AltFileSystems` leaks filesystem paths outside the canonical store; (c) any future misconfig exposing `:8181` to the open internet becomes a one-probe config dump; (d) this is the only endpoint in the app that leaks this much meta-state.
- **Same pattern reproduces on:** `/note.json?id=<anything>`, `/group.json?id=<anything>`, `/resource.json?id=<anything>` — and likely every route that falls through to the template error renderer with a `.json` suffix.
- **Root cause (hypothesis, needs confirmation):** the JSON template renderer is passed the full Pongo2 template context (`_appContext`, `_requestContext`, `errorMessage`, `pageTitle`, plus navigation state `adminMenu` in some variants) and serializes it all. The fix is to have `.json` error responses return only `{"error": "...", "status": N}` — no internal context.
- **Fix:** a dedicated `renderJSONError(w, status, msg)` helper used by every `.json` error path, replacing whatever currently serializes the whole template context.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-22-2/bhp05-resource-json-error.json`, `bhp05-note-json-error.json`, `bhp05-group-json-error.json`; inline curl in iter 7 log.

### BH-019 · Entity names accept null bytes, RTL override, and embedded newlines without sanitization
- **Status:** **FIXED** (2026-04-22, PR #23, merged 0aa5d39e — `SanitizeEntityName` helper wired into all entity create/update paths)
- **Severity:** medium (no SQL injection — GORM parameterizes — but UI spoofing, display integrity, interop, and log/CSV breakage are real)
- **Iter:** 7 · **Workflow:** API fuzz B4 (control-char injection)
- **Repros:**
  - `POST /v1/tag name=bh7-nul%00byte` → `HTTP 200`, tag ID 30 persists with a literal `\x00` in its name (Python's `json` decoder throws `JSONDecodeError` on the `/v1/tags` list response; Go's `encoding/json` survives it as ` `).
  - `POST /v1/tag name=bh7-%E2%80%8D%E2%80%AE` (ZWJ + RTL override U+202E) → `HTTP 200`, tag ID 28 saved. Any UI rendering the name will visually reverse subsequent text — a classic spoofing vector if displayed near user-generated content.
  - `POST /v1/tag` with raw `\n` bytes in `name` → `HTTP 200`, tag ID 29 persists with literal newlines, breaking single-line CSV exports, log lines, search snippets.
- **Why it matters:** the same endpoints take names that flow into page titles, link labels, autocomplete menus, group-export manifests, and possibly filenames on disk. Null byte is the most dangerous — C-based libraries (including some of what Go shells out to, e.g. ffmpeg/libreoffice for thumbnails) will truncate at the null.
- **Fix:** on create/update of any human-named entity (`tag.Name`, `group.Name`, `note.Name`, `resource.Name`, `noteType.Name`, `category.Name`), strip / reject:
  - any `\x00` byte
  - any Unicode C0 control char except `\t`
  - any Unicode directional override (`U+202A`–`U+202E`, `U+2066`–`U+2069`)
  - embedded newlines/CRs (or collapse them to spaces)
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-22-2/b4-control-char-tags.json`.

### BH-018 · Perceptual-similarity false positives on uniform/solid-color images — `DifferenceHash` returns 0 for all of them, AHash is computed but unused
- **Status:** **FIXED** (2026-04-22, c3-image-hashing, PR #25 merged 5e24866f — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 6, code-confirmed)
- **Severity:** medium (real correctness bug in a user-visible feature — "Similar Resources" panel and "Merge Others To This" bulk action both surface the false positives)
- **Iter:** 6 · **Workflow:** perceptual-hash / near-dup detection probe
- **Observation:** resources 111 (lightblue 300×300 PNG) and 112 (lightblue variant) were expected to match. Instead, 112 ended up listed as similar to 113 (orange 300×300 PNG) — a completely different color. DB shows Hamming distance 0 for both pairs, and in fact for every pair of solid-color images uploaded.
- **Root cause (code-verified):**
  - `hash_worker/worker.go:392-393`: both `imgsim.AverageHash(img)` (AHash) and `imgsim.DifferenceHash(img)` (DHash) are computed.
  - `:240-241`: both are persisted into `image_hashes.a_hash_int` / `d_hash_int`.
  - `:423`: `findAndStoreSimilarities(resource.ID, dHashInt)` passes **only** DHash.
  - `:431` `findAndStoreSimilarities` compares against `d_hash_int` exclusively; Hamming distance stored on `resource_similarities.hamming_distance`.
  - `imgsim.DifferenceHash` works on adjacent-pixel *differences*. For a uniform image, every adjacent-pixel difference is zero → DHash is `0x0000000000000000` for any solid color. All solid-color images collide at Hamming distance 0 regardless of the actual color.
  - AHash, which encodes average brightness, would trivially distinguish orange from lightblue — but is never consulted.
- **Why this escapes unit tests:** most test images are photos or gradients where DHash works fine. The failure mode is specific to uniform / near-uniform images, which *do* appear in real corpora (blank scans, error placeholders, screenshots of plain UI).
- **Fix (pick one, in order of ambition):**
  1. Add an AHash-based secondary check: when DHash Hamming distance ≤ threshold, require AHash Hamming distance ≤ a separate threshold too. Won't regress true positives on real photos; kills the solid-color false positive class.
  2. Short-circuit at hash time: if DHash is exactly `0`, flag the image as "no perceptual signal" and exclude it from the similarity index entirely.
  3. Combine AHash and DHash into a single composite distance (weighted sum or max).
- **User-facing consequence:** "Merge Others To This" bulk action is dangerous in the current state — a user could delete/merge totally different images together, in a destructive op that isn't reversible.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-22-1/resource-dup-b-similar-section.png`; code citations above.

### BH-017 · Missing `schema_version` in a group-import manifest produces the misleading error "unsupported schema_version 0"
- **Status:** **FIXED** (2026-04-22, c11-import-ux, PR #36 merged 06bc2b20 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 5)
- **Severity:** cosmetic (correctness is fine — the import IS rejected; but the message is confusing)
- **Iter:** 5 · **Workflow:** group import manifest contract probe
- **Repro:** remove `schema_version` entirely from a valid manifest.json inside the export tar, re-tar, import. Error surfaced: `archive: unsupported schema_version 0 (supported: [1])`.
- **Root cause:** Go's default for an absent `int` JSON field is `0`, and the reader dispatches on that as "unsupported version". A separate "missing required field" branch would tell the user something actionable.
- **Fix:** at manifest parse time, distinguish "field absent" (use a `*int` or a presence flag) from "field == 0"; emit "manifest is missing required field `schema_version`" for the former.

### BH-016 · Import result UI hides GUID-reused AND GUID-merged entities — shows "0 created" when the import actually re-linked or merged existing ones
- **Status:** **FIXED** (2026-04-22, c11-import-ux, PR #36 merged 06bc2b20 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 5), scope confirmed broader in iter 6 (merge path also silent)
- **Severity:** minor (misleading feedback; no data loss)
- **Iter:** 5 (re-link path) + 6 (merge path)
- **Observation (iter 5, re-link path):** exported a group, deleted it (but not its notes/resources — BH-014's orphan semantics), re-imported the tar. Result: `Groups created: 3, Resources created: 0, Notes created: 0`. The 2 notes + 2 resources were re-linked by GUID but that's invisible.
- **Observation (iter 6, merge path):** exported a group, kept the source, re-imported with default policy `merge`. UI correctly warns "1 entities match by GUID". Import succeeds — but counters show `Groups created: 0, Resources created: 0, Notes created: 0` and message "Import completed." The user can't tell whether the merge did anything.
- **Root cause (code-verified):** `application_context/import_plan.go:154-188` — `ImportApplyResult` exposes `CreatedResources` / `CreatedNotes` only; no `MergedGroups`, `MergedResources`, `MergedNotes`, or `LinkedByGUID` counters. `templates/adminImport.tpl:384-406` binds only the "created" counters.
- **Fix:** extend `ImportApplyResult` with `MergedGroups`, `MergedResources`, `MergedNotes`, `LinkedByGUID*`, `SkippedByPolicy*` counters; surface them in the template next to "created", e.g. "2 created, 1 merged, 0 skipped, 2 re-linked".
- **Connected to:** BH-014 — if the parent-delete UX were clearer, users would less often hit the re-link path, but the counter gap is independent.

### BH-015 · Export progress percentage overflows 100% for small-payload exports (UI shows 5140%)
- **Status:** **FIXED** (2026-04-22, c10-jobs-ui-polish, PR #35 merged dd2c68b2 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 5, code-confirmed)
- **Severity:** minor cosmetic (no data impact, but "Export complete (5140%)" looks broken and erodes trust)
- **Iter:** 5 · **Workflow:** export a group with 2 × 176-byte image resources
- **Observed:** jobs stream reports `{"progressPercent": 5140.909, "totalSize": 352, "progress": 18096, "source": "group-export"}` — because `totalSize` counts only unique resource blob bytes (352 = 2 × 176) while `progress` counts every byte written to the tar (manifest.json, group/note/resource/schema JSONs, tar block padding — 18 KB total). The ratio blows past 100%.
- **Root cause (code-verified):**
  - `application_context/export_context.go:175-180` — `plan.totalBytes += r.FileSize` only accumulates unique blob file sizes. Metadata overhead is never estimated.
  - `src/components/downloadCockpit.js:297` — `getProgressPercent()` uses `Math.min(100, ...)` (used for the progress bar).
  - `src/components/downloadCockpit.js:268` — `formatProgress()` uses `job.progressPercent.toFixed(1)` **uncapped** (used for the text label).
  - `templates/adminExport.tpl:122` — `Math.round(job?.progressPercent || 0)` **uncapped** (the "(N%)" badge).
  - So the bar caps visually at 100% but the adjacent text says 5140%. The two UI surfaces contradict each other.
- **Fix (pick both):**
  1. (UI, one-liner) cap the display: `Math.min(100, Math.round(job?.progressPercent || 0))` in both `adminExport.tpl:122` and `downloadCockpit.js:268`.
  2. (Backend, preferred long-term) include an estimate of JSON/metadata bytes in `totalBytes` so the percentage is actually accurate, not just clamped. Rough sketch: add a per-entity "JSON overhead" constant (e.g. 1KB) × count of groups+notes+resources+schemas, plus the `manifest.json` size.
- **Evidence:** jobs API response above; code citations above.

### BH-014 · Deleting a parent group silently orphans its children (no warning, no choice)
- **Status:** verified (iter 4)
- **Severity:** minor UX (destructive operation with no confirmation about consequences)
- **Iter:** 4 · **Workflow:** investigation / group hierarchy
- **Observation:** deleting `Inv-Top` (id=1006) with two child groups `Inv-Sources` (id=1007) and `Inv-Docs` (id=1008) left the children alive with `OwnerId=null`. No dialog warned the user this would happen; the orphaning was silent.
- **Why it matters:** the user's mental model of "this is a container for my investigation" is violated with no feedback. If they don't then inspect `/groups` with the right filter, the ex-children look indistinguishable from top-level groups.
- **Fix options (pick one, or combine):**
  1. Confirm dialog on parent-group delete: "This group contains 2 child groups and N notes/resources. Deleting will orphan them. Continue?".
  2. Block the delete if the group has children, requiring the user to re-home or delete them first.
  3. Offer a choice at delete time: "Orphan children" | "Delete children recursively" | "Cancel".
- **Evidence:** after-delete `curl /v1/group?id=1007` and `/1008` showed `"OwnerId":null`; no UI warning seen during delete.
- **Scope note:** this is not a regression from cycle-prevention code — the backend's parent-delete semantics are orphan-by-default. Bug is the lack of user-facing disclosure, not the storage behavior.

### BH-013 · MRQL results page has no default LIMIT and no pagination — all rows render into the DOM at once
- **Status:** **FIXED** (2026-04-22, c12-mrql-polish, PR #37 merged d9f7ed77 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 4)
- **Severity:** minor (performance / usability; could become major in deployments with millions of rows, which CLAUDE.md explicitly calls out as a target)
- **Iter:** 4 · **Workflow:** saved-query flow
- **Repro:** `/mrql` → run `type = note` (no LIMIT clause). 264 notes render into a single scrollable list (`document.querySelectorAll('[href^="/note"]').length === 268` via eval). No paginator, no "load more", no warning banner about the result size.
- **Expected:** server-side default LIMIT (e.g. 200 or 500) with a visible hint "Showing first N — add LIMIT / OFFSET to page further", or a client-side paginator on the MRQL result container.
- **Actual:** the full set is streamed + rendered. Fine today with tens of entities; catastrophic on the "millions of resources" deployment profile from CLAUDE.md.
- **Fix (pick one):**
  1. In `application_context/mrql_context.go` (or the query runner), inject a default `LIMIT 500` when the parsed MRQL has no `LIMIT`. Surface a banner "Default limit applied (500 rows) — add an explicit LIMIT to see more".
  2. Paginate client-side in the MRQL result container.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-4/06-mrql-264-results.png`; DOM count via eval.

### BH-012 · Saved MRQL queries cannot be updated in place — only create + delete are wired in the UI
- **Status:** **FIXED** (2026-04-22, c12-mrql-polish, PR #37 merged d9f7ed77 — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 4, code confirmed)
- **Severity:** minor feature-gap (forces awkward delete-and-recreate workflow, and an entire backend endpoint goes unused)
- **Iter:** 4 · **Workflow:** saved-query flow — modify an existing query
- **Repro:** save a query, reload it into the editor via the Saved panel, edit the MRQL text, click Save → the Save dialog opens with an EMPTY Name field, treating this as a new save. There is no "Update" affordance.
- **Root cause (code-verified):**
  - `server/routes.go:499` — `PUT /v1/mrql/saved` is registered and points at `GetUpdateSavedMRQLQueryHandler`.
  - `src/components/mrqlEditor.js:370` — the editor's `save()` path is `fetch('/v1/mrql/saved', { method: 'POST', ... })`, unconditionally. No branch ever calls PUT.
- **Fix:** track the currently-loaded saved-query ID in `mrqlEditor` state. If the user loaded an existing query and hasn't changed the name, offer "Update" (PUT) as the default action alongside "Save as new" (POST). If the name is changed, fall back to POST create.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-4/16-mrql-no-edit-mode.png`; `mrqlEditor.js:370`, `routes.go:499`.

### BH-011 · Image ingestion accepts invalid / truncated uploads and silently stores them with `Width=0, Height=0`
- **Status:** **FIXED** (2026-04-22, c3-image-hashing, PR #25 merged 5e24866f — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 3)
- **Severity:** **major** (data integrity — any partial upload survives; cascades to BH-008 and to broken thumbnails, broken perceptual-hash, broken crop, broken `.body` gallery views)
- **Iter:** 3 · **Workflow:** truncated-PNG probe
- **Repro steps:**
  1. `convert -size 200x200 xc:purple /tmp/bh3-good.png` (or `magick …`)
  2. `head -c 400 /tmp/bh3-good.png > /tmp/bh3-truncated.png` — real PNG header, no IDAT/IEND
  3. Upload `/tmp/bh3-truncated.png` through `/resource/new`
  4. `curl 'http://localhost:8181/v1/resource?id=107' -H 'Accept: application/json'` → `{"ID":107,"Name":"bh3-truncated.png","Width":0,"Height":0,"ContentType":"image/png","Size":400}`
- **Actual:** no error, no warning, stored as a first-class image resource.
- **Expected:** ingestion should either (a) refuse the upload if `image.Decode()` fails/returns zero dims, (b) accept it but mark it as a "raw file" (non-image ContentType), or (c) at minimum flag the resource with a "decode failed" annotation the UI can surface. Right now the user has no idea anything went wrong until they try to crop/thumbnail/compare.
- **Root cause (likely — confirm before fixing):** image resource ingestion in `application_context/resource_media_context.go` (and/or the upload handler) sets `ContentType=image/png` from the `Content-Type` header, then calls into a dimension extractor that fails silently and leaves `Width=Height=0`. The "decode failed" path needs to become a hard rejection or a visible resource-level warning.
- **Fix (sketch):**
  1. In the Go image ingestion flow, if `image.Decode` returns an error OR the decoded bounds have `Dx()==0 || Dy()==0`, reject the upload with a 400 `"Uploaded file is not a valid image (failed to decode)"`.
  2. Backfill audit: scan for existing `ContentType LIKE 'image/%' AND (Width=0 OR Height=0)` — resources 87 and 107 are two we already know about; production may have more.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-3/truncated-png-accepted.png`; API JSON above.
- **Relationship:** BH-008 is the client-side consequence (crop overlay hidden). BH-011 is the upstream ingestion gap. Fixing BH-011 prevents future BH-008 occurrences but does not retroactively clean existing bad rows.

### BH-010 · Schema-editor "Preview Form" seeds numeric fields with `0` instead of empty, producing a bogus range error
- **Status:** verified (iter 3)
- **Severity:** minor (preview-only UX; does NOT affect the real note-create form per the hunter's separate workflow-A run)
- **Iter:** 3 · **Workflow:** NoteType MetaSchema authoring (Visual Editor → Preview Form tab)
- **Repro steps:**
  1. Create a NoteType with a numeric field `year` constrained `min=1900, max=2100`.
  2. Open the Visual Editor, click the Preview Form tab.
  3. Observe `#field-year` — value is `"0"`.
  4. Focus then blur the field — the onBlur validator fires "Must be at least 1900" even though the user typed nothing.
- **Root cause (code-verified):** in `src/schema-editor/modes/form-mode.ts` `_renderNumberInput` the template reads `.value=${data !== undefined && data !== null ? String(data) : ''}`. The preview must be seeding `data` as `0` (probably a type-coerced `number` default in the preview harness) instead of `undefined`.
- **Fix:** make the preview harness pass `undefined` (or omit the key) when the schema has no explicit `default`; also consider defensively treating `data === 0 && !('default' in schema)` as empty at the renderer level.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-3/preview-form-year-zero.png`.

### BH-009 · Schema-editor form-mode: `required` and `pattern` violations never show an error message (silent submit-blocked)
- **Status:** **FIXED** (2026-04-22, c2-form-ux, PR #24 merged 7b7e9fee — see Fixed / closed table below)
- **Original status (pre-fix):** verified (iter 3, merges hunter findings F1+F2 — same fix location)
- **Severity:** **major** (user clicks Save on a form with many fields and gets no feedback — just a non-responsive button; catastrophic for discoverability on any non-trivial MetaSchema)
- **Iter:** 3 · **Workflow:** NoteType MetaSchema authoring → create a note with required+pattern fields
- **Repro steps:**
  1. Create a NoteType with a required `title` string and a `doi` string with `pattern: "^10\\..*"`.
  2. At `/note/new`, select that NoteType.
  3. Leave `title` blank (or set `doi = "not a doi"`), fill the rest legitimately, click Save.
- **Actual:** native form validation blocks the submit (browser may flash a tooltip), but the custom `#field-<name>-error` span stays empty and `aria-invalid` is not set. User sees Save "do nothing" with no in-page explanation.
- **Root cause (code-verified):** `src/schema-editor/modes/form-mode.ts:1010-1034` — `_renderStringInput.onBlur` and `_renderNumberInput.onBlur` only branch on `schema.minLength`/`maxLength` (strings) and `schema.minimum`/`maximum` (numbers). `input.validity.valueMissing` and `input.validity.patternMismatch` (plus `typeMismatch`, `tooShort`, `tooLong`, `stepMismatch`) are never consulted, so the error span stays empty for those cases.
- **Fix (single location, both findings):**
  ```ts
  // Inside onBlur, after the existing min/max checks:
  if (!error && input.validity.valueMissing) {
    error = 'This field is required';
  } else if (!error && input.validity.patternMismatch) {
    error = schema.patternDescription
      || `Must match the expected format` + (schema.pattern ? ` (${schema.pattern})` : '');
  } else if (!error && input.validity.typeMismatch) {
    error = `Invalid ${inputType} value`;
  }
  ```
  Also hook the same routine into `form.addEventListener('submit', ...)` so errors surface on Save, not only on blur (user may never blur a blank field).
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-3/note-blank-title-save.png`; eval results confirming empty error spans with `validityMessage` populated.

### BH-008 · Crop selection overlay is invisible when image metadata has Width=0 / Height=0
- **Status:** verified
- **Severity:** minor (functional: user can submit a crop with no visual feedback and get a server-side error; also masks genuinely broken images behind a useless modal)
- **Iter:** 2 · **Workflow:** photo-archive (crop modal)
- **Root cause (code-verified):**
  - `src/components/imageCropper.js:159` — `if (!rect || !this.naturalW || !this.naturalH) return 'display: none';` hides the selection overlay whenever the img hasn't reported a natural size. If the stored DB `Width`/`Height` are `0` AND the browser can't decode the image (e.g. truncated bytes → `img.onload` never fires or `naturalWidth === 0`), the overlay never appears, yet the Crop button stays enabled because the `hasSelection()` check (`:171-174`) only looks at `this.rect`.
  - Confirmed against resource `id=87` (`Action Resource 1773602392573`): `curl '.../v1/resource?id=87'` → `{"Width":0,"Height":0,"ContentType":"image/png"}`; POST to `/v1/resources/crop` returns `400 {"error":"image cannot be cropped: decode failed (source 1499 bytes, ...): unexpected EOF"}`.
- **Follow-on question (do NOT treat as separate bug yet):** resource 87 sits in the DB with `Width=0, Height=0` and truncated bytes. That's most likely stale E2E-fixture data, not a live ingestion bug — but if it's reproducible through the UI upload path, it deserves its own entry. Next iteration could try uploading a deliberately truncated PNG and see if the ingestion still succeeds with `Width=0`.
- **Fix:**
  1. In `submit()` / the Crop button's `:disabled`, also require `this.naturalW > 0 && this.naturalH > 0`.
  2. When `img.onerror` fires OR natural dimensions stay zero after load, show a non-dismissable banner in the modal: "This image could not be decoded; cropping is unavailable." — the user should never be left staring at a blank crop area without feedback.
- **Evidence:** `imageCropper.js:158-167`; API response above.

### BH-007 · Version-compare action bar: "Upload New Version" label wraps to three lines when "Compare Selected" link appears
- **Status:** **FIXED** (2026-04-22, c13-cosmetic-cleanup, PR #32 merged fec44787) — see Fixed / closed table below
- **Original status (pre-fix):** verified (template + screenshot)
- **Severity:** minor cosmetic / usability (makes the primary "Upload" CTA look broken)
- **Iter:** 2 · **Workflow:** photo-archive (resource → Versions panel → Compare)
- **Root cause (template-verified):**
  - `templates/partials/versionPanel.tpl:54-78` — a `flex items-center justify-between` container holds the Cancel/Compare toggle button, a conditional `<template x-if="compareMode && selected.length === 2">…Compare Selected…</template>`, **and** the inline `<form>` with file input + comment input + submit button. Designed for 2 children; becomes cramped when the third appears, and the rightmost button text ("Upload New Version") gets the squeezed-out space.
- **Fix:** stack the upload form on a second row (`flex-col sm:flex-row gap-2`, or wrap the compare controls separately), or change the button to `whitespace-nowrap` and let the form shrink instead.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-2/21-version-compare-layout-bug.png`; template lines 54-78.

### BH-006 · **Systemic** form-data-loss: every native create/edit-form blows up to a bare error page on server-side validation failure
- **Status:** **FIXED** (2026-04-22, c2-form-ux, PR #24 merged 7b7e9fee — see Fixed / closed table below)
- **Original status (pre-fix):** verified, scope expanded twice (iter 2: resource-only → iter 3: all 5 create flows → iter 4: edit flows too)
- **Severity:** **major** (real user-facing data loss on a routine error path — replicated across 6+ entity endpoints, both create and edit)
- **Iter:** 2 (discovered on resource) + 3 (confirmed systemic on creates) + 4 (confirmed also affects edit forms — group cycle case)
- **Reproduced endpoints:**
  - `/resource/new` with remote URL 404 → `/v1/resource` Error 400 "remote URL returned HTTP 404"
  - `/group/new` with invalid `OwnerId=99999999` → `/v1/group` Error 400 "owner group not found"
  - `/note/new` with invalid `NoteTypeId=99999999` → `/v1/note` Error 400 "note type not found"
  - `/category/new` with duplicate name → `/v1/category` Error 400 "a category with that name already exists"
  - `/tag/new` with duplicate name → `/v1/tag` Error 400 "a tag named '…' already exists"
  - **iter 4: `/group/edit?id=…` with cycle or self-loop owner → `/v1/group` Error 400 "setting this owner would create an ownership cycle" / "a group cannot be its own owner"** (evidence `tasks/bug-hunt-evidence/iter-2026-04-21-4/07-cycle-error.png`)
- **Root cause:** each create-form posts **natively** to `/v1/<entity>`. The handlers return a generic error page rendered at that URL instead of (a) redirecting back to the form with an error query param, or (b) being replaced with a JS-intercepted async submit that keeps the user on the form and shows an inline error.
- **Why it matters now:** this isn't just a paper cut — on a note with a rich body and custom MetaSchema fields the user can have typed for minutes. One typo in an ID, duplicate name, or a transient upstream hiccup, and everything is gone with a "Go back" link that re-opens a blank form.
- **Fix (prefer (2) — the audit is already done):**
  1. Each entity's create handler in `server/api_handlers/` (plus `template_handlers/`) should, when the `Accept` header is HTML-ish, redirect back to `/<entity>/new` with either (a) session-flash of the submitted form body + error, or (b) form values re-encoded as query params and a distinct `error=` param. On JSON requests, keep the current JSON 400.
  2. Alternatively, convert all create-forms to intercepted async submit (Alpine `@submit.prevent` + `fetch`) so the page never navigates; `bulkSelection.js:171-192` is a working precedent for that pattern.
  3. A shared redirect-with-form-values helper in Go is the least-invasive global fix.
- **Evidence:** iter-2 `tasks/bug-hunt-evidence/iter-2026-04-21-2/16-remote-404-error.png`; iter-3 `tasks/bug-hunt-evidence/iter-2026-04-21-3/group-form-loss.png`; error bodies above.

### BH-001 · Duplicate "META DATA" heading on tag and note-text pages
- **Status:** **FIXED** (2026-04-22, c13-cosmetic-cleanup, PR #32 merged fec44787) — see Fixed / closed table below
- **Original status (pre-fix):** verified
- **Severity:** minor (cosmetic, but visible on every tag page and every note text view)
- **Iter:** 1 · **Workflow:** recipe-collection (tag detail page)
- **Root cause (code-verified):**
  - `templates/displayTag.tpl:28-29` includes `partials/sideTitle.tpl` with `title="Meta Data"`, then `partials/json.tpl`. The `json.tpl` partial itself renders `<h2 class="sidebar-group-title">Meta Data</h2>` at `partials/json.tpl:16`, so both headings stack.
  - Same anti-pattern at `templates/displayNoteText.tpl:54-55`.
  - `displayResource.tpl`, `displayNote.tpl`, and `displayGroup.tpl` do NOT have this double include — they call `json.tpl` alone.
- **Fix:** drop the `{% include "/partials/sideTitle.tpl" with title="Meta Data" %}` line from `displayTag.tpl` and `displayNoteText.tpl`. `json.tpl` already owns the heading.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-1/22-tag-page-null-meta-error.png`, `.../21-notetext-double-metadata.png`

### BH-002 · `renderJsonTable(null)` throws on entities with no Meta (console pollutes, empty sidebar stays empty silently)
- **Status:** **FIXED** (2026-04-22, c13-cosmetic-cleanup, PR #32 merged fec44787) — see Fixed / closed table below
- **Original status (pre-fix):** verified
- **Severity:** minor (silent-ish: UI still loads, but every such page logs an Alpine error and `metaTableInner` ends up with zero children)
- **Iter:** 1 · **Workflow:** recipe-collection (freshly created tag with no Meta)
- **Root cause (code-verified):**
  - `templates/partials/json.tpl:33` is `x-init="$el.appendChild(renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData))"` — no guard for empty/null `jsonData`.
  - `src/tableMaker.js:3` (`renderJsonTable`) returns a primitive string for null/undefined input (falls through the Array / Date / object branches and the primitive branches). `appendChild(string)` throws `TypeError: parameter 1 is not of type 'Node'`.
- **Fix options (pick one):**
  1. Guard at the template: `x-init="jsonData && $el.appendChild(renderJsonTable(...))"`.
  2. Make `renderJsonTable` return a `DocumentFragment` / empty `HTMLElement` instead of a string when input is null/undefined — safer, also fixes the recursion paths that cast results via `typeof content === "string"` (e.g. `tableMaker.js:262`).
  - Option 2 is the robust fix; option 1 is a one-line band-aid.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-1/13-tag-page-console-error.png`

### BH-003 · Resource thumbnail checkbox / lightbox hit-target conflict
- **Status:** **closed — not reproduced** (iter 2)
- **Severity:** unclear — major if real, non-bug if hunter clicked the image area by accident
- **Iter:** 1 · **Workflow:** recipe-collection (bulk select flow)
- **Claim:** clicking the checkbox region of a resource card that has a large image thumbnail opens the lightbox; after closing, the lightbox overlay keeps intercepting subsequent checkbox clicks.
- **Why I'm not cementing it yet:** the hunter's repro says they used a Playwright ref (`e154`) from a `snapshot`; the snapshot's "checkbox" ref could easily have been the surrounding image anchor. Screenshot shows the lightbox open but doesn't prove it was reached via the checkbox input. Needs a targeted repro that (a) tabs to the checkbox by role and (b) clicks purely by bounding box.
- **Iter-2 verdict:** clicking the checkbox by bounding-box coordinates selects the card cleanly (store goes 0→1, no lightbox). Evaluated `getComputedStyle(document.querySelector('.overlays')).pointerEvents` — returns `none` in both closed- and open-lightbox states. Iter-1 hunter almost certainly clicked the image/anchor, not the checkbox input.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-2/13-checkbox-click-result.png`, `.../14-image-click-lightbox.png`

### BH-004 · Bulk selection "keyboard Space doesn't register" claim
- **Status:** **closed — not reproduced** (iter 2)
- **Severity:** would be major (keyboard a11y) IF real, but evidence suggests not real
- **Iter:** 1 · **Workflow:** recipe-collection (bulk select keyboard path)
- **Claim:** Space on a focused resource checkbox does not update `$store.bulkSelection.selectedIds`; the bulk action toolbar stays hidden.
- **Why it's probably not a real bug:**
  - `src/components/bulkSelection.js:237-242` explicitly binds `@keydown.space.prevent` and `@keydown.enter.prevent` → `toggle(itemId)`. That binding is on the checkbox itself via `x-bind="events"` (see `templates/partials/resource.tpl:3`, `templates/partials/note.tpl:3`, etc.).
  - `src/index.js:54-62` (`setCheckBox`) sets both the `checked` attribute AND the `.checked` property, so the visual state reflects the store.
  - The hunter's Playwright `press Space` almost certainly did not deliver a keydown to the actual `<input type="checkbox">` (focus was probably on a wrapper). Playwright's `check()` bypasses click/keydown events entirely — so it would genuinely not trigger the store, but that's a test-tool artifact, not a real-user bug.
- **Iter-2 verdict:** with focus explicitly placed on `<input type=checkbox aria-label="Select …">` (confirmed via `document.activeElement.outerHTML`), Space toggles the store 0→1 and Enter toggles it back. Iter-1 hunter was focused on a wrapper element.
- **Evidence:** `tasks/bug-hunt-evidence/iter-2026-04-21-2/15-keyboard-space-enter-works.png`

### BH-005 · Global search is case-sensitive prefix-only (feature gap)
- **Status:** unverified (claim plausible; no code trace done yet)
- **Severity:** feature-gap (discoverability pain, not a defect against current spec)
- **Iter:** 1 · **Workflow:** recipe-collection (Cmd+K global search)
- **Claim:** `Pasta` matches; `pasta` does not. `Weeknight` matches; `Weeknite` (one typo) returns nothing and no near-match suggestion.
- **Follow-up:** confirm by inspecting `src/components/globalSearch.js` + the backing search API (likely SQLite FTS5). The repo already builds with the `fts5` tag per `CLAUDE.md`, so fuzzy/case-insensitive support may simply be a configuration improvement. Priorities: case-insensitive first, fuzzy second.

---

## Fixed / closed pre-existing

| ID | Status | Confirmation |
|----|--------|---------|
| BH-P01 | **fixed** (confirmed iter 7) | Bulk endpoints (`/v1/resources/delete`, `addTags`, `removeTags`, `addMeta`) return `400 {"error":"at least one resource ID is required"}` on empty ids, `404 {"error":"one or more resources not found"}` on nonexistent. No silent 200s. |
| BH-P02 | **fixed** (confirmed iter 7) | `POST /v1/resources/addTags ids=<real>&EditedId=99999999` → `404 {"error":"one or more tags not found"}`. |
| BH-P03 | **fixed** (confirmed iter 7) | `/note?id=abc`, `/resource?id=abc`, `/group?id=abc`, `/tag?id=abc` all return `400` with user-friendly `"invalid value for \"id\": must be a valid number"`. No gorilla/schema internals leak. |
| BH-P04 | **fixed** (confirmed iter 4) | Duplicate-tag API returns `{"error":"a tag named \"…\" already exists"}` — clean JSON, no raw SQLite leak. |
| BH-P06 | **fixed** (confirmed iter 4) | `GET /notes?name=pasta&tags=1` correctly pre-populates the Name filter with "pasta". Evidence: `tasks/bug-hunt-evidence/iter-2026-04-21-4/10-notes-filter-prepopulated.png`. |
| BH-P05 | **fixed** (2026-04-22, c1-error-hygiene, merged 0aa5d39e) | Added `_appContext` + `_requestContext` to `discardFields` denylist in `server/template_handlers/render_template.go`. `.json` error paths no longer serialize server config. Regression test: `server/api_tests/json_error_leaks_appcontext_test.go`. |
| BH-019 | **fixed** (2026-04-22, c1-error-hygiene, merged 0aa5d39e) | New `application_context/validation/entity_name.go` `SanitizeEntityName` helper rejects NUL bytes, C0/C1 controls (except `\t`), Unicode directional overrides (U+202A–U+202E, U+2066–U+2069), embedded newlines. Wired into tag/group/note/resource/noteType/category create+update. API test: `server/api_tests/entity_name_control_chars_test.go`. |
| BH-006 | **fixed** (2026-04-22, c2-form-ux, PR #24 merged 7b7e9fee) | New `HandleFormError` helper in `server/http_utils/http_helpers.go` implements Post-Redirect-Get + form-value preservation on HTML-accepting requests. Wired into all 6 entity create/edit handlers. Templates re-populate fields from `queryValues.*` and show an error banner. JSON 400 path unchanged. E2E: `e2e/tests/c2-bh006-form-redirects.spec.ts`. |
| BH-009 | **fixed** (2026-04-22, c2-form-ux, PR #24 merged 7b7e9fee) | `src/schema-editor/modes/form-mode.ts` now consults `input.validity.{valueMissing,patternMismatch,typeMismatch,tooShort,tooLong,stepMismatch}` on blur and dispatches blur on every input at submit. Error state tracked in reactive `_errors: Map<string,string>`; renders `aria-invalid="true"` + `#field-<name>-error` spans. E2E: `e2e/tests/c2-bh009-schema-editor-validation.spec.ts`. |
| BH-011 | **fixed** (2026-04-22, c3-image-hashing, PR #25 merged 5e24866f) | Image ingestion in `application_context/resource_media_context.go` now rejects uploads where `image.Decode` errors or bounds are zero, returning HTTP 400 "uploaded file is not a valid image (failed to decode)". API test: `server/api_tests/image_ingestion_rejects_truncated_test.go`. Fixture fix: `cmd/mr/testdata/sample.jpg` replaced with a valid 4×4 JPEG (was malformed, previously masked by the W=0/H=0 bug). |
| BH-018 | **fixed** (2026-04-22, c3-image-hashing, PR #25 merged 5e24866f) | New `AreSimilar` helper in `hash_worker/worker.go` requires both DHash AND AHash Hamming distances below thresholds before recording a similarity pair. New flag `--hash-ahash-threshold` / `HASH_AHASH_THRESHOLD` (default 5). Unit test: `hash_worker/worker_solid_color_test.go` — solid lightblue + orange NOT recorded as similar; near-duplicate gradients still recorded. |
| BH-020 | **fixed** (2026-04-22, c4-deletion-cascade, PR #26 merged 7abe0e77) | Shared scrubber `application_context/block_ref_cleanup.go` walks `note_blocks.content` and strips matching IDs from `resourceIds[]`, `groupIds[]`, `calendars[].source.resourceId`, `queryId`. Resource/group/saved-query DELETE handlers call it inside their transaction. One-shot boot migration `MigrateBlockReferencesOnce` scrubs pre-existing orphans, gated by `SKIP_BLOCK_REF_CLEANUP=1` + GORM upsert marker for Postgres compat. Gallery/references/table UI components gracefully render "unavailable" on 404. Tests: `application_context/block_ref_cleanup_test.go` (12 cases) + `server/api_tests/block_ref_cascade_test.go` (3 integration cases). |
| BH-024 | **fixed** (2026-04-22, c4-deletion-cascade, PR #26 merged 7abe0e77) | Table-block query handler in `server/api_handlers/block_api_handlers.go` now routes the inner query-fetch error through `statusCodeForError` so `gorm.ErrRecordNotFound` yields HTTP 404 instead of 500. API test: `server/api_tests/table_block_dangling_query_returns_404_test.go`. |
| BH-025 | **fixed** (2026-04-22, c5-jobs-ui-a11y, PR #27 merged f60bd9f3) | `src/components/adminExport.js` `init()` rehydrates from `localStorage.getItem('adminExport:currentJobId')` and subscribes to the jobs SSE stream. On submit the jobId is persisted; on completion or stale rehydrate it is cleared. E2E: `e2e/tests/c5-bh025-admin-export-reload.spec.ts`. |
| BH-026 | **fixed** (2026-04-22, c5-jobs-ui-a11y, PR #27 merged f60bd9f3) | `getJobTitle()` in `src/components/downloadCockpit.js` falls back to `job.name \|\| 'Group export'` when `job.source === 'group-export'`. New `x-if` branch in `templates/partials/downloadCockpit.tpl` renders `<a href="/v1/exports/{jobId}/download">` when `status === 'completed' && source === 'group-export' && resultPath`. E2E: `e2e/tests/c5-bh026-download-cockpit-group-export-link.spec.ts`. |
| BH-028 | **fixed** (2026-04-22, c5-jobs-ui-a11y, PR #27 merged f60bd9f3) | Download cockpit panel gains `role="dialog" aria-modal="true" aria-labelledby` + `$watch('isOpen')` focus management (first-focusable on open, trigger-restore on close). Progress bars gain `role="progressbar" aria-valuenow/min/max + aria-label`. Connection-status dot gains `role="img" + aria-label` and color contrast bumped from stone-400 to stone-500/600 for WCAG AA. E2E: `e2e/tests/c5-bh028-download-cockpit-a11y.spec.ts`. |
| BH-027 | **fixed** (2026-04-22, c6-block-editor-a11y, PR #28 merged 5460bdae) | `templates/partials/blockEditor.tpl` + `src/components/blockEditor.js`: gallery `<img>` gains dynamic `:alt` from `resourceMeta[resId]?.name`; heading-level `<select>` gains `aria-label="Heading level"`; move-up/move-down/delete buttons gain `:aria-label` + live-region announcement on reorder; Add-Block picker trigger gains `aria-expanded/aria-haspopup/aria-controls`; picker dropdown converted to `<ul role="listbox">` with roving tabindex + Arrow/Home/End handlers. E2E: `e2e/tests/accessibility/c6-bh027-block-editor-a11y.spec.ts`. |
| BH-023 | **fixed** (2026-04-22, c7-alt-fs, PR #29 merged 8467c32f) | Three-layer fix: (1) `archive/manifest.go` `ResourcePayload` gains optional `storage_location` field, forward-compat, NO schema_version bump. Exporter + importer wired in `application_context/export_context.go` + `apply_import.go`. (2) `models/query_models/resource_query.go` `ResourceCreator` + `ResourceFromRemoteCreator` gain `PathName`; `AddResource` in `application_context/resource_upload_context.go` validates against `Config.AltFileSystems` and routes file IO accordingly. (3) `templates/createResource.tpl` renders a `<select name="PathName">` when alt-fs is configured; `altFileSystems` exposed via `resource_template_context.go`. Tests: `application_context/export_import_altfs_test.go`, `server/api_tests/resource_create_pathname_test.go`, `e2e/tests/c7-bh023-alt-fs-select-visible.spec.ts`. |
| BH-031 | **fixed** (2026-04-22, c8-share-allowlist, PR #30 merged 3bed7dd8) | `server/share_server.go` `handleBlockStateUpdate` resolves the target block's type after note/ownership checks and requires it in an allowlist `map[string]bool{"todos": true}`. Non-matching types return HTTP 403 Forbidden. API test: `server/api_tests/share_server_block_state_allowlist_test.go`. Also exports `ShareServer.Handler()` for in-process testing. |
| BH-001 | **fixed** (2026-04-22, c13-cosmetic-cleanup, PR #32 merged fec44787) | Dropped the duplicate `{% include "/partials/sideTitle.tpl" ... %}` from `templates/displayTag.tpl` and `templates/displayNoteText.tpl`; `partials/json.tpl` already owns the `<h2>Meta Data</h2>`. E2E: `e2e/tests/c13-bh001-dup-meta-heading.spec.ts`. |
| BH-002 | **fixed** (2026-04-22, c13-cosmetic-cleanup, PR #32 merged fec44787) | `renderJsonTable(null)` and `renderJsonTable(undefined)` now return an empty `DocumentFragment` up front in `src/tableMaker.js`, so the `appendChild` call in `templates/partials/json.tpl` no longer throws `TypeError: parameter 1 is not of type 'Node'`. Object guard also simplified now that null/undefined is handled up front. E2E: `e2e/tests/c13-bh002-json-table-null.spec.ts`. |
| BH-007 | **fixed** (2026-04-22, c13-cosmetic-cleanup, PR #32 merged fec44787) | `templates/partials/versionPanel.tpl` action bar now uses `flex flex-wrap gap-y-2` so the upload form drops to a second row on narrow widths; the Compare toggle + Compare-Selected share an inner flex row; both action buttons get `whitespace-nowrap` so labels never split mid-label. E2E: `e2e/tests/c13-bh007-version-panel-layout.spec.ts` asserts button height stays within 1.8x its line-height with Compare Selected visible at 1024px. |
| BH-015 | **fixed** (2026-04-22, c10-jobs-ui-polish, PR #35 merged dd2c68b2) | UI cap: `Math.min(100, ...)` on both label sites (`templates/adminExport.tpl:122`, `src/components/downloadCockpit.js` `formatProgress`). Backend accuracy: new `estimateJSONOverhead(plan)` adds `2 KB manifest + 1 KB × entity count` to `plan.totalBytes` at the end of `buildExportPlan` in `application_context/export_context.go`. Unit: `application_context/export_overhead_test.go`. E2E: `e2e/tests/c10-bh015-export-progress-cap.spec.ts`. |
| BH-036 | **fixed** (2026-04-22, c10-jobs-ui-polish, PR #35 merged dd2c68b2) | `/admin/export` gains a helper line citing `config.ExportRetention` (`templates/adminExport.tpl` `data-testid="export-retention-helper"`). `downloadCockpit` shows an "Expires in X" line per completed group-export row, computed from `job.completedAt + exportRetentionMs`. Values threaded into every template via `wrapContextWithPlugins` in `server/routes.go`; ms variant shipped to the client via a `<meta name="x-export-retention-ms">` tag on `base.tpl` and read by `downloadCockpit.js` on init. E2E: `e2e/tests/c10-bh036-export-retention-disclosure.spec.ts`. |
| BH-016 | **fixed** (2026-04-22, c11-import-ux, PR #36 merged 06bc2b20) | `ImportApplyResult` in `application_context/import_plan.go` gains 9 new counters: `MergedGroups/Resources/Notes`, `LinkedByGUIDGroups/Resources/Notes` (reserved — forward-compat), `SkippedByPolicyGroups/Resources/Notes`. Wired into `apply_import.go` at the GUID-collision switches for groups, resources, and notes; `replace` branches also bump `Merged*` (replace is a mutation-heavy variant of merge). `HasMutations()` extended accordingly. `templates/adminImport.tpl` surfaces created + merged + skipped-by-policy per entity type. Unit: `application_context/import_counters_test.go`. |
| BH-017 | **fixed** (2026-04-22, c11-import-ux, PR #36 merged 06bc2b20) | `archive/reader.go::ReadManifest` now reads the manifest body once, unmarshals into a `map[string]json.RawMessage` to presence-check `schema_version`, then unmarshals into the typed `Manifest`. Absent field → new `ErrMissingSchemaVersion` ("manifest is missing required field `schema_version`"). Present-but-invalid → existing `ErrUnsupportedSchemaVersion`. Unit: `archive/reader_missing_schema_version_test.go`. |
| BH-012 | **fixed** (2026-04-22, c12-mrql-polish, PR #37 merged d9f7ed77) | `src/components/mrqlEditor.js` tracks `loadedSavedQueryId` / `loadedSavedQueryName`; loading a saved query populates them, deleting the loaded row clears them, and saving-as-new clears them after a successful POST. New `updateQuery()` routes to `PUT /v1/mrql/saved?id={id}` reusing the loaded name + current editor text. The `templates/mrql.tpl` action bar splits into an `mrql-update-button` (visible only when a saved query is loaded) and an `mrql-save-as-new-button` whose label flips between "Save" and "Save as new" based on `canUpdate`. The saved-queries panel exposes `data-testid="mrql-saved-panel"` with `data-saved-id` per row, and the save dialog gains `mrql-save-name-input` / `mrql-save-confirm-button` testids. E2E: `e2e/tests/c12-bh012-mrql-update-vs-save.spec.ts` (3 scenarios). |
| BH-013 | **fixed** (2026-04-22, c12-mrql-polish, PR #37 merged d9f7ed77) | Removed the hardcoded `defaultMRQLLimit = 1000` const in `application_context/mrql_context.go`; replaced with `ctx.defaultMRQLLimit()` which reads `Config.MRQLDefaultLimit` and falls back to the exported `DefaultMRQLLimitFallback = 1000` when the field is zero (keeps `MahresourcesConfig{}` test fixtures working without per-file edits). New `--mrql-default-limit` flag / `MRQL_DEFAULT_LIMIT` env var (default 500) wired through `main.go` → `MahresourcesInputConfig` → `MahresourcesConfig`. `MRQLResult` + `MRQLGroupedResult` gain `default_limit_applied` + `applied_limit` JSON fields, set in `ExecuteMRQL` / `ExecuteMRQLGrouped` / `ExecuteMRQLGroupedWithScope` based on whether `parsed.Limit < 0` at the override boundary. `mrqlEditor.js::execute()` captures the flag and `templates/mrql.tpl` renders an `mrql-default-limit-banner` ("Default limit applied (N rows) — add LIMIT / OFFSET to the query to paginate."). Unit: `server/api_tests/mrql_default_limit_test.go` (non-grouped + grouped + explicit-limit paths). E2E: `e2e/tests/c12-bh013-mrql-default-limit-banner.spec.ts`. Docs: new row in CLAUDE.md config table. |

---

## Iteration log

### Iter 14 — 2026-04-22 04:03 CEST (consolidation)
- **Mission:** no new bug hunt — re-verify the older half of the active backlog hasn't been silently fixed. 12-bug sample across the curl + browser paths.
- **Hunter:** Evidence Collector (Sonnet), mixed curl + `bughunt14` playwright-cli session, ~6 min wall.
- **Matrix verdict (12 bugs sampled):**
  - **12 STILL PRESENT, 0 FIXED.** Log is accurate.
  - BH-017 was flagged "CHANGED" by the hunter but the error text is literally the same (`"unsupported schema_version 0 (supported: [1])"`) — just reached via a cleaner repro. Count it as still present.
  - BH-011 empirically reproduced again on a fresh truncated PNG upload (new resource ID 115 saved with `W:0 H:0 ContentType:image/png`).
  - BH-015 caught two live samples: group-export jobs `dee4ddf5` and `90aab83d` showing `progressPercent: 5140.9` and `5333.5`.
  - BH-034 pushed to 50 MB this time (iter 12 did 25 MB) — `HTTP 200` in 0.12s, no size limit triggered.
  - BH-P05 still leaks `DbDsn`, `FfmpegPath`, `_appContext.Config` on `/resource.json?id=abc`.
- **Opportunistic findings:** none genuine. `/timeline`, `/resources/similar`, `/resource/similar` all return 404 (routes either removed or never existed at those paths; the similarity surface is the detail-page panel per iter 6, not a dedicated page). No new bugs filed.
- **Evidence-drift note (worth tracking, not filing):** BH-020's gallery block 21 in note 276 still references `resourceId=112`, but due to SQLite ID reuse, resource 112 is NOW a 25 MB binary probe from iter 7, not the image the block originally pointed at. The structural bug (dangling-ref after delete) remains live; the specific target just got recycled. Future fix verification will need to re-create the exact scenario rather than rely on this artifact.
- **404 page spurious `alert('1')` sighting** from the hunter's opportunistic pass: this is stale test-data pollution (an `alert(1)` payload from a prior iter's XSS probe in a note Description that the 404 fallback template is rendering through `|safe`). Not a new bug — CLAUDE.md explicitly sanctions unescaped custom HTML in description/sidebar/header. Noted for orchestrator awareness: the test DB is accumulating noisy content that will increasingly surface in unrelated contexts.
- **Net new real bugs this iter:** 0 (intentional — this was a consolidation pass).
- **Log-hygiene outcome:** **35 active bugs confirmed accurate**; no stale entries to remove. The log now has provenance for every bug older than iter-9 being verified recently.
- **Follow-up directives for next hunter:**
  1. **Resume real-world bug hunting** next iter — consolidation is done for this round.
  2. **Still queued** from prior: share-token expiry design-gap filing, primary-server security-headers audit, a11y-specs-vs-iter-11 diff.
  3. **Database-state hygiene note:** the test DB now has hundreds of `[bughunt-*]` entities plus genuine orphans (resources 87, 107, 115 with W=0/H=0; various dangling block refs). Not a bug, but iters are running slower and opportunistic pages keep surfacing test-data pollution. If the user wants a cleanup pass, we could dedicate a short iter to deleting `[bughunt-*]`-prefixed entities via bulk API. Do **not** auto-do this — only on explicit request.
  4. **Future consolidation cadence:** run one of these every ~6 iters so we catch silent fixes without letting the log rot. Roughly 5-10 minutes each.

### Iter 13 — 2026-04-22 03:33 CEST
- **Workflow:** browser-first iter (correcting iter-12's curl-overuse drift). A) end-to-end share-note experience in a real browser; B) share management UX + share-other-entity probe; C) admin surfaces (export retention, hash-worker); D) one small curl probe for the shares API shape.
- **Hunter:** Evidence Collector (Sonnet), `bughunt13` + `bughunt13-fresh` sessions, ~12 min wall. 17 screenshots captured (previous iter had 0 for browser-driven claims).
- **Net new real bugs this iter:** 4.
  - **BH-035 minor** — no centralized share management dashboard; revocation is per-note.
  - **BH-036 minor** — export UI doesn't disclose retention window (compounds BH-025 / BH-026).
  - **BH-037 cosmetic** — perceptual-hash values not visible in UI; can't debug BH-018 without SQL.
  - **BH-038 cosmetic today / latent major** — `shareToken` leaks into every note card's Alpine `x-data` on `/notes`. Orchestrator re-confirmed via curl (3 tokens in page body).
- **Headline positives** (worth calling out — the share surface is in solid shape on the fundamentals):
  - Shared page renders end-to-end; all gallery images load cleanly from `:8383` with zero console errors, no CORS complaints, no mixed-content issues.
  - CSS (`tailwind.css`, `index.css`) and JS (`main.js`) all load correctly from `:8383`.
  - Lightbox on shared page works; clicks serve from `:8383` (no leakage to `:8181`).
  - **References-block groups render as read-only spans, NOT as anchors.** That closes the iter-12 "references leak group data" scope caveat — viewers see names but can't navigate to the author's primary server.
  - Read-only enforcement holds — 0 edit/delete elements in shared DOM, 0 forms, only 3 buttons (lightbox close/prev/next).
  - Fresh-session share render works (no cookies required).
- **Pre-existing confirmed:** BH-033 (non-routable share URL — screenshot evidence on note 282), BH-031 (injected `{"malicious_data":true}` still visible in note 282's gallery block state from earlier iter — persistent write leak).
- **Non-bug:** `alert(1)` / `console.log(...)` execution in note Descriptions on the primary server is within CLAUDE.md's documented "trusted operator — custom HTML is an intentional extension point" model. Not filed.
- **Follow-up directives for next hunter:**
  1. **Consolidation iter** is now genuinely due — we're at 31 active bugs and haven't verified fix-status across the older ones in 10+ iters. Next iter could `curl` a small matrix confirming each open bug still repros, so we don't accumulate stale entries.
  2. **Expiry for share tokens** — BH-035's fix list references "if BH-033's expiry-field fix lands". In truth, no iter has filed the *lack of* expiry as a bug. Might be worth filing — a permanent token that can only be revoked by going to the specific note is low-defensibility for a public-facing surface. Worth a prompt to think about whether this is a design choice or a gap.
  3. **Still unfinished:** security-headers audit on the primary server (BH-032 analogue), request-body size probe extended to a multi-GB stream, and running the a11y specs against iter-11 findings.
  4. **Browser-first worked:** all 17 screenshots + clean structure. Keeping this per-workflow tool-labeling pattern for future iters.

### Iter 12 — 2026-04-22 03:03 CEST
- **Workflow:** A) the **secondary share server on :8383** (untouched for 11 iters, config reveals `ShareBindAddress: "127.0.0.1", SharePort: "8383"`); B) "photo essay curator" ribbon tying shares to real usage; C) request-body size-limit probe.
- **Hunter:** Evidence Collector (Sonnet), `bughunt12` + curl against both ports, ~12 min wall.
- **Raw findings:** 4 new bugs + comprehensive security probe matrix on the share server.
- **Orchestrator verdict:**
  - **BH-031 medium** — share-server block state write path has no type allowlist. Orchestrator verified at `share_server.go:131-173`: token + block-ownership are checked, block type is not. Any share-token holder can persist arbitrary state shapes to gallery/text/references/heading/divider blocks — only intended for todos.
  - **BH-032 minor** — no security headers on share responses. Clickjacking + Referer-token-leak exposure (Google Fonts external-fetch in base template).
  - **BH-033 minor** — ShareBaseUrl uses bind address verbatim, producing non-routable share URLs when bound to loopback (the default).
  - **BH-034 minor (latent major)** — no `MaxBytesReader` on resource/version upload paths. Orchestrator confirmed `version_api_handlers.go:86` has `ParseMultipartForm(100 << 20)` with no `MaxBytesReader` guard; `import_api_handlers.go:41-45` does it correctly. 25 MB binary accepted empirically.
- **Positive signals (solid surface here):**
  - **Token cryptographic hygiene is excellent** — 128-bit `crypto/rand` hex, unique index, no enumeration, no timing oracle (~8ms flat for valid/invalid tokens).
  - **No cross-surface route leakage** — `:8383` returns 404 on `/v1/`, `/admin/`, `/resource`, `/api/`. The share server is a completely separate handler set, not a proxy.
  - **No BH-P05 repro on share** — `GET /s/<token>.json` returns plain-text 404, not a JSON config dump.
  - **SQL-injection probe in token slot** returns 404 (parameterized via GORM).
  - **Write method confusion** — `PUT /s/<token>` and `DELETE /s/<token>` correctly return 405; `POST /s/<token>` returns 404 (gorilla-mux quirk, not exploitable).
  - **Resource access is scoped** — only hashes that appear in the note's resource set or gallery blocks are accessible via `/s/<token>/resource/<hash>`.
- **Scope caveat worth operator awareness** (not filed as a bug): the `references` block renders referenced group names, descriptions, and categories to anonymous share viewers. This is by-design but expands the share's effective scope beyond "just this note". An operator who thinks "I'll share the note only" may surprise themselves.
- **Net new real bugs this iter:** 4 (BH-031 medium, BH-032/033/034 minor). 0 scope expansions; this was all fresh surface.
- **Follow-up directives for next hunter:**
  1. **Broader security-headers audit on the primary server** — BH-032 applies to the share server but the primary :8181 has the same gap. Lower priority because the primary is documented "private network only" (CLAUDE.md), but the CSP story would still help defense-in-depth.
  2. **Share feature extensions** — can you share a **group** (not just a note)? A **query**? The share routes found only `GET /s/<token>` for notes — if groups can't be shared at all, that's a natural feature gap.
  3. **Share token management UX** — is there a list of all shared notes somewhere, with the ability to revoke individual tokens? (There's a `DELETE /v1/note/share` per code, but surfacing matters.)
  4. **Queue status**: export retention UI, hash-worker job surface, run-a11y-specs-vs-iter-11-findings diff — all still pending.
  5. **Consider a "consolidation" iter** soon — at 27 active bugs, it's worth a hunter dedicated to re-checking fix status across the whole active set. Ideally scheduled AFTER the developers have had a chance to address some of them.

### Iter 11 — 2026-04-22 02:33 CEST
- **Workflow:** dedicated **accessibility audit** on flows not covered by the 14 existing a11y specs in `e2e/tests/accessibility/` — block editor, Download Cockpit, group tree, resource compare. Plus probes for MetaSchema reference-field cascade and plugin block type.
- **Hunter:** Accessibility Auditor (Sonnet), `bughunt11` session with axe-core + ARIA tree eval, ~19 min wall.
- **Raw findings:** 10 numbered issues across 4 flows.
- **Orchestrator verdict:**
  - **Consolidated to 4 composite bugs**, one per surface, since each grouping shares a natural fix location:
    - **BH-027 major** — block-editor: gallery `<img>` no alt (axe critical), heading select no label (axe critical), move/delete buttons title-only, Add Block picker missing disclosure ARIA.
    - **BH-028 major** — downloadCockpit: panel not a dialog + no focus management, progress bars no ARIA, connection dot color-only. (Positive: live region IS used for job lifecycle events.)
    - **BH-029 minor** — group tree: missing `role=tree/treeitem`, no arrow-key WAI-ARIA pattern. (Positive: expand button `aria-expanded` + `aria-label` with child count are correct.)
    - **BH-030 minor** — compare view: diff cards color-only (WCAG 1.4.1), radiogroup no roving tabindex.
- **MetaSchema reference-cascade probe: N/A.** `src/schema-editor/tree/detail-panel.ts:212` defines the full supported type set as `['string', 'integer', 'number', 'boolean', 'object', 'array', 'null']`. No reference-type field exists, so the BH-020 class cascade question doesn't apply through this path.
- **Plugin block type: not vulnerable.** One plugin installed (`example-blocks` / `counter`). Stores only `label` (string) + `count` (number); zero entity IDs. BH-020 scope stays at 4 block types (gallery, references, calendar, table). The counter block itself has a minor a11y gap (`+1` button no `aria-label`, no live region on count update) — noted but not filed: it's a demo plugin, not production functionality.
- **Net new real bugs this iter:** 4 composite (BH-027 major, BH-028 major, BH-029 minor, BH-030 minor). 2 negative findings (MetaSchema, plugin block) that close those open questions.
- **Follow-up directives for next hunter:**
  1. **Run the existing a11y specs** (`e2e/npm run test:with-server:a11y`) and diff against these new findings — do any existing specs cover flows we now know have bugs but fail to catch them? That could surface flaky or weak assertions.
  2. **A11y audit for the schema-editor authoring flow** (not just display) — `schema-editor-a11y.spec.ts` exists but iter 3 BH-009 showed silent validation failures, which is a11y-adjacent. Worth a focused review.
  3. **Jobs UI end-to-end fix test** — if BH-025 + BH-026 + BH-028 land together, this is a good acceptance scenario.
  4. **Remaining queued:** export retention UI, hash-worker job surface, large-body upload probe (request-body size limit).

### Iter 10 — 2026-04-22 02:03 CEST
- **Workflow:** operator migrating a knowledge base between instances — large-ish export, jobs UI observation, page-reload mid-flow, code-audit sweep for BH-024 siblings.
- **Hunter:** Evidence Collector (Sonnet), `bughunt10` session + curl, ~10 min wall.
- **Raw findings:** 2 new bugs + 1 full endpoint matrix.
- **Orchestrator verdict:**
  - **BH-024 sibling audit: CLEAN.** 11 endpoints tested with `?id=9999999` (nonexistent); all return 404 via `statusCodeForError()` helper at `server/api_handlers/error_status.go`. Zero siblings. **Refined BH-024 scope:** the bug is NOT "endpoint returns 500 for missing record" — it's "endpoint returns 500 when an *existing* record contains a dangling reference to a deleted target". Narrower + more actionable. Updated the BH-024 entry.
  - **BH-025 new, medium.** `adminExport.init()` doesn't resubscribe to the SSE stream; `this.job` resets to null on reload.
  - **BH-026 new, medium.** Download Cockpit panel blanks out group-export job titles and lacks a download-link branch for them. Paired with BH-025 — together they form the whole "operator reloads and loses their export" failure mode.
- **Re-confirmed:**
  - **BH-015 still open** — live SSE data for 5 existing exports shows `progressPercent: 5140.9` and `5333.5` for `totalSize: 352 / 176` bytes.
- **Headline positives:**
  - `downloadCockpit.connect()` SSE reconnect works correctly; `init` event rehydrates full job state across tabs.
  - Multi-tab sync works — both tabs see the same jobs via independent subscriptions.
  - `POST /v1/jobs/cancel` wired through both components.
  - ErrRecordNotFound translation is consistent across the app (only BH-024's inner-lookup is the outlier).
- **Net new real bugs this iter:** 2 (BH-025, BH-026) + 1 scope correction (BH-024).
- **Follow-up directives for next hunter:**
  1. **Export retention surface** — CLAUDE.md says 24h default but the UI doesn't show retention info. Is it possible to tell a stale tar is about to vanish? Minor UX.
  2. **Plugin block type** — still queued. BH-020 audit skipped it; plugins may store IDs.
  3. **Note MetaSchema reference-field deletion cascade** — BH-020 covered blocks, not MetaSchema fields. If a schema-editor field has `type: "reference"` pointing at an entity that gets deleted, is the Meta value scrubbed?
  4. **Hash-worker job surface** — is there a way to see pending hash jobs or the similarity pair count over time?
  5. **Large body + streaming** — iter 7 confirmed app-level 1000-char name limit, but the general request-body limit wasn't explored. Try a multi-MB file upload to the resource API without multipart — is there a size cap anywhere?

### Iter 9 — 2026-04-22 01:33 CEST
- **Workflow:** A) Alt-fs "archival NAS" scenario — confirm what the `FILE_ALT_*` config actually enables (leaked `some_key → /some/folder` in BH-P05); B) block-types audit — is BH-020's dangling-ref bug isolated to `gallery` or systemic?
- **Hunter:** Evidence Collector (Sonnet), `bughunt9` + Bash + curl, ~10 min wall.
- **Raw findings:** 5 bullets → consolidated to 1 scope expansion + 2 new bugs by the orchestrator.
- **Orchestrator verdict:**
  - **BH-020 scope expanded (systemic).** Confirmed via `curl` that 4 block types retain stale IDs after target deletion: `gallery`→resourceIds, `references`→groupIds, `calendar`→resourceId in source, `table`→queryId. `text`/`heading`/`divider`/`todos` don't embed external IDs → unaffected. Gallery's finding wasn't the bug — it was a symptom of a systemic pattern.
  - **BH-023 new, medium.** Alt-fs half-implemented across 3 layers: no UI selector, multipart API silently drops `PathName` (because `ResourceCreator` struct has no such field), and `archive/manifest.go`'s `ResourcePayload` has no `storage_location` — so export/import strips the binding. Grouped as one composite bug because they share a root cause and need a coordinated fix.
  - **BH-024 new, minor.** `/v1/note/block/table/query` returns HTTP 500 for dangling queryId. Hunter saw HTTP 200 + error body; I saw HTTP 500 + error body (both wrong; expected 404). Filed with the behavior I could reproduce. Orchestrator spot-checked that other endpoints (`/v1/group?id=9999`) correctly return 404, so this is an outlier not translating `gorm.ErrRecordNotFound`.
- **Headline positives:**
  - Block-editor XSS handling remains solid across all new block types touched.
  - The core block-content storage (fractional-position strings, per-block UUID) survives all these experiments cleanly. The dangling-ref bugs are about deletion-cascade, not about block storage correctness.
- **Net new real bugs this iter:** 2 (BH-023 medium, BH-024 minor) + 1 scope expansion (BH-020).
- **Follow-up directives for next hunter:**
  1. **Audit other gorm ErrRecordNotFound paths** — BH-024 surfaced an outlier. Sweep `application_context/*_context.go` for returns of raw gorm errors to HTTP; likely more endpoints have the same 500-vs-404 mis-translation.
  2. **Plugin block type** — iter 9 deferred the `plugin` block. If plugins can embed IDs of their own, they're BH-020 class too.
  3. **Dangling refs in note MetaSchema free fields** — schema-editor allows reference-type fields in a note's Meta. Same deletion-cascade question as blocks.
  4. **Jobs UI / SSE reconnect** — still queued.
  5. **Hash-worker job UI** — still queued.
  6. **Hunter miscite watch:** the iter-9 hunter reported HTTP 200 where I saw HTTP 500. Not a big deal this time (both are bugs), but it's the second time a hunter's quoted status code disagreed with reality (iter 3 also had a hunter cite the wrong endpoint for F3). Quick orchestrator spot-checks on status-code claims are worth keeping.

### Iter 8 — 2026-04-22 01:03 CEST
- **Workflow:** A) "Daily journal" block-editor deep dive (long text block, reorder, delete, dangling resource, XSS probes); B) OpenAPI spec drift — generate `openapi.yaml` via `cmd/openapi-gen` and diff against live routes in `server/routes.go`.
- **Hunter:** Evidence Collector (Sonnet), `bughunt8` session + Bash, ~12 min wall.
- **Raw findings:** 1 from block editor + 1 observation elevated to bug + 1 OpenAPI drift.
- **Orchestrator verdict:**
  - **BH-020 new, minor.** Verified via `curl /v1/note/blocks?noteId=275` — gallery block still holds `resourceIds:[112]` after resource 112 deletion.
  - **BH-021 new, minor.** Elevated the hunter's "markdown rendering gap" observation into a real bug: `_italic_` and backtick inline code don't render in block text. Users will reasonably expect them to.
  - **BH-022 new, minor (docs).** 11 live routes missing from the OpenAPI spec, most notably the entire MRQL subsystem (6 routes).
- **Headline positives:**
  - Block editor handles **3161-character long text** without truncation or perf issues.
  - Reorder uses **fractional position strings** (`n`, `t`, `w`, `y`, `z` — lexicographic comparison, never collides in the common case) — elegant design, persists correctly.
  - **XSS escape is robust**: `<img src=x onerror=...>`, `<svg onload=...>`, `<iframe srcdoc=...>` all stored as escaped text and rendered inert. `renderMarkdown` HTML-escapes before substituting.
  - OpenAPI generator passes its own bundled validator (156 paths, 80 schemas, 20 tags) and **no phantom routes** — only omissions, never hallucinations.
- **Net new real bugs this iter:** 3 (BH-020 minor, BH-021 minor, BH-022 minor/docs).
- **Follow-up directives for next hunter:**
  1. **Block types audit** — BH-020 was confirmed on `gallery`. Grep `src/components/blocks/` for other block types that embed `resourceIds` or `noteIds` — are any of THEM vulnerable to the same dangling-reference bug? (e.g. a `video` block, an `audio` block, a `link` block with a resource pointer).
  2. **MRQL documentation** — BH-022 surfaced that MRQL is entirely undocumented in OpenAPI. Related: is there ANY user-facing docs for MRQL syntax? Check `docs/` or in-app help. If not, that's a docs gap worth filing.
  3. **Jobs UI / SSE reconnect** — still queued from iter 5 follow-up. Now-plausible scenario: export a large group (we have one from iter 5/6), reload the page mid-export, does `downloadCockpit.js:219`'s `_reconnectDelay` resume cleanly?
  4. **FILE_ALT_*** — alternative file systems. Completely untouched. Config confirms `AltFileSystems: {"some_key":"/some/folder"}` is live (from iter 7 BH-P05 leak). Worth testing: does the alt-fs picker show up in resource upload? What happens on write failure?
  5. **Scheduled tasks (hash worker visibility in UI)** — BH-018 showed the hash worker runs, but we don't know if there's a surface to see pending / failed hash jobs.

### Iter 7 — 2026-04-22 00:33 CEST
- **Workflow:** backend API fuzz. A) re-verify the 4 remaining legacy pre-existing bugs (P01, P02, P03, P05); B) five classes of `/v1/*` fuzz probes (malformed JSON, oversized bodies, type confusion, unicode/control injection, method confusion); C) resource-collision policy `duplicate` vs `skip`; D) quick check that `groupCompareView` is wired.
- **Hunter:** Evidence Collector (Sonnet), mostly `curl`-driven, ~10 min wall.
- **Legacy re-verify:**
  - **BH-P01, BH-P02, BH-P03 all FIXED.** Promoted to the Fixed table with exact curl evidence. Cuts the "unverified legacy" backlog from 4 to 1.
  - **BH-P05 STILL PRESENT, promoted to active as major.** 1214-byte JSON config leak on every `.json` error — `DbDsn`, `FfmpegPath`, `FileSavePath`, `AltFileSystems`, all timeouts, all worker config. I curled it myself to confirm.
- **New finding:** **BH-019 medium** — null bytes, RTL override, and embedded newlines accepted in entity names (tested on `/v1/tag`; same validation surface likely on group/note/resource/noteType/category names).
- **Fuzz matrix (negatives worth recording):**
  - Malformed JSON: clean 400 with raw Go `encoding/json` message (leaks that the stack is Go; not actionable).
  - 5 MB body: rejected by an app-level 1000-char name limit; no HTTP-layer size limit tested.
  - Type confusion (`offset=abc`, `limit=-1`, `sortBy=; DROP TABLE`, `order=injection' OR 1=1`): all silently coerced or whitelisted. No injection surface. Silent coercion is slightly odd UX but not a bug.
  - Method confusion: `GET /v1/resources/delete` returns 404 instead of 405. Gorilla-Mux default behaviour; minor inconsistency, not filed.
- **Resource collision policy:** `duplicate` and `skip` both behave correctly in the tested re-import. The distinction only matters when `guid_collision_policy != merge` and GUIDs are absent — worth noting for future coverage but not a bug today.
- **`groupCompareView`:** route `GET /group/compare` IS wired (`GroupCompareContextProvider` → `groupCompare.tpl`). Not a dead feature. Page renders, no console errors seen.
- **Net new real bugs this iter:** 2 (BH-P05 promoted to major, BH-019 medium). 3 pre-existing bugs closed as fixed. 1 remaining unverified legacy → 0.
- **Follow-up directives for next hunter:**
  1. **Control-char fix verification / broader coverage** — once BH-019 is fixed, re-fuzz group/note/resource/noteType/category names to confirm the fix applies everywhere. Likely a single central helper.
  2. **BH-P05 impact audit** — check every Pongo2 template error branch for `.json` leak. `template_handlers/` + `server/error_handler.go` (or wherever the `.json` suffix triggers JSON encoding of the template context).
  3. **Pending queue:** jobs UI + SSE reconnect, MRQL at seeded scale (BH-013 severity check), block-editor with dangling resource reference, `/admin*` UIs, FILE_ALT_* alternative filesystems, OpenAPI spec validation via `cmd/openapi-gen`.
  4. The legacy backlog is now cleared. Future hunters are free to explore wholly new surfaces.

### Iter 6 — 2026-04-22 00:03 CEST
- **Workflow:** A) perceptual-hash / near-dup (upload solid-color near-dupes, check similarity UI), B) GUID-collision re-import with source still present, C) quick verify of the tag-combobox-on-group-create lead from iter 5.
- **Hunter:** Evidence Collector (Sonnet), `bughunt6` session, ~14 min wall.
- **Raw findings:** 3 new + 1 verify.
- **Orchestrator verdict:**
  - **Workflow C (tag combobox) → not reproduced.** `GET /v1/group?id=1012` confirmed `Tags: [{ID:27, Name:"[bh5-exp]"}]` after saving with the tag selected. Iter-5 aside closed.
  - **F1 (DHash false positive) → BH-018 new, medium.** Verified in code. Real correctness bug in a user-surfaced feature.
  - **F2 (merge-policy import hides activity) → merged into BH-016.** Same root cause as iter 5's re-link-hides-activity finding. Updated BH-016 scope to cover both re-link and merge silent paths.
  - **F3 (server-stats API missing hash config) → rejected.** The hunter looked at the wrong endpoint; `/v1/admin/data-stats` does return the full hash config under `config.hashSimilarityThreshold/hashPollInterval/hashBatchSize/hashCacheSize`. Confirmed via curl. Not a bug.
- **Headline positives:**
  - **Perceptual-hash UI does exist** and works end-to-end (detail-page "Similar Resources" panel, resources-list "Show Only With Existing Similar Images" filter, admin-overview "Similarity Detection" stats, "Merge Others To This" bulk action). Worker processes uploads within seconds.
  - **GUID collision policy UI works as spec'd** — dropdown with `merge` / `skip` / `replace` options, clear "N entities match by GUID" warning before apply. Default `merge` is idempotent — tested by re-importing twice, no duplicates.
- **Net new real bugs this iter:** 1 (BH-018 medium). 1 scope expansion (BH-016). 1 aside closed.
- **Follow-up directives for next hunter:**
  1. **Jobs UI + SSE reconnect** (still queued) — BH-015 surfaced that `downloadCockpit.js` has asymmetric capping; run a long-ish export, page-reload mid-export, verify the `_reconnectDelay` reconnection path at `downloadCockpit.js:219` reattaches and resumes the progress stream.
  2. **Resource collision policy** — iter 6 tested GUID policy (`merge`/`skip`/`replace`). The Resource Collision Policy dropdown is separate (`skip` / `duplicate`). Test both scenarios.
  3. **MRQL perf at seeded scale** — still queued. Bulk-create 5k notes via API and re-run `type = note`; confirm BH-013 (no LIMIT) manifests at scale.
  4. **Block editor with dangling resource** — text block referencing a deleted resource; reorder under latency.
  5. **`/compare` for groups** — `src/components/groupCompareView.js` exists; groupCompareView not yet exercised in any iter.
  6. **Backend-only fuzz iter** — BH-P01, P02, P03, P05 still pending; do one focused run entirely over `curl` against `/v1/*` bulk and error paths.

### Iter 5 — 2026-04-21 23:33 CEST
- **Workflow:** A) group export → delete → re-import round-trip; manifest v1 contract probes (unknown major version, unknown top-level key, missing schema_version, malformed JSON); B) version-compare view exercise.
- **Hunter:** Evidence Collector (Sonnet), `bughunt5` session, ~18 min wall.
- **Headline positives** (worth calling out, not bugs):
  - **Round-trip is functionally correct.** 3 groups, 2 resources, 2 notes, 1 tag, custom Meta, note Meta — all survived end-to-end.
  - **Manifest v1 contract is enforced as documented in CLAUDE.md.** Unknown schema_version 2 → clear reject. Unknown top-level key → silently ignored (forward compat holds). Malformed JSON → clear parse error, no panic.
  - **Version compare works.** Correctly reports "Files differ" with Type/Size/Hash matches on two PNG versions; correctly reports "Files are identical" when the same version is selected twice. No console errors.
- **Net new real bugs:** 3.
  - **BH-015 minor** — Export progress % overflows 100% for small exports; text and bar contradict.
  - **BH-016 minor** — Import result panel hides GUID-reused entities; shows "0 created" when in fact things were re-linked.
  - **BH-017 cosmetic** — Missing `schema_version` reports "unsupported version 0" instead of "missing field".
- **Hunter's aside about BH-006**: flagged that a tag combobox during group creation didn't persist the tag on save. **Not folded into BH-006** — that's a potentially-different UI interaction bug (tag not actually added, vs form-loss on error). Parked as a lead for a future iter.
- **Still not touched:** the **Jobs UI** for long-running operations (hash worker, export retention), **alternative file systems** (FILE_ALT_*), **OpenAPI spec validation**, and **image perceptual-hash similarity** features. Candidates for iter 6+.
- **Follow-up directives for next hunter:**
  1. **Jobs + download cockpit UI** — now that BH-015 surfaced a bug in `downloadCockpit.js`, the whole jobs pane is worth a focused run. Test: long-running job with cancel, page reload while export is running (does SSE reconnect via `_reconnectDelay` path? `downloadCockpit.js:219`), job retention cleanup.
  2. **GUID re-link flows in more depth** — export a group, KEEP it (don't delete), re-import → what happens with duplicate GUIDs? Does the import offer an override (`GUIDCollisionPolicy` is mentioned in code comments)?
  3. **Perceptual-hash similarity** — `-hash-similarity-threshold`, `-hash-worker-*` flags. Upload near-duplicate images, see if the UI surfaces any similarity / dedupe affordance. Probably an undertested feature slot.
  4. **Tag combobox on group create** — quick verify the hunter's aside; if reproducible it's a real bug.
  5. **Still queued:** MRQL perf at 10x data, block editor with dangling resource, `/admin*` UIs, scheduled tasks.

### Iter 4 — 2026-04-21 23:03 CEST
- **Workflow:** A) MRQL / saved-Query flow, B) Group hierarchy + cycle / self-loop / parent-delete probes, C) Block editor with XSS probe. Plus fast re-verifies of BH-P04 and BH-P06.
- **Hunter:** Evidence Collector (Sonnet), `bughunt4` + `bughunt4-b` sessions, ~16 min wall.
- **Raw findings:** 3 new + 2 re-verifies + comprehensive MRQL / cycle probe results.
- **Orchestrator verdict:**
  - **BH-P04 → fixed, closed.** Duplicate-tag API returns friendly JSON error, not raw SQLite.
  - **BH-P06 → fixed, closed.** Lowercase filter URL params now pre-populate.
  - **F1 (group cycle → raw error page) → merged into BH-006.** Same root-cause pattern, but crucially this is the first EDIT form caught — scope expanded again from "all create forms" to "all create AND edit forms with native post + error". Not a new entry.
  - **F2 (no update for saved MRQL) → BH-012 new, minor.** PUT endpoint exists (`routes.go:499`); UI never calls it (`mrqlEditor.js:370` POST-only). Real feature gap.
  - **F3 (MRQL no default LIMIT / pagination) → BH-013 new, minor today / potentially major at scale.** 264 notes all rendered into the DOM; explicit CLAUDE.md hint about "millions of resources" deployments makes this a latent perf cliff.
  - **Cascade probe → BH-014 new, minor UX.** Parent delete silently orphans children; deserves at least a confirm dialog.
- **Positive signals this iter:**
  - Block editor correctly escapes `<script>` / `onerror` payloads — no XSS surface there.
  - MRQL DSL correctly rejects SQL-injection-style probes at parse time.
  - Parse errors (unbalanced parens, unknown field) return clear inline messages.
  - Cycle + self-loop detection at the backend is solid — it's only the UI surfacing that hurts (BH-006).
- **Net new real bugs this iter:** 3 (BH-012 minor, BH-013 minor, BH-014 minor). 2 pre-existing bugs closed as fixed (BH-P04, BH-P06). BH-006 scope expanded.
- **Follow-up directives for next hunter:**
  1. **Group export / import round-trip** — CLAUDE.md calls out `archive/manifest.go` schema v1 as a "stable public contract". Next iter should: export a group with nested subgroups + resources + notes, wipe, re-import, diff. Plus malformed manifests (unknown major version, unknown fields) — verify the readers reject / ignore per spec.
  2. **MRQL performance at 10x data** — if a hunter can seed a few thousand notes (maybe via API bulk-create), re-run the unlimited query and time it. Might be worth making BH-013 major.
  3. **Block editor deeper dive** — iter 4 only lightly touched blocks. Try: image block with a resource that's been deleted (dangling reference), very long text blocks (do they stream or dump?), rapid reorder under network latency.
  4. **Remaining untouched:** `/compare`, `/resource/compare`, scheduled tasks / backfill jobs (`-hash-worker-*`, `-export-retention`), alternative file systems (`FILE_ALT_*`), OpenAPI spec (`cmd/openapi-gen`).
  5. **Housekeeping:** resources 87, 107 (broken images) + many `[bughunt-*]` entities now live in DB. Not blocking, but deserves a single cleanup pass before iter 10 to prevent cross-iteration contamination. Leave for the user to decide — do NOT auto-delete.

### Iter 3 — 2026-04-21 22:33 CEST
- **Workflow:** A) "Research Paper" NoteType MetaSchema authoring (schema-editor + Preview Form + real note create with required/pattern fields); B) form-loss audit on 4 endpoints; C) truncated-PNG ingestion probe.
- **Hunter:** Evidence Collector (Sonnet), `bughunt3` session, ~14 min wall.
- **Raw findings:** 4 new (F1 required-no-error, F2 pattern-no-error, F3 systemic form-loss, F4 preview numeric=0) + 1 confirmed probe.
- **Orchestrator verdict:**
  - F1 + F2 → **merged into BH-009** (same fix location in `form-mode.ts`, natural to fix together). Major.
  - F3 → **merged into BH-006** (scope expanded from resource-only to "systemic across all entity create-forms"). Stays major.
  - F4 → **BH-010** new, minor. Preview-only — the hunter's workflow-A run showed the real note form does NOT seed `0`, only the Preview tab does.
  - Truncated-PNG → **BH-011** new, major. **Not** a dupe of BH-008: BH-008 is the client-side overlay consequence, BH-011 is the upstream ingestion gap.
- **Net new real bugs this iter:** 3 (BH-009 major, BH-010 minor, BH-011 major). Plus BH-006 expanded from "1 form" to "5 forms systemic".
- **Follow-up directives for next hunter:**
  1. **MRQL / saved-Query flow** — still untouched across 3 iterations. Build a non-trivial query (multi-entity, with date filter and tag filter), save it, re-run it, edit it, share the URL. Candidates for bugs: query param encoding, autocomplete, column order persistence, pagination under saved queries.
  2. **Group hierarchy / nested groups / `GroupRelation`** — iter 1's proposed "trip photo-dump with nested sub-groups" never got exercised. Especially worth testing: can you create a cycle (A owns B, B owns A)? Delete a parent that has children — what happens to the children? Group export/import round-trip (manifest schema v1 contract).
  3. **Block editor** — `templates/partials/blockEditor.tpl` and `src/components/blockEditor.js`. Non-trivial surface; only exercised tangentially in iter 3.
  4. **Backend-only API fuzz iter** — pick one future iter to only hit `/v1/*` endpoints with malformed JSON, missing auth headers (wait, no auth — but malformed params), mass bulk operations, and XSS probes in descriptions / custom headers (though custom HTML injection is explicitly allowed per CLAUDE.md).
  5. **Re-verify BH-P04 + BH-P06** via direct API probes and promote them to `fixed` if the nice error is still there.
  6. **Before next hunter runs**, consider cleaning up DB: resources 87, 107 are broken-image ghosts from bug hunting; many `[bughunt-*]` entities exist. They are not disruptive yet but will muddy future test runs.

### Iter 2 — 2026-04-21 22:10 CEST
- **Workflow:** "Family-photo archive" — upload image resources (JPG + fake-HEIC), drive the crop modal (bounds, width/height input driver, aspect ratio, extreme values), save as new version, exercise version list + compare view; then targeted re-repros of BH-003/BH-004 plus triage of iter-1 `20-remote-url-404.png`.
- **Hunter:** Evidence Collector (Sonnet), `bughunt2` session, ~18 min wall.
- **Raw findings:** 3 new + 2 re-repros.
- **Orchestrator verdict:**
  - **BH-003 → closed, not reproduced.** Checkbox click selects cleanly; `.overlays` has `pointer-events: none` in all states (verified via `getComputedStyle`). Iter-1 hunter clicked the image anchor.
  - **BH-004 → closed, not reproduced.** With focus explicitly on the `<input type=checkbox>` (verified via `document.activeElement.outerHTML`), Space and Enter both toggle the store. Iter-1 hunter had focus on a wrapper element.
  - **BH-006 (remote-URL form blowup) → verified, major** — escalated from the un-triaged iter-1 screenshot.
  - **BH-007 (version-compare layout) → verified, minor.**
  - **BH-008 (crop overlay invisible when W/H=0) → verified, minor** — also surfaces a latent data question about resource 87 (truncated bytes in DB; parked, not filed).
  - Recent crop commits ("center modal and image", "height input driver + HEIC/AVIF UI gating") appear to work correctly from the hunter's positive probes — no regressions surfaced there.
- **Net new real bugs this iter:** 3 (BH-006 major, BH-007 minor, BH-008 minor).
- **Follow-up directives for next hunter:**
  1. **Broader form-loss audit** — hit every native `<form method="post">` the user fills in (create-group, create-note with a rich body, create-note-type with a MetaSchema, create-category, edit-description) and simulate a server-side validation failure on each. Does the user lose the page? BH-006 is probably the tip of a systemic pattern.
  2. **Try uploading a deliberately truncated PNG** and see if the resource gets stored with `Width=0, Height=0` or if the ingestion path rejects it. (Follow-up to BH-008.)
  3. **Pick a different domain this iter**: MRQL / saved-Query flow OR note-type MetaSchema authoring (`templates/` has a `schema-editor` web component and the block editor is non-trivial). These are undertested code paths with a lot of surface.
  4. **Still outstanding:** BH-005 (search fuzzy/case), BH-P01/P02/P03/P05 need their own audit day — not every iter, but worth scheduling one focused "backend-only API fuzz" iter soon.
  5. Re-verify BH-P04 (duplicate tag) and BH-P06 (lowercase filters) once through the JSON API so we can promote them to `fixed`.

### Iter 1 — 2026-04-21 21:37 CEST
- **Workflow:** "Weeknight Recipes" home-cook flow — group → 3 notes → 3 image resources → 3 tags → linking → global search → list filters → bulk select/tag → inline edit → keyboard tab → error paths.
- **Hunter:** Evidence Collector (Sonnet), `bughunt1` session, ~26 min wall.
- **Raw findings:** 5 (F1 dup-meta, F2 renderJsonTable null, F3 lightbox/checkbox, F4 keyboard bulk select, F5 search).
- **Orchestrator verdict:**
  - F1 → **BH-001 verified** (template code confirms).
  - F2 → **BH-002 verified** (code confirms missing guard + tableMaker null path).
  - F3 → **BH-003 unverified**; claim plausible but repro ambiguous.
  - F4 → **BH-004 contradicted by code**; reads as Playwright targeting artifact. Parked with re-repro directive.
  - F5 → **BH-005 unverified feature-gap**; plausible, needs code trace.
  - P04/P06 → promoted to "possibly-fixed" bucket; re-confirm next run.
- **Net new real bugs this iter:** 2 (BH-001, BH-002).
- **Follow-up directives for next hunter:**
  1. Re-repro BH-003 and BH-004 with the targeted methodology described above.
  2. Pick a DIFFERENT workflow direction to broaden coverage (candidates for iter 2: "importing a trip photo-dump as a hierarchical group with nested sub-groups", or "building a saved Query and re-running it").
  3. Try at least one flow that exercises the `imageCropper` / resource versioning path — recent commits touched crop UX and it's undertested.
  4. Try remote URL ingestion (the hunter saw a 404 screenshot `20-remote-url-404.png` that wasn't fully triaged).
