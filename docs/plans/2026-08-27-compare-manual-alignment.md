# Compare page, Package 2: Manual alignment

Derived from the `/resource/compare` extension board (artifact
`49077fa1-1c51-47ce-89bd-0abef8d94460`), package 2, items 2.1 and 2.2. Anchors
re-verified 27 Aug 2026 against `feat/compare-manual-alignment`'s base
`e4319332`: `imageCompare.js:153` is `setScale`, `:77` is `overlayScale`,
`compareImage.tpl:12` is the mode-selector toolbar. The board's own line numbers
were taken against `415dd859` and have drifted through package 1; the file names
have not.

## The defect

Package 1 gave the overlay modes a scale policy: `relative`, `fit` and
`stretch`, with a corner anchor. All three are *whole-pair* policies computed
from intrinsic dimensions. None of them can act on a pair that is the same size
and simply not in register -- a flatbed scan placed differently on the glass, a
handheld re-photograph of a document, a screenshot taken at a different scroll
offset. The subject sits eleven pixels to the left, every automatic policy
agrees the two images are already correctly sized, and the interface offers
nothing.

The residual scale case is the same shape: a re-photograph at 103% of the
original is not a resolution change (`fit` does nothing, because the intrinsic
dimensions are identical) and not an aspect change (`stretch` does nothing
either). Only the reader can say how much.

## Settled decisions

Settled with the user across three rounds before any code was written.

| # | Decision |
|---|---|
| 1 | An armed **Align** toggle (`aria-pressed`, beside Flip and Anchor) owns drag and the arrow keys. Nothing nudges while disarmed. Chosen over a modifier-drag (undiscoverable, and Alt-drag belongs to the window manager) and over drag-anywhere-but-the-handle (a three-pixel miss silently moves an image, which is the invisible-state failure 2.2 exists to prevent). It is also the only option that disambiguates the arrow keys, which `_keyHandler` already spends on `sliderPos` and `opacity`. |
| 2 | The offset is stored **on the version slot** -- one `{dx, dy, k}` describing slot 1 relative to slot 0 -- never on "trail". Same indexing as `_sizes` and `_urls`. |
| 3 | **Flip inverts** rather than resets: the applied transform becomes `translate(-dx/k, -dy/k) scale(1/k)` when `swapped`. The alignment survives an A/B, which is what a flip is for. |
| 4 | Units are **box pixels** -- the intrinsic units of the `max(w1,w2) x max(h1,h2)` box -- emitted into the transform as percentages of the element. Resize-invariant, and the same coordinate space every other number in the component lives in. Cost accepted: on a 4000px scan rendered 800px wide one arrow press is a fifth of a screen pixel, so Shift-10 is the usable keyboard step there. |
| 5 | Arrow keys nudge 1 box px, Shift 10. Drag maps rendered pixels into box pixels through the box's own measured width. |
| 6 | **Scale ships here**, not deferred to 4.1. `+` / `=` and `-` at 1%, Shift 10%; wheel over the box while armed, `preventDefault`ed. Clamped **25%-400%**. No scale-drag -- drag is spent on translate and a modifier-drag reopens decision 1. |
| 7 | `transform-origin` **follows the anchor**: centre by default, `0 0` under `anchor === 'top-left'`. Scaling from the centre after the reader has asserted "these line up at the corner" walks that corner off by half the scale change. |
| 8 | Translate is bounded so **a quarter of the moved version stays in frame**, on `nudgeSlider`'s 1-99 precedent (`imageCompare.js:402`): a state that shows nothing looks broken. The readout reports the bounded value, so the number on screen is the number in effect. **Corrected after review:** the first implementation used half the *box*, which does not hold that promise -- a small version anchored to the corner of a large box clears the frame entirely well before it has travelled half a box. The bound is a fraction of the version's own rendered size, and it is anchor-aware. |
| 8b | The bound is **re-applied after zoom, scale and anchor** (`_reboundOffset`), each of which moves it while leaving the offset where it was -- `dx 600` in an 800 box renders at x = 900..1100 the moment the reader zooms to 25%. A **flip does not** re-apply it: the other three are the reader changing something and being answered, while a flip's whole purpose is to show the same alignment the other way round. Found in review round 2. |
| 8c | An offset is **converted when the box's own dimensions are corrected** (`noteSizeFrom`), since it is measured in that box's pixels. A reader who nudged against the stored placeholder would otherwise watch their correction change size when the real dimensions arrived. Found in review round 2. |
| 8a | A nudge from **outside** the bound may only move inward, never snap. A flip derives the inverse, and the inverse of an extreme correction is legitimately outside this side's bound -- `dx 600` at 25% inverts to `-2400` at 400% -- so clamping on the first arrow press moved the image by the whole difference instead of by one pixel. Implemented by widening the range to include wherever the value already is, **not** by "the nudge must reduce the distance", which round 2 showed lets a large delta overshoot through the range and out the far side. Found in review rounds 1 and 2. |
| 9 | The Align button **handles its own arrow keys**, calling the same `nudge()` the container handler does. `_keyHandler` skips events targeted at anything focusable on the grounds that "anything focusable answers its own arrow keys"; giving the button its own handler satisfies that rule instead of carving an exception out of it. The interlock is `_keyHandler`'s existing `if (e.defaultPrevented) return`. |
| 10 | Readout and Reset are **always present** in the overlay modes (`0, 0 - 100%` at rest, Reset `aria-disabled` at identity), hidden in side-by-side by the same `x-show` as scale and anchor. Package 1 decision 8, unchanged: a control that cannot act is marked, not removed. |
| 11 | The visible readout updates continuously; a **separate visually-hidden `aria-live="polite"` region** is written on each keyboard nudge and on **drag end only**. A live region updated per `pointermove` produces a queue that reads for minutes. |
| 12 | `R` and Reset clear **translate and scale together**. `R` is armed-only and guarded against firing from a text field, like the container handler. Reset **keeps** the arming: the reason you reset is that you are about to try again. |
| 13 | Arming survives a mode switch, a Flip and a Reset. Nothing auto-disarms -- alignment is iterative, and re-arming between presses turns a two-second correction into a click per pixel. |
| 14 | Align stays live under **all three** scale policies, Stretch included. Anchor's refusal under Stretch is a statement of fact (no slack, so zeroing a centring margin provably changes nothing); translate acts under every policy. It inherits only the `scaleAvailable` refusal. |
| 15 | **Refused when `scaleAvailable` is false**, reusing that predicate rather than a second copy. Under the `index.css:2679` fallback the lead is `position: static` and the trail is pinned to the top with `margin: 0`, so the two are not in a shared coordinate space at all -- and the formats that actually reach it (HEIC, TIFF) paint nothing to align. |
| 16 | Visible label **"Align"**; accessible name **bound to the version**, `'Nudge and zoom ' + trailLabel`. Flip exchanges which file is trailing, so a fixed "the newer version" is wrong half the time -- the mistake that produced the whole `lead`/`trail` design (`imageCompare.js:20-24`). |
| 17 | No persistence and no URL state. Package 1 decision 4 and the board's own note that 5.1 owns view state. |
| 18 | Branch `feat/compare-manual-alignment`, no push, no PR. Postgres suite skipped and said out loud: if this lands as scoped, no `.go` file changes and no SQL path can regress. |

## Constraints carried from the code

Not decisions -- properties of the file that the implementation has to respect.

1. **`overlayScale` returns a fixed key set.** Alpine's object style binding sets
   only the properties it names, so a key dropped between two renders keeps
   whatever it was last set to. `transform` and `transformOrigin` must therefore
   appear in *every* return path, including the no-dimensions early return and
   the `stretch` branch, as `''` where they do not apply.
2. **The drag surface is the overlay box itself.** In slider mode the trail
   `<img>` is `pointer-events-none` and the lead sits inside a
   `pointer-events-none` clip wrapper, with the slider handle above at `z-10`.
3. **The transform applies in all three overlay modes**, toggle included.
   Blinking a nudged pair is how a reader checks the registration took.
4. **`_sizes` and `_urls` are indexed by the server's left/right**, while
   `leadUrl` is `_urls[swapped ? 1 : 0]`. Decision 2 exists because getting this
   backwards transposes the offset on every flip -- the same trap package 1's
   plan flagged for the box.
5. **The container key handler skips any focusable target**, on the grounds that
   a focusable element answers its own arrow keys. Arming does not and must not
   change that: the scale radiogroup navigates with arrows, and claiming them
   ahead of the skip would break it. So "while armed the arrows nudge" holds
   where no other control has focus, and on the Align button itself, which
   carries its own handler for exactly that reason. The docs say this rather
   than the stronger thing.

## Work

### 2.1 Nudge and zoom the trail image

State on the component: `aligning` (armed), and `_offset = { dx, dy, k }` in box
pixels and a scale factor, describing slot 1 relative to slot 0.

- `alignTransform(index)` returns the transform for a slot: identity for slot 0,
  `_offset` for slot 1, and the inverse when `swapped` puts slot 0 on the trail
  side.
- Folded into `overlayScale`, so one style object carries sizing, anchoring and
  alignment, and constraint 1 holds by construction.
- `nudge(dx, dy)` and `zoomBy(dk)` clamp per decisions 6 and 8.
- Pointer drag on the overlay box while armed, reusing `startSliderDrag`'s
  teardown shape: `mousemove`/`touchmove` on the document, ended by `mouseup`,
  `touchend`, `touchcancel` and `window.blur`, with the ender kept on the
  component so `destroy()` can end a drag the page is leaving mid-gesture.
- Wheel on the box while armed, `preventDefault`ed. The box is a `div`, so the
  listener is non-passive by default and the prevent takes. `Ctrl`/`Meta`+wheel
  is left alone -- it is the browser's own magnification, and a trackpad pinch
  arrives as exactly that -- and a `deltaY` of 0 (a sideways swipe) is ignored
  rather than read as "not up" and answered by zooming out. The announcement
  waits for the burst to stop, which is the wheel's form of announcing at the
  end of a drag. All three found in review.

### 2.2 Offset readout and reset control

- `offsetLabel` renders `+12, -4 - 103%`, reporting the clamped values.
- Reset button, `aria-disabled` at identity, clearing translate and scale and
  keeping the arming.
- A visually-hidden `aria-live="polite"` region, written on keyboard nudge and
  drag end only.

## Also from review

Round 4:

- **A rebound corrects what its own operation broke, never what was already
  true.** Round 3 removed `setScale`'s early return so a repair could not be
  blocked; that let a no-op activation -- clicking the already-selected policy,
  or `Home` on the selected radio -- clamp a flip-derived offset and rewrite the
  reader's original number. `offsetWithinBound` is snapshotted before each of
  the five operations that move the bound, and only an offset that *was* inside
  is pulled back. Nothing but a flip can leave one outside, so refusing to
  repair is refusing to repair a state no path produces.
  - The corner this leaves, known and accepted: from a flip-derived offset that
    is outside, an anchor round trip is lossy, because under `top-left` that
    offset is legal and centring again genuinely breaks the bound. Pinned by
    `an operation that does move the bound still pulls a flipped offset in`.
    The alternative -- never rebounding while flipped -- costs the guarantee in
    the view the reader is actually looking at, which is worse.
- **The load-time rebound waits for both versions to report.** The box is `max`
  of the two, so while one side is a real measurement and the other still the
  stored placeholder it describes a pair that does not exist; clamping against
  that transient made the same two files land on a different offset depending on
  which decoded first. `_measured` tracks reporting rather than *changing*,
  because a load confirming the stored size writes nothing and still means that
  version has been measured.
- **`touchstart` no longer `preventDefault`s.** It suppresses the synthesized
  `click`, so while armed a *tap* on toggle mode's button did nothing at all.
  `touch-action: none` is what stops the page scrolling, and the first
  `touchmove` prevents as well; `mousedown` still prevents, because there it
  stops a native image drag taking the gesture over.

Round 3:

- **The keyboard path never reached the rebound.** `onScaleKeydown` delegates to
  the generic `onRadiogroupKeydown`, which assigns `this.scale` directly and
  never touches `setScale`, so a scale changed by arrow key kept an offset the
  new sizing had put outside the frame. The rebound is passed as a callback, and
  `setScale`'s early return for "already selected" is gone: it meant pressing
  the already-selected policy could not repair such a state.
- **The box is width-driven, so a box pixel is isotropic.** The conversion in
  `noteSizeFrom` scaled `dx` and `dy` by their own axis ratios, which is wrong:
  the box takes the container's width and derives its height from
  `aspect-ratio`, so one box pixel is `renderedWidth / box.w` on screen in
  *either* axis. Correcting only the height moves no physical distance at all
  and the offset was doubled. Both components scale by the width ratio now, and
  the rebound runs afterwards because the versions changed size too.
- **Only a drag that ends in toggle mode arms the click suppression.** Toggle's
  frame is a button and the click ending a drag across it would also switch
  versions; a drag in onion skin produces no such click, so stamping there
  swallowed the first click after a mode switch.
- The drag path is now unit-tested against minimal DOM stubs. This suite runs
  with no DOM, so it had been reachable only end-to-end -- which left its own
  rules (announce once at the end; arm the suppression only where that click can
  happen) pinned by nothing that runs in a second.

Rounds 1 and 2:

- The toggle-mode click suppression stamps only a gesture that **changed the
  offset**, and is spent on the one click it was for. A flag the next click
  cleared reached forward into an unrelated one.
- `announceOffset` cancels a pending wheel-settle announcement, which would
  otherwise fire after a Reset and announce a value the reader had moved past.
- Controls marked `aria-disabled` now **look** unavailable
  (`index.css`): the attribute was set and nothing rendered differently, so a
  refused Align, scale radio or Anchor invited a press with a normal cursor and
  a hover response. Their text colour is deliberately *not* faded -- they stay
  focusable and are read aloud, so dimming would trade a measured contrast ratio
  for a visual hint. This corrects package 1's controls as well as this one's.

## Verification

- **vitest** over the transform arithmetic: the flip inversion, both clamps, the
  fixed key set including the two new keys, the `scaleAvailable` refusal, and
  that a scale-policy change leaves the offset alone.
- **e2e** (`compare-manual-alignment.spec.ts`) over rendered geometry, never the
  inline style strings -- same reasoning as the package 1 spec, whose comment
  explains that the `index.css` fallback branch can override those styles
  wholesale and a style assertion would pass on a page drawing the pair
  completely wrong.
- `go test --tags 'json1 fts5' ./...`, `npm run build-js` with `public/dist`
  committed alongside the source change, and `npm run test:with-server:all`.
- `docs-site/docs/features/versioning.md` gains the alignment controls beside
  the scale table. No screenshot retake: the skill regenerates all thirty and
  the existing shot is still accurate about what it shows.
- The board is republished to its existing URL with 2.1 and 2.2 marked shipped.
