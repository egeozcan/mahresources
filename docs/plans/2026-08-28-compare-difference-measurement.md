# Compare page, Package 3: Difference & measurement

Derived from the `/resource/compare` extension board, packages 1 and 2 (both
merged into master: `feat/compare-manual-alignment` is an ancestor of
`6cd35872`). Branch: `feat/compare-difference-measurement`, off `master`.
Anchors re-verified 28 Aug 2026 — the board's line numbers had drifted through
packages 1 and 2, exactly as the brief warned; the file names have not.

## The three items

| # | Item | Board size | Re-measured |
|---|------|-----------|-------------|
| 3.1 | Difference blend as a fifth mode | XS | **XS** — one radio button, one mode string in the template's `@keydown` enumeration, one CSS rule, one icon. Confirmed. |
| 3.2 | Pixel-diff heatmap and "% pixels changed" | M | **M, at the top of M** — the canvas plumbing is bounded (a capped sample canvas means `getImageData` never scales with the source), but the placement math must re-derive the whole overlay stack (scale × anchor × offset × flip), the recompute has ~12 writer sites, and the gate + banner + live region each carry their own a11y contract. Not L: the sample cap removes the only cost that grew without bound. |
| 3.3 | Blink comparator | S | **S** — the brief's "an interval plus a `destroy()` clear" omits the rate control (2–8 Hz is in the brief), the `prefers-reduced-motion` listener and its mid-play change, and the mode-switch pause. Still S. |

## Settled decisions

| # | Decision |
|---|----------|
| 1 | **3.1 — Difference is the fifth mode in the existing radiogroup**, appended after Toggle. The `@keydown` handler's value list and the new radio both name `'difference'`; the container key handler needs no new branch (difference owns no keys). Its accessible name carries the semantics the visible label cannot: "Difference blend: black means identical" — the visible label is hidden below 768px, so the aria-label is the entire name (package 1 decision 6's rule). |
| 2 | **3.1 — The difference box copies onion skin's structure, minus the opacity bar.** One `x-show="mode === 'difference'"` block: lead image in the flow of the overlay box, trail image on top with a `compare-overlay-img--difference` class carrying `mix-blend-mode: difference`. It composes with scale, anchor and the package-2 offset *by construction* — the images are styled by the same `leadScale`/`trailScale` bindings every overlay mode shares. One rule, no exceptions carved out. |
| 3 | **3.2 — The heatmap is an armed overlay, not a sixth mode.** A toolbar button "Pixel diff" (`aria-pressed`), `x-show` in the overlay modes, following the Anchor/Align precedent. When armed it paints a mask canvas over the pair and puts the percentage in the summary banner. Off by default; state survives a mode switch (the arming-survives-mode-switch rule package 2 decision 13 set for Align). |
| 4 | **3.2 — The mask compares what is painted, in frame pixels**, by re-deriving the overlay placement in canvas coordinates from the numbers the component already computes: `elementSize(index)` (its doc comment names exactly this reuse), the anchor's rest position, and the trail transform from `trailOffset` — `p → O + k(p − O) + (dx, dy)` with `O` the transform-origin (element centre, or its top-left corner under `anchor: top-left`). Canvas pixel space, never screen pixels: the brief's warning that a harness modelling the wrong input accuses correct code applies to the implementation too. |
| 5 | **3.2 — The sample canvas is capped.** Sampling resolution = the box scaled so neither side exceeds `HEATMAP_SAMPLE_SIDE = 512`. `getImageData` therefore costs ~0.26 MP per compose, whatever the source images measure — the multi-megapixel hang the brief describes is structurally impossible. Repaints during drags are coalesced with `requestAnimationFrame`. |
| 6 | **3.2 — A size gate, before work** (`HEATMAP_CONFIRM_ABOVE_MEGAPIXELS = 12`, combined *source* megapixels of both versions), mirroring `textDiff`'s `CONFIRM_ABOVE_BYTES` (`src/components/textDiff.js:15`): over the gate, arming shows a notice with the real number and a "Compute anyway" button, reusing the `compare-diff-gate` / `compare-gate-btn` classes. It is a courtesy gate against decode + downsample cost on a phone, not a bound the page can enforce — the same honesty round 11 of the teardown granted the text gate. |
| 7 | **3.2 — The heatmap computes only on a whole pair.** `!_measured[0] || !_measured[1]` → no computation, no percent event. This is the load-window rule the alignment's deferred rebound already carries: computing against one real measurement and one stored placeholder bakes transient, decode-order-dependent geometry into a number the reader is told is a measurement. Arming during the window waits; the completing report computes. Both decode orders then answer with the final geometry alone. |
| 8 | **3.2 — The percentage's denominator is the overlap.** Numerator: pixels where both images painted (source alpha > 0) whose maximum visible RGB difference over the frame's `#fafaf9` backdrop exceeds `HEATMAP_THRESHOLD = 0` (literal painted frame-pixel difference; distinct unpremultiplied RGBA tuples that composite to the same frame colour remain identical). Overlap = 0 → the banner shows an em dash with a stated reason. A pixel only one version paints is not "changed", it is "missing in one" — and the summary banner's Size/Dimensions stats already carry that story. |
| 9 | **3.2 — The banner percentage rides a window event, because the banner is not in the component's scope.** `compare.tpl` renders the banner outside the `x-data` that includes `compareImage.tpl` (and renders it for every category, not just image), so the component dispatches `compare-pixel-diff` on `window` with `{ percent, overlapEmpty }`. A tiny registered Alpine store listens in JavaScript and the banner reads that store; compare templates deliberately carry no `@…window` listener markup. The banner stat is **not** a live region: it updates per repaint, and a repaint happens per drag frame. |
| 10 | **3.2 — Announcements are separate from the display, written on discrete events only.** A visually-hidden `aria-live="polite"` region mirrors `offsetAnnouncement`: written when the toggle completes, after a keyboard nudge, at drag end, when the wheel burst settles, and after scale/anchor/flip/reset changes and the completing size report — never once per `pointermove` or per `requestAnimationFrame`. The announce calls sit beside the existing `announceOffset()` calls, and a vitest corpus walks every writer so a new one that skips the announcement fails by name (package 2's stated lesson: apply the guarantee at every writer, enumerated, not at one). |
| 11 | **3.3 — Blink is play/pause + rate, toggle-mode only.** A "Blink" button (`aria-pressed`, starts stopped) and a native range input labelled "Blink rate in flashes per second" (2–8, step 1, default 4), both `x-show="mode === 'toggle'"`. Playing flips `showLeft` directly — not through `toggleSide`, whose click-suppression and `e.detail` logic is a pointer event's business. A manual click during playback just adds one flip. Above the WCAG three-flashes boundary, the box and image backgrounds flatten onto mid-grey and the images render through `contrast(8%)`: the 2–8 Hz timing remains available while the maximum possible luminance swing stays below the flash definition, and visible/help text states why contrast changed. |
| 12 | **3.3 — `prefers-reduced-motion` refuses to start and stops a running blink.** With the media query matching, the button is drawn `aria-disabled` with a title stating the reason (the aria-disabled-not-hidden rule package 1 decision 8 set). A `change` listener on the media query pauses a running blink and re-marks the control. Duck-typed so the unit suite (no DOM) gets "allowed". |
| 13 | **3.3 — Blink state is announced on the discrete events only**: start ("Blinking at N flashes per second"), pause, rate change. The flips themselves are the feature and are announced by nothing — a live region updated at 4–8 Hz is the pointermove mistake at the brief's own frequency. Leaving toggle mode pauses (one `$watch('mode')` in `init()`, which is also the heatmap's repaint hook). `destroy()` clears the interval and the rAF handle. |
| 14 | **HEIC/TIFF: 3.1 works, 3.2 refuses, 3.3 works.** Difference mode's box follows the `index.css` fallback branch like the other overlay modes and blends whatever paints. The Pixel diff button is `aria-disabled` with the scale/anchor wording ("One of the two versions reports no dimensions, so there is nothing to compare pixel by pixel") and its guard covers the forced press. Blink needs no geometry. An empty box or a NaN percentage is a bug; refusing with a stated reason is the answer. |
| 15 | **No persistence, no URL state** — package 1 decision 4 and package 2 decision 17, unchanged. The heatmap's mask and the blink both reset per visit. |
| 16 | **The heatmap mask paints in every overlay mode, difference included.** One rule rather than a carve-out per mode; the reader who finds it obscures the blend they chose toggles it off. Slider mode's handle keeps its `z-10`; the mask sits at `z-5`, `pointer-events: none`. |
| 17 | **Draw from the DOM `<img>` elements, not fresh fetches.** The eight `data-compare-image` elements hold both versions already decoded; `slotsForImage(img)` attributes each element to its slot. `Content-Disposition: attachment` on the version-file route affects navigation, not `<img>` rendering — the page already renders these URLs. Same-origin, so the canvases stay clean and `getImageData` works. |

## Constraints carried from the code

1. **`overlayScale` returns a fixed key set** (package 2 constraint 1). The heatmap reads from the same getters but writes nothing into style bindings, so it cannot break this — asserted once in a vitest to keep it that way.
2. **`_sizes`/`_urls` are indexed by the server's left/right**; `lead`/`trail` are derived from `swapped`. `heatMapPlacement(index)` is per *slot*, like `elementSize(index)`; the trail side is `this.swapped ? 0 : 1`.
3. **`_offset` is slot 1 relative to slot 0** and a flip mutates nothing; `trailOffset` derives the inverse. The heatmap reads `trailOffset`, never `swapped`-adjusted arithmetic of its own.
4. **The container key handler skips focusables and the radiogroup handler assigns state directly** — no new keys are introduced, so no new guard is needed. Blink owns no keys either.
5. **`aria-disabled` suppresses neither focus nor pointer events** — every refused control (Pixel diff on HEIC, Blink under reduced motion) gets a guarded handler tested via `click({ force: true })`.
6. **Vitest runs in the node environment** (no jsdom, no canvas). The unit-tested surface is the pure math (`heatMapPlacement`, the threshold/denominator counting, state transitions with fake timers and spied `setInterval`); the canvas plumbing is e2e-tested in a real browser.

## Work

### 3.1 — Difference blend (XS)

- `templates/partials/compareImage.tpl`: append `'difference'` to the mode radiogroup's `@keydown` list; add the fifth radio (icon: two overlapping squares, matching the 16×16 stroked set); add the difference-mode block after toggle mode.
- `public/index.css`: `.compare-overlay-img--difference { mix-blend-mode: difference; }` beside the other overlay rules.
- Component: nothing — `mode` already flows from the radiogroup.

### 3.3 — Blink comparator (S)

- State: `blinking`, `blinkRate` (default 4), `_blinkTimer`, `_reducedMotionQuery`, `_blinkAnnouncement`, `_blinkAnnounceParity`.
- Component methods: `toggleBlink()`, `_startBlinkInterval()`, `pauseBlink(announce)`, `blinkRateChanged()`, `announceBlinkState()`, `blinkReducedMotion` getter; `$watch('mode')` pauses when leaving toggle; `destroy()` clears.
- Template: play/pause button + rate range in the toolbar, `x-show="mode === 'toggle'"`.
- CSS: none beyond existing classes.

### 3.2 — Pixel-diff heatmap (M)

- Constants: `HEATMAP_SAMPLE_SIDE = 512`, `HEATMAP_CONFIRM_ABOVE_MEGAPIXELS = 12`, `HEATMAP_THRESHOLD = 0`.
- Pure math (vitest-covered): `heatMapPlacement(index)` → `{ x, y, w, h, k, dx, dy, originX, originY }` in box pixels; `heatMapSampleSize()`; `countChanged(lead, trail, threshold)` → `{ changed, overlap }` over two `Uint8ClampedArray`s.
- Canvas plumbing (e2e-covered): two reused scratch canvases, `getImageData` each; `_repaintHeatMapDom()` (mask ImageData → the canvas named by component mode, never by timing-sensitive rendered visibility); `_scheduleHeatMapRepaint()` (rAF-coalesced, with discrete announcements deferred until a completed paint).
- Gate: `heatmapNeedsConfirm` state, `toggleHeatMap()`, `confirmHeatMap()`; notice reuses `compare-diff-gate`.
- Writers enumerated (repaint + announce): `toggleHeatMap`, `confirmHeatMap`, `setScale`, `onScaleKeydown`'s callback, `toggleAnchor`, `swapSides`, `resetAlignment`, `handleAlignKey`, the align drag's `upHandler`, `announceOffsetWhenSettled`'s timeout, the mode `$watch`, and `noteSizeFrom`'s completing-report branch.
- Banner: event-driven stat span in `templates/compare.tpl` after the Dimensions stat.

## Test list (red first)

| Layer | What |
|-------|------|
| vitest (`imageCompare.test.ts`) | `heatMapPlacement` across scale × anchor × offset × flip (flip = placement matches `trailOffset`'s inverse); `countChanged` (identical → 0 changed, threshold boundary, alpha-only-painted-in-one excluded from both numerator and denominator, overlap 0); whole-pair guard (half-measured refuses, completing report computes); gate trigger; blink state transitions with fake timers (flips per tick, pause clears, reduced motion refuses, mode switch pauses, rate restart, destroy clears, flips never announce); the announcement corpus over every enumerated writer. |
| e2e (`e2e/tests/regressions/compare-difference-measurement.spec.ts`) | Difference mode: radio in the group with roving tabindex, computed `mix-blend-mode`, images stacked; an in-page canvas precondition proves the marked fixture pair is pixel-identical (so the mode's black is what a reader sees). Heatmap: identical pair → banner "0%", mask canvas clean; different pair → percent > 0 and the mask canvas's colored fraction agrees; offset nudge changes the percent and moves the mask; HEIC refuses with reason (forced click, no percent); gate notice on the large pair, "Compute anyway" computes; live region updates on keyboard nudge, not during drag. Blink: starts stopped; play alternates the visible side label; pause stops; rate 2 vs 8 Hz flip counts in a generous window; `emulateMedia({ reducedMotion: 'reduce' })` marks the control and refuses; pausing mid-play announces once; axe over the toolbar in all new states. |
| a11y suite | `c17-bh030-compare-view-a11y.spec.ts` unchanged and still green; new states covered by the new spec's axe scans. |
| Docs | `docs-site/docs/features/versioning.md`: Difference row in the image-modes table; a short "Pixel diff" paragraph (what the percentage means, the gate); a "Blink" row. No screenshot retake (package 1/2 precedent). |

New fixtures (committed): `compare-heatmap-large-v1.png` / `-v2.png` (~3000×2200 flat colour, distinct marker pixels so the hashes differ; ≈13.2 combined MP, over the gate). Generated with a throwaway Go program using `image/png` — no new dependency.

## Implementation review

- [x] 3.1 Difference blend implemented and browser-tested.
- [x] 3.3 Blink comparator implemented, including reduced-motion refusal and cleanup.
- [x] 3.2 pure placement/counting math implemented and unit-tested.
- [x] 3.2 canvas mask, banner bridge, size gate, HEIC refusal and discrete live-region behavior implemented.
- [x] Documentation updated; committed browser fixtures and rebuilt bundle included.
- [ ] Full SQLite, browser+CLI, a11y, Postgres and CSS gates.
- [ ] Two consecutive independent review rounds without major findings.

The final pre-commit re-measurement is unchanged: **3.1 XS, 3.2 M at the top of M, 3.3 S**. Implementation exposed two timing details inside the expected M work — Alpine's `x-show` can lag the first repaint, and a live-region update must wait for the rAF computation — but neither widened the interface or added an unbounded path.

**Independent review round 1 found majors, all addressed before the next round:** TIFF had stored geometry but no browser-decodable pixels, partial-alpha changes were not counted, sparse edits could round to `0%`, and rates above three flashes per second needed a flash-safe rendering. TIFF/HEIC are now refused by content type, alpha participates through the painted composite, nonzero shares below 0.1% render as `<0.1%`, and high-rate Blink flattens onto mid-grey through 8% contrast. The same pass changed the application threshold from the plan's subjective 32 to literal frame-pixel difference (`0`), added reverse-decode-order and 2 Hz/mid-play reduced-motion coverage, made toggle-off announce, and stopped scheduling disarmed heatmap frames. Existing compare e2e locators were narrowed for the fifth mode and second range input.

**Independent review round 2 also found majors, all addressed before round 3:** heatmap samples now compare the visible RGB composed over the real frame backdrop while retaining source alpha for the overlap rule; sparse formatting is symmetric at both endpoints (`<0.1%` / `>99.9%`); HEIC/TIFF media types are normalized and cover common aliases and parameters; the large-pair gate is a polite status and moves focus to **Compute anyway**; a paused Blink rate change is announced; and one test combines scale × anchor × offset × flip. Axe now scans the complete `.compare-toolbar` in Difference, active Blink, and armed Pixel-diff states, plus the visible gate. A browser screenshot regression proves identical partially transparent versions render black, rather than merely asserting the CSS declaration.

**Independent review round 3 found two standards majors, both addressed before round 4:** flash-safe Blink now hands the heatmap its grey backdrop and 8% contrast transform and schedules a discrete repaint when entering/leaving that rendering; a partially-transparent browser fixture proves a normal-frame 100% difference collapses to 0% under the filter and returns on pause. Both large-pair gate exits now return focus to **Pixel diff**, with the E2E walking dismiss and confirm. The refusal text and docs now state the durable policy (Pixel diff does not support HEIC/TIFF) instead of blaming the current browser decoder.

**Independent review round 4 found one spec major, addressed before round 5:** the exact refusal set omitted the registered `image/tiff-fx` subtype. Refusal is now a normalized media-family predicate covering `tif`/`tiff`, `heic`/`heif`, their `x-` forms, and hyphenated family subtypes such as TIFF-FX and HEIC sequences; the unit corpus enumerates TIFF-FX and vendor aliases.

**Independent review round 5: no majors.** Its two low notes were closed before round 6: the family table now pins HEIC, HEIF, sequence and `x-` positives plus near-match negatives, and the Scale/Anchor docs no longer claim all HEIC/TIFF pairs lack dimensions or conflate geometry availability with Pixel diff's policy.

## Order

| Batch | Contents |
|-------|----------|
| 0 | This plan, committed on the branch before any code. |
| A | 3.1, red e2e + vitest first. |
| B | 3.3, red vitest then e2e. |
| C | 3.2a: placement math + `countChanged`, red vitest first. |
| D | 3.2b: paint + percent + banner event + writers, red vitest then e2e. |
| E | 3.2c: gate + HEIC refusal + live-region discipline, red e2e. |
| F | Docs, `npm run build-js` with `public/dist` committed alongside, full gates. |

pi review after each batch; no batch closes on a review with majors.

## Gates

- `go test --tags 'json1 fts5' ./...`
- `npm run test:unit`
- `./scripts/css-scan-test.sh`
- `cd e2e && npm run test:with-server:all` (browser + CLI; read `.last-run.json` and a redirected log, never a piped `tail`)
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1` and `cd e2e && npm run test:with-server:postgres`
- No `.go` file is expected to change; the Postgres run runs anyway, said out loud now rather than skipped quietly.
