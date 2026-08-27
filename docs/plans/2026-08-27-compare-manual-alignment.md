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

- `alignStyle(index, elementW, elementH)` returns the transform for a slot:
  identity for slot 0, `_offset` for slot 1, and the inverse when `swapped`
  puts slot 0 on the trail side.
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

- `offsetLabel` renders `+12, -4, 103%`, reporting the clamped values.
- Reset button, `aria-disabled` at identity, clearing translate and scale and
  keeping the arming.
- A visually-hidden `aria-live="polite"` region, written on keyboard nudge and
  drag end only.

## Also from review

Round 22:

- **An operation owns exactly the axes it pushed out of bounds.** Rounds 20
  and 21 armed `_sizeOwed` by asking what an operation *touched*, which is
  broader than what it broke; the gap produced three defects of one shape, a
  claim recorded over an outside the operation did not create, which then
  suppresses the anchor of one a flip legitimately derives and lets resolution
  snap an axis the reader had placed. Reported by review: the flag was one
  boolean for both axes -- stored 800x600 both, actual 800x900 and 800x600,
  arm, zoom to 25%, nudge to 450, Flip (displayed -1800), slot 0 reports
  800x900, ArrowUp once, slot 1 confirms; the height-only report claimed x,
  though every width in the pair is 800 and the offset converts through the
  width ratio alone, so canonical 450 resolved to 300 one way and 450 the
  other. Found while fixing it: the same over-claim through the control
  operations -- nudge to 450, zoom to 25% (450 is at the bound before and at
  the smaller bound after, inside both times), Flip, ArrowUp: 450 back as 300.
  The rule is now the invariant rather than an approximation of it: an axis is
  owed exactly when an operation took it from inside its bound to outside.
  `_noteSizeOwed(before)` compares a per-axis snapshot taken before the
  operation against the ranges standing after it, and both arming sites go
  through it; `offsetWithinBound` is derived from the new per-axis getter, so
  the range arithmetic exists once. `offsetIsPlaced` stays in
  `_reboundOrDefer`'s early return -- redundant for the owed flag, not for the
  rebound debt an undisplaced correction would otherwise arm. Known corner,
  not chased: the flag does not clear when the reader walks an axis back
  inside during the window, so a flip-derived outside on that axis later in
  the same window still gets no anchor; no repro produces a wrong canonical
  offset from it, and it is the same family as round 20's accepted corner.
- Standards: the round-19 "one axis never resolves another" test used only
  confirming reports, so it never exercised a report that moves one axis's
  bound while the other is legitimately outside. Covered now.

Round 21:

- **A bound moved under nothing displaced breaks nothing, and owes nothing.**
  Round 20's `_sizeOwed` says where an outside display position came from -- a
  bound-mover put it there (owed, and the deferred rebound must pay it) or a
  flip derived it (legitimate, and the resolution must preserve it). Both
  arming sites armed unconditionally, including when the reader had not
  displaced the version at all; a zero translation is inside every range
  `translateRange` can produce, so nothing was broken and nothing could be
  owed, and the stale claim outlived the whole load window and suppressed the
  anchor of a genuinely flip-derived outside. Through the control operations
  (reported by review): stored 400x300 and 800x600, slot 0 confirms, press
  Anchor while nothing is corrected, arm, nudge to +600, Flip, ArrowDown once,
  slot 1 confirms -- the flipped -600 clamped to -300, canonical 600 rewritten
  to 300. Through the reports (found while fixing the first): stored 800x600
  both, arm, slot 0 reports 800x900, zoom to 25%, nudge to 450, Flip,
  ArrowDown once, slot 1 confirms -- the flip-derived -1800 clamped to -1200,
  canonical 450 back as 300. One predicate for both, `offsetIsPlaced`, which
  is deliberately not `offsetIsIdentity`: that is also false at a bare zoom,
  and a bare zoom displaces nothing either, so keying on identity would have
  left `zoomBy` arming through the same hole.
- Standards: no findings.

Round 20:

- **A drag increment is physical; a keyboard step is not.** Round 19 made the
  window's increments ride raw, which is right for a key press -- an abstract
  unit the window redefines as stored-box pixels -- and wrong for a drag,
  which hands `nudge` a distance the hand covered on the screen already
  converted into whichever transient box was standing. Recording that raw
  changes what it measures: stored 800x600 both, actual 600x800/1200x400, box
  rendered 600px, one report, drag 60 screen px -- slot-0-first resolved to
  dx=120 (the 60px asked for) and slot-1-first to dx=180 (90px), a visible
  jump when the second version decoded. `nudge` takes an internal third
  argument, `physical`, set only by the drag's move handler: a physical
  increment is recorded converted into the request's canonical pixels, so the
  resolution's conversion telescopes it back to the screen distance drawn.
  Converting keyboard steps the same way would reintroduce round 19 verbatim.
- **Resolution widens only around outside-ness that was legitimate at nudge
  time.** Round 19b widened each axis around the display's value *at
  resolution*, so an outside position the completing report had itself created
  -- it shrank or moved the trailing version, which is the debt that report
  armed -- was preserved as though it were a flip-derived inverse. Stored
  800x200/1200x100, actual 1200x800/100x400, one report, Shift+ArrowLeft x100:
  slot-0-first ended at -900 (entirely out of frame) and slot-1-first at
  -637.5, against a legal boundary of -625 both ways. Provenance cannot be
  read at resolution and `_sizeReboundDue` cannot answer it either -- a nudge
  sets that too, because it is the resolution trigger and not only the
  bound-repair debt. So a second flag, `_sizeOwed`, is armed exactly where a
  **bound-mover** arms the debt (size reports, and `_reboundOrDefer`'s deferred
  branch) and never by a nudge; every in-window nudge snapshots, per axis, the
  display's position when it is outside the transient range and nothing is
  owed, storing it slot-relative on the request as `ax`/`ay` so a flip carries
  it. Resolution clamps each axis to the final bound widened by its anchor, or
  plainly when there is none. Both orders now land on -625; the round-17/18/19
  repros are unchanged, an outward gesture from a flip-derived outside still
  moves nothing rather than snapping 600px inward, and an outside a report
  created is paid rather than laundered by the next nudge. Accepted corner: a
  flip between two in-window nudges that produces a *new* legitimate outside
  gets its anchor refreshed by the second nudge, since a nudge never arms
  `_sizeOwed` -- there is no case where it is silently dropped.
- **A missing stored size is not a missing conversion.** With no server
  dimensions for one version, `_storedBoxW` is null and both the creation and
  the resolution conversion fell back to 1 -- which reads as "no frame" rather
  than "this frame". Stored 0x0 and 800x600, slot 0 reports 400x300, one
  Shift+ArrowRight, slot 1 reports 1200x600: the displayed offset scaled with
  the box to 15 and the request pulled it back to 10, so the correction
  visibly shrank when the corrected width arrived. The request now captures
  its own canonical width at creation, `_requestUnitsW = _storedBoxW || box.w`,
  and every conversion reads that. With a stored box this is exactly the
  round-19 behaviour; without one the step means creation-box pixels and
  resolves to 15. No decode order diverges here -- with one stored side
  missing only one order can open a nudgeable window. Accepted corner: with no
  stored box, a further in-window report that changes the width between two
  nudges still records the later steps in creation-box units.
- Standards: no code-smell findings; a test-quality gap. The load-window
  suite covered keyboard nudges only, so drag conversion was tested only after
  the geometry settled, and there was no case of a clamped request whose
  trailing element changes size at the completing report, nor of a request
  with no stored box -- which is why 91 tests passed over all three defects.
  Five tests added, three of them red first.

Round 19:

- **A step means stored-box pixels, not standing-box pixels.** The request
  rode its increments through the box-width conversion at each report -- unit-
  faithful for a position that already lived in that box, but an increment was
  denominated in whichever transient box stood at the press. Stored 800x600
  both, actual 600x800/1200x400: one Shift+ArrowRight between the reports
  resolved to dx=15 slot-0-first (the 1.5x conversion) and dx=10 slot-1-first
  (the 1200-wide box already existed at the press). The request's translation
  is now denominated in **stored-box** pixels -- the one frame both decode
  orders share: a fresh request converts its base out of the transient box at
  creation, increments ride raw, box-width reports leave the request alone,
  and the deferred rebound converts it once, into the final box, when it
  resolves. Both orders land on 15; the round-17/18 repros converge unchanged
  (-450, -375) because there the stored and standing widths coincide. (The
  round-18 rule "the request scales on its own presence" is subsumed: the
  request now converts at resolution regardless of what the display shows,
  which is what that rule's walk-back case needed.)
- **One axis never resolves another.** The request is created whole from the
  displayed offset, so a y-only ArrowDown during the window carried the
  display's legitimately-outside flip-derived x (-1800 at k=4) into the
  request, and resolution clamped it to the final bound (-1200), rewriting
  canonical dx 450 -> 300. Each axis now resolves as a `boundedNudge` from
  where the display sits: an untouched axis keeps what it legitimately
  occupies, a swallowed inward increment still lands on the final boundary,
  and an outward one from outside moves nothing -- the same widening `nudge`
  itself applies. The widened nudge takes the request's unclamped value;
  clamping first and widening after reads the clamp's own pull-in as an
  inward gesture and snaps anyway.
- Standards: an orphaned JSDoc block duplicating `_reboundOffset`'s docs sat
  above `offsetWithinBound` (round-17 edit fallout). Deleted.

Round 18:

- **A remainder survives its walk home.** The deferred request rode through
  the box-width conversion inside `!offsetIsIdentity`, but display and request
  diverge exactly there: stored 800x600, actual 600x800, one report, nudge
  -1100 (clamped), nudge +600 (free) leaves an identity readout with -500
  still outstanding -- and the completing report then converted a non-identity
  sibling but skipped this one, so slot-0-first resolved to -450 and
  slot-1-first to -375. The request now scales on its own presence whenever
  the box width changes; both orders land on -375.
- **Reset said it could not clear what it would clear.** The button was drawn
  `aria-disabled` off `offsetIsIdentity` alone, while pressing it at an
  identity readout with a pending remainder would actually clear that
  remainder -- a reader who trusted the disabled state watched the second load
  move their image. `resetIdle` means no displayed correction AND no
  outstanding request, and clearing one announces: something changed.

Round 17:

- **A zoom clamped at the boundary arms no deferred debt.** `zoomBy` called
  `_reboundOrDefer` even when the clamp held `k` -- pressing + at 400% changed
  no geometry yet armed the debt, and the confirming report later paid it
  against alignment made after the no-op: with 100x100 placeholders, zoom to
  400%, nudge to -150, slot 0 confirms, press + again (already at the bound),
  flip, nudge to -56.25, slot 1 confirms -- -56.25 was rewritten to -37.5. The
  same no-op invariant the scale group carries now holds here: `zoomBy`
  computes `k` first and returns when the clamp held it.
- **A nudge during the load window keeps its request.** Nudging while exactly
  one version is measured clamped against transient geometry -- the trail is
  whichever slot decoded first -- so the same two files and input landed on
  different canonical offsets per decode order: stored 800x600 and actual
  600x800 both, one report, nudge fully left, other report lands dx=-450 when
  slot 0 reported first and -412.5 when slot 1 did, because the width
  conversion scaled an already-clamped value that a pull-in-only rebound could
  not restore. What is deferred is now split from what is shown. The display
  still obeys the transient bound at every instant -- the pair must stay
  watchable even if the second image never decodes -- but every increment the
  clamp swallowed rides onto `_requestedOffset`, which zoom factors and box
  conversions take alongside them, Reset retires with the correction, and
  which moves freely when the display does; the deferred rebound resolves that
  request against the final bound, so what arrives is where the reader asked
  to be bounded by the geometry both versions imply, whatever order they
  reported in.

Round 16:

- **The keyboard scale path defers like the mouse one.** `onScaleKeydown`'s
  callback rebounded against the current geometry directly, so a scale
  changed by arrow key between the two load reports clamped the transient
  trail and landed on a different offset than the same change by click:
  stored 100x100, actual 400x300/200x800, arm, anchor, zoom 200%, nudge to
  +50, report, ArrowRight to Fit, report -- lead-first finished at dy=150
  and trail-first at 200; clicking Fit gives 200 in either order. The
  callback now routes through `_reboundOrDefer`.
- **A no-op scale activation arms no deferred debt.** Pressing the
  already-selected policy changes no geometry, yet `_reboundOrDefer` armed
  the debt whenever the pair was half-measured -- so a later confirming
  report paid it against alignment made after the no-op: 100x100 stored and
  reported, zoom to 400%, nudge to -150, slot 0 confirms, click Relative
  (already selected), flip, drag to -56.25, slot 1 confirms -- the stale
  debt clamped it to -37.5. `setScale` returns when the value is unchanged
  (round 4 made the old early-return's repair path a no-op anyway), and the
  keyboard callback skips a selection that changed nothing.

Round 15:

- **A control used between the two load reports defers its rebound.** Zoom,
  scale and Anchor clamp against whatever geometry is current -- and while
  the pair is half-measured that geometry does not exist: the trail is
  whichever slot decoded first, so a 600x800 report against an 800x600
  placeholder clamps differently per order, and the second report's width
  conversion then scales the already-clamped value, which a deferred rebound
  can only pull further in. Stored 800x600, zoom to 25%, nudge to -430,
  report 600x800, Anchor, report 600x800: lead-first finished at -112.5 and
  trail-first at -84.375. The three controls now route through
  `_reboundOrDefer`, which clamps immediately outside the load window and
  owes the clamp to the both-measured moment inside it -- the same debt the
  size corrections pay.
- **Only the primary button drags.** A right-button `mousedown` started a
  drag and `preventDefault`ed it (suppressing the context menu); its release
  generates no click, yet stamped the toggle-click suppression, so a
  deliberate left click inside the window was eaten. Non-primary buttons now
  return before any of that.

Round 14:

- **A release announces only an actual drag.** The round 12 fix scoped the
  click suppression to real movement but left the announcement comparing the
  offset at press vs release -- so a load that converted the offset mid-press
  announced "Offset +80, 0, 100%" for a gesture that never moved, reading a
  conversion as the reader's own action. The live region is now written on
  `anyMove` like the suppression: keyboard nudges and actual drag ends only.

Round 12:

- **Disarming ends the drag, not just the next move.** Round 11's fix made
  the move handler end the gesture on observing the disarmed state -- lazy,
  so a reader who disarmed and *re-armed* without moving in between could
  resume the supposedly dead gesture. `toggleAligning` now removes the
  gesture's move listeners the moment it disarms; the enders stay, because
  the release still fires the button's click and the suppression has to
  recognise that click as the drag's.
- **The click suppression is armed by movement, not by an offset change.**
  `upHandler` compared the offset at press vs release, so a load that
  converted the offset mid-press -- no drag at all -- armed the suppression
  and ate the release-click, which was a tap the reader meant for the
  version switch. The move handler now records that it actually moved, and
  the stamp is armed by that.
- **The e2e assertion of the Reset control's enabled state checks absence**
  (`not.toHaveAttribute('aria-disabled')`), which `'true'`-comparison
  passed for the `"false"` value Alpine never leaves behind.

Round 11:

- **Disarming mid-drag ends it.** The move handler checked `aligning` at
  gesture start only, so Space on the focused Align button while the mouse
  was still held left the drag live -- decision 1 is that nothing nudges
  while disarmed. The next move now ends the gesture (teardown and one
  announcement if the offset moved) instead of moving the image.
- **The fake-DOM harness keeps the document and window registries apart.**
  One shared registry meant the teardown assertions could not tell a
  listener removed from the wrong target from one properly removed; the
  blur ender is on `window` and the drag listeners on `document`, which is
  exactly the target mix a regression could confuse.

Round 10:

- **Reset retires the debt with the correction that earned it.** The debt is
  the promise that a specific correction will be brought back once both
  versions report; Reset discards that correction but left the promise
  armed, so a later confirming report paid it against alignment the reader
  made *after* the reset: zoom to 25%, nudge to 450, slot 0 reports
  1600x1200 (convert to 900, arm), Reset, flip, nudge to +900, slot 1
  confirms -- the stale debt clamped +900 to +400. `resetAlignment` now
  clears the debt.
- **The onion-opacity range is not a text field.** The text-field guard
  matched plain `input`, so the range swallowed `+`, `-` and `R` while
  focused -- keys it has no use for, since a range answers only the arrows
  and Home/End. The guard is now `input:not([type="range"]), select,
  textarea`, and a unit test pins the distinction: R and = reach the armed
  alignment from the range, the arrows stay its own, and a real text field
  still owns every key.

Round 9:

- **A report that moves neither the offset nor its bound arms no debt.** The
  arming required a size change and a non-identity offset, but not that the
  change moved anything the offset depends on. A report that only resized
  the *leading* version without overtaking the box changed neither the
  offset nor the bound it sat in, yet armed the debt -- which a later
  confirming report then paid against alignment made after it: stored
  1600x1200 both, nudge to dx=1, slot 0 reports 800x600 (box unchanged,
  trail unchanged, no conversion, debt armed), flip, zoom to 25%, nudge to
  +850, slot 1 confirms -- the stale debt clamped +850 to +600. The arming
  now requires the report to have moved the box or the trailing version's
  element.
- **Comments explain the constraint, not the review.** The noteSizeFrom
  comment cited "the round 4 rule", a pointer into the review history;
  `docs/lessons.md`'s rule is that a comment must earn its place without
  that provenance. The constraint now stands alone.
- **The plan's Work section matched the implementation** (`alignStyle`, the
  comma-separated readout) rather than names the code never had.
- **The e2e spec deletes the category it creates**, so the fixture does not
  leak into the worker's shared ephemeral server.

Round 8:

- **A debt armed at identity never rewrites an alignment made later.** The
  arming condition required a size change but not a non-identity offset --
  and `within` is trivially true at identity, so the first size change armed
  the debt even though there was no offset for it to have moved. A later
  confirming report then paid it against alignment the reader had made
  *after* the change: stored 800x600, slot 0 reports 800x1200 (identity, no
  conversion, debt armed), arm Align, flip, zoom to 25%, nudge to +450,
  slot 1 confirms -- the stale debt clamps +450 to +300, a rebound
  correcting what no operation of its own broke. The arming now also
  requires the offset to be non-identity.

Round 7:

- **A deferred rebound is paid in the orientation that incurred it.** The
  debt round 6 scoped to size changes was still paid against the *current*
  orientation's bound. A flip landing between the size change and the
  confirming load made the pay clamp the flipped inverse and rewrite the
  canonical offset differently than the same measurements without the flip:
  stored 800x600, align to dx=450 at 25%, slot 0 reports 1600x1200 (convert
  to 900, arm the debt), flip, slot 1 confirms -- the inverse was clamped
  from -3600 to -2400 and the canonical from 900 to 600, where the flip-free
  order rebounds it to 850. The same two files landing on a different
  correction depending on when the reader happened to flip is the
  order-dependence the load-time rebound exists to remove. The pay now
  completes the correction in the orientation where the size change incurred
  it (the flip itself still never reapplies the bound), so both orders
  converge on 850 / -3400.
- **The stale `touch-action: none` comment was corrected.** Round 6 changed
  the CSS to `manipulation` but left the component's comment claiming `none`,
  misdocumenting the accessibility rule the change existed for.

Round 6:

- **The rebound debt is armed by a size change, never by a confirm.** Round
  5's debt was armed by any pre-completion report where the offset was
  inside -- including one that merely confirmed a stored size. A reader who
  then flipped would have the legitimately-outside inverse clamped by the
  next confirm: 800x600 stored, slot 0 confirms, align to dx=450 at 25%,
  flip, slot 1 confirms -- the inverse was rewritten from -1800 to -1200,
  and the original 450 to 300.
- **A report that fills both slots still reaches the rebound.** One image
  showing on both sides (two identical URLs) fills both slots in a single
  report, so the completing call is also the size change; the debt branch
  (which requires the pair to be incomplete) never armed, and the pay branch
  (which required the debt) never fired. A stored 400x800 pair aligned to
  dy=600, reported as 800x400, converted dy to 1200 against a bound of 300
  and left the version entirely outside the frame. The pay now also fires on
  a completing call that itself changed the size and moved an offset that
  was inside.
- **Modified keys stay the browser's.** Ctrl+R reloads, Ctrl+/- zooms the
  page and Alt+ArrowLeft goes back; while armed they instead reset, zoomed
  the image and nudged, and preventDefaulted the browser's own action. The
  alignment's shortcuts are the plain keys with Shift as their step
  modifier, so ctrl/meta/alt are refused in both entry points -- the
  container handler and the Align button's own, which bypasses it.
- **Pinch zoom stays available.** `.compare-box-aligning` used
  `touch-action: none`, which removed the reader's ability to magnify the
  page for as long as Align was armed -- the touchscreen equivalent of
  stealing Ctrl+wheel, which the wheel handler already leaves to the
  browser. Now `manipulation` (pan remains suppressed by the touchmove
  preventDefault), a two-finger touchstart never starts a drag, and a second
  finger landing mid-drag stops handling without preventing.
- **A drag converts both axes through the box's width.** The move handler
  converted the y-axis through the rendered *height*, a rounded integer, so
  on an extreme aspect ratio a vertical drag disagreed with the horizontal
  one about how many box pixels a screen pixel was -- the same mistake
  `noteSizeFrom` made and round 3 fixed. Both axes now use the width ratio.
- **A release outside the toggle button arms no suppression.** The click
  that follows a drag only fires when the release lands on the button;
  released outside, no click follows, and the stamp ate the reader's next
  deliberate click inside the window. The stamp now checks the release
  landed inside the box.

Round 5:

- **A load-time rebound is owed, not skipped.** The rebound waited on
  `_measured` becoming both-true, but the report that completes the pair may
  be one that only *confirms* a stored size: it flips `_measured` without
  touching `_sizes`, and the early return for "nothing changed" skipped the
  rebound the earlier report had set up. With the lead growing first and the
  trail then confirming its stored (smaller) size, the converted offset (600)
  stayed against the bound the smaller trail leaves (500), and the version
  sat entirely outside the frame. The debt is now recorded when a correction
  moves an offset that was inside the old bound, and paid the moment both
  versions have reported; `within` is still snapshotted before the correction
  (round 4's rule), so a flip-derived offset a report changed nothing for is
  never pulled back.
- **A touch drag never arms the toggle-click suppression.** `touchmove`'s
  `preventDefault` cancels that gesture's compatibility click, so no click
  can follow a touch drag -- and stamping the 400ms window anyway ate a
  deliberate tap after a drag, which was a tap the reader meant for the
  version switch. The stamp is now mouse-only.
- **The key guard is scoped to the keys a focused control owns.** The
  container handler's focusable-target guard ran before every alignment
  shortcut, so after clicking Flip or Anchor, `R` (and `+`/`-`) did nothing
  while focus sat on the button. The guard still refuses text fields every
  key and other focusables the arrows and Home/End they answer themselves,
  but `+`, `-` and `R` -- which no button or radio answers -- pass through to
  the armed alignment.

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
