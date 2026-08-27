# Compare page, Package 1: Registration & scale

Derived from the `/resource/compare` extension board (artifact
`49077fa1-1c51-47ce-89bd-0abef8d94460`), against master `415dd859`. Anchors
re-verified 27 Aug 2026: `imageCompare.js:19`/`:62`, `compareImage.tpl:12`/`:17`,
`index.css:2667`/`:2670`/`:2679` all still read as the board describes.

## The defect

The three overlay modes (slider, onion skin, toggle) share one box of
`max(w1,w2) x max(h1,h2)` and size each image inside it as a percentage of its
own intrinsic dimensions, centred by `margin: auto`. That is *true relative
scale*, which is one of three defensible policies and is hard-coded. A
1600x1200 rescan of an 800x600 original therefore draws at double size and
registers with nothing, and the interface offers no way to say so.

A pair with no stored dimensions is worse: `overlayRatio` returns null, the
`index.css:2679` branch puts both images back in the flow at one origin, and
there is no registration at all.

## Settled decisions

| # | Decision |
|---|---|
| 1 | All four board items in scope: 1.1 naturals, 1.2 Fit, 1.3 Stretch, 1.4 Anchor. |
| 2 | Loaded naturals **win** over stored dimensions once available, guarded on `naturalWidth > 0 && naturalHeight > 0`. Stored values are an SSR pre-load placeholder. |
| 3 | The `index.css:2679` fallback branch **stays**. HEIC/TIFF reach this comparator and no browser renders them, so those pairs never produce usable naturals and that branch remains load-bearing. The board's "the branch deletes itself" is wrong. |
| 4 | No persistence. Scale and anchor reset per visit, like `mode`, `swapped`, `sliderPos` and `opacity`. Scale policy is a property of the pair, not of the reader. |
| 5 | Scale is a second `compare-segmented-control` radiogroup; anchor is a single `aria-pressed` toggle beside Flip. No new keyboard code, no third roving-tabindex group for a binary choice. |
| 6 | Labels **Relative / Fit / Stretch**. `aria-label`s "Relative size", "Fit to frame", "Stretch to match, distorts aspect ratio" -- the visible `.compare-seg-label` is hidden below 768px, so the warning has to live in the accessible name. |
| 7 | **Relative stays the default.** Identical-dimension pairs render pixel-identically under Relative and Fit, so the default only decides the divergent minority, and no measurement says resolution-change beats crop there. One-line reversal later. |
| 8 | Controls that cannot act are not drawn as if they can: scale + anchor hidden in side-by-side (`x-show`), anchor `aria-disabled` in Stretch (no slack to anchor), both `aria-disabled` when the pair has no usable dimensions. |
| 9 | **`max x max` stays the box for all three modes.** It is flip-invariant and agrees with the board's "the larger's frame" whenever one image dominates in both axes, which is every real pair. `overlayRatio` is untouched. |
| 10 | The `resource_version_context.go` decoder gap is fixed in this branch (see item 0). |
| 11 | Both 1.1 fixtures: AVIF for absent dimensions, EXIF-rotated JPEG for disagreeing ones. Plus vitest over the fill guard. |
| 12 | `docs-site/docs/features/versioning.md` gains a scale-mode table. Screenshot **not** retaken -- the skill regenerates all thirty and the existing shot is still accurate about the four modes it shows. |
| 13 | Branch `feat/compare-registration-scale`, no push, no PR. Teardown plan bullet struck; artifact republished to its existing URL at the end. |

## Work

### 0. Go: the version path's missing decoders

`application_context/resource_version_context.go` imports only `image/gif`,
`image/jpeg`, `image/png`. The resource upload path imports webp, bmp and tiff
as well. Every **WebP version therefore stores `0x0`** -- `DecodeConfig` has no
decoder and `getDimensionsFromContent` returns zeros -- while the browser
renders WebP perfectly. Add the three blank imports.

Not retroactive: rows already written keep their zeros, which is one of the
reasons 1.1 exists rather than being replaced by this.

### 1.1 Fill `_sizes` from the loaded images

`noteLeadSize($event)` / `noteTrailSize($event)` on the component resolve the
**original** index through `swapped` before writing -- `_sizes` and `_urls` are
indexed by the server's left/right, while `leadUrl` is `_urls[swapped ? 1 : 0]`,
so a lead image records into slot 1 when swapped. Getting this backwards
transposes the box on every flip.

- Guard on `naturalWidth > 0 && naturalHeight > 0`. Rejects dimensionless SVGs
  and images that failed to decode.
- Write through a new array so Alpine re-renders the derived getters.
- Idempotent: eight `<img>` elements exist per pair (four modes x two sides,
  all kept in the DOM by `x-show`), so every side reports up to four times, and
  again on each flip when the swapped `:src` re-fires `load`.
- `init()` sweeps `img.complete && naturalWidth > 0` for images the browser had
  already finished before Alpine bound the handler -- `@load` never fires for a
  cached image that completed first, which would leave the whole feature silently
  inert on a second visit.

### 1.2 / 1.3 Fit and Stretch

`scale` state, `['relative', 'fit', 'stretch']`, default `'relative'`.
`overlayScale(index)` gains two branches ahead of the existing arithmetic:

- Unknown sizes -> `''` first, whatever the mode. The `2679` fallback branch owns
  the layout there, and an inline `height:100%` would beat its `height:auto` and
  break a case that currently works.
- `fit` -> `width:100%;height:100%;` and let the existing `object-fit: contain`
  do the rest. For a pure resolution change both aspect ratios equal the box's,
  so neither letterboxes and registration is exact.
- `stretch` -> the same plus `object-fit:fill;`, overriding the class. Inline
  rather than a modifier class so the whole scale policy stays in one function.

`overlayRatio` is not touched.

### 1.4 Anchor

`anchor` state, `'center' | 'top-left'`. Two mechanisms, one intent, appended by
`overlayScale`:

- Relative: `margin:0;` against the class's `margin:auto`. With `inset:0` plus
  explicit width/height this is over-constrained, and CSS resolves it by
  ignoring `right`/`bottom` in LTR -- top-left.
- Fit: `object-position:0 0;` against the initial `50% 50%`, which is where the
  letterbox slack lives once both images are at 100%.
- Stretch: no slack exists, so the control is `aria-disabled` rather than drawn
  as if it does something.

## Control surface (`templates/partials/compareImage.tpl`)

A second radiogroup and one toggle, both `x-show="mode !== 'side-by-side'"`,
driven by the existing `onRadiogroupKeydown($event, 'scale', [...])`. Icons
match the existing four: 24x24 stroked outlines -- nested rects (relative), a
rect with inward corner marks (fit), a rect with outward arrows (stretch).

Unavailability is `aria-disabled` plus a guarded handler, never the `disabled`
attribute: `disabled` removes a `role="radio"` from the tab order and breaks the
roving-tabindex invariant the group depends on.

**The guard has to cover the keyboard path, not just the click.**
`onRadiogroupKeydown` assigns `this[stateKey]` directly, so arrow keys, Home and
End would change the scale while the group announces itself disabled. The refusal
belongs in front of that handler as well as in front of `@click`.

Surveyed by surface rather than by file: `templates/compare.tpl:248` is the only
`{% include %}` of this partial anywhere under `templates/`, so these controls
appear on exactly one page.

CSS: one rule, `.compare-seg-btn[aria-disabled="true"]`. Everything else reuses
existing classes.

## Tests, red first

| Layer | What |
|---|---|
| Go | A WebP version stores real dimensions. Red before item 0. |
| vitest (`src/components/imageCompare.test.ts`) | `overlayScale` across three modes x two anchors x missing sizes; `noteSize` guard, idempotence, and index resolution under `swapped`. |
| E2E (`e2e/tests/regressions/compare-registration-scale.spec.ts`) | Rendered geometry, not style strings -- a style assertion passes on a style the `2679` branch overrides, which is exactly the failure worth catching. |
| a11y | axe over the extended toolbar; roving tabindex across the new group. |

E2E assertions:

- Same-aspect pair at two resolutions: the two overlay images' `boundingBox()`es
  coincide within a pixel in Fit, and differ in Relative.
- Stretch: both boxes equal the container's.
- Anchor top-left: the smaller image's `x`/`y` equal the container's; Centre:
  strictly greater.
- AVIF pair (`image/avif` is in `RasterImageContentTypes` and **no Go decoder
  exists anywhere in the tree**, so it stores `0x0` permanently -- this fixture
  survives item 0 forever): registers instead of stacking at one origin.
- EXIF-rotated JPEG pair: the box is built from the browser's transposed
  dimensions, not the stored ones. This is the only assertion that distinguishes
  "naturals win" from "fill only the gaps".
- Side-by-side: both new controls are **hidden**, asserted with `toBeHidden()`.
  `x-show` sets `display:none` and leaves the elements in the DOM, so a
  count-zero assertion would red forever against a correct implementation.

**Two premises to settle empirically before writing those last two specs**, not
to assume: that Playwright's bundled Chromium decodes AVIF, and that it
transposes `naturalWidth` for EXIF orientation. If the second does not hold,
that fixture is dropped and said so, not worked around.

New assets: a same-aspect PNG pair at two resolutions, an AVIF pair, an
EXIF-rotated JPEG pair.

## Order

| Batch | Contents |
|---|---|
| 0 | This plan file, committed on the branch before any code. |
| A | Item 0, red Go test first. |
| B | 1.1, red E2E (AVIF) first. |
| C | 1.2 + 1.3 + the scale radiogroup. |
| D | 1.4 + the anchor toggle. |
| E | Docs, teardown-plan bullet, artifact republish. |

pi review after each batch; no batch closes on a review with majors.

## Gates

- `go test --tags 'json1 fts5' ./...`
- `npm run test:unit`
- `npm run build`, with the `public/dist` diff committed alongside its source change
- `./scripts/css-scan-test.sh`
- `cd e2e && npm run test:with-server:all`
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
  and `cd e2e && npm run test:with-server:postgres`
