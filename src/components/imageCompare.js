// The keys the WAI-ARIA radiogroup pattern owns. Named once because two places
// need the same set: the handler that acts on them, and the refusal that must
// still swallow them.
const RADIOGROUP_KEYS = ['ArrowRight', 'ArrowLeft', 'ArrowUp', 'ArrowDown', 'Home', 'End'];

// Manual alignment bounds and steps, in box pixels and scale factors.
//
// The translation is bounded so that a quarter of the moved version stays in
// the frame, for the reason `nudgeSlider` clamps to 1-99 rather than 0-100: a
// state that shows nothing at all reads as a broken page rather than as a
// choice the reader made. It is a fraction of the **version**, not of the box,
// because those come apart exactly where it matters -- a small version in a
// large box, anchored to the corner, clears the frame entirely long before it
// has travelled half a box.
const ALIGN_KEEP_VISIBLE = 0.25;
// The zoom range is deliberately reciprocal (0.25 = 1/4), so the inverse a flip
// derives is always inside it too and never has to be brought back.
const ALIGN_ZOOM_MIN = 0.25;
const ALIGN_ZOOM_MAX = 4;
// How long after an alignment drag a click on the toggle-mode button is read as
// the end of that drag rather than as a request to switch versions.
const ALIGN_CLICK_SUPPRESS_MS = 400;
// A wheel gesture arrives as a burst of events. The announcement waits for the
// burst to stop, the way a drag's waits for the pointer to come up.
const ALIGN_ANNOUNCE_SETTLE_MS = 200;
const ALIGN_STEP = 1;
const ALIGN_STEP_LARGE = 10;
const ALIGN_ZOOM_STEP = 0.01;
const ALIGN_ZOOM_STEP_LARGE = 0.1;

// The blink comparator's rate range, in flips per second. Named where the
// range input's bounds and the announcement read them, so the three cannot
// drift.
const BLINK_RATE_MIN = 2;
const BLINK_RATE_MAX = 8;
const BLINK_RATE_DEFAULT = 4;
// Kept in lockstep with `.compare-box--flash-safe` in public/index.css.
const BLINK_FLASH_BACKDROP = [128, 128, 128];
const BLINK_FLASH_CONTRAST = 0.08;

// The zero-width space that makes a repeated announcement a *changed* string.
// `aria-live` fires on text change, so nudging away from a value and back would
// otherwise land on the text already in the region and announce nothing. It is
// alternated rather than accumulated, which is all the property requires: two
// consecutive announcements always differ. Screen readers do not voice it.
const ANNOUNCE_MARK = '\u200B';

// The pixel-diff heatmap's constants. The sample cap is what bounds the work:
// whatever the source images measure, the compose canvases never exceed this
// side, so a repaint costs well under a megapixel of getImageData. The gate is
// textDiff's CONFIRM_ABOVE_BYTES analogue -- a courtesy ask before spending
// decode-and-downsample work on a very large pair, not an enforced bound.
const HEATMAP_SAMPLE_SIDE = 512;
const HEATMAP_CONFIRM_ABOVE_MEGAPIXELS = 12;
// Pixel diff is literal after composition: any channel change counts. An
// identical pair still measures exactly zero because identical bytes take the
// identical placement and sampling path.
const HEATMAP_THRESHOLD = 0;
// `.compare-overlay-box`'s painted backdrop. Source alpha participates in the
// visible RGB through this colour while source alpha itself still decides the
// overlap denominator.
const HEATMAP_BACKDROP = [250, 250, 249];
function normalizedContentType(value) {
  return String(value || '').split(';', 1)[0].trim().toLowerCase();
}

function heatMapUnsupportedContentType(value) {
  const type = normalizedContentType(value);
  // Refuse the media families, not only today's common spellings. TIFF-FX is
  // a registered TIFF subtype, and sequence/vendor-prefixed HEIF forms belong
  // to the same durable Package 3 refusal contract.
  return /^image\/(?:x-)?(?:tiff?|heic|heif)(?:$|-)/.test(type);
}

/**
 * Compare the two composed frames: per-pixel, count the overlap and mark the
 * changed pixels.
 *
 * Denominator: pixels both versions painted (alpha > 0). A pixel only one
 * version paints is not "changed" -- it is "missing in one", which the
 * summary banner's Size and Dimensions stats already tell. Numerator: overlap
 * pixels whose maximum per-channel absolute difference exceeds the threshold.
 *
 * Returns `changed` and `overlap` counts plus the finished mask bytes (RGBA,
 * the mask colour where changed, transparent elsewhere) so the canvas half
 * paints exactly what this counted -- one pixel loop, not two.
 */
export function heatMapDiff(lead, trail, threshold, rendering = {}) {
  let changed = 0;
  let overlap = 0;
  const n = Math.min(lead.length, trail.length);
  const mask = new Uint8ClampedArray(n);
  const backdrop = rendering.backdrop || HEATMAP_BACKDROP;
  const contrast = rendering.contrast ?? 1;
  const painted = (data, index, channel) => {
    const composite = Math.round((
      data[index + channel] * data[index + 3]
      + backdrop[channel] * (255 - data[index + 3])
    ) / 255);
    // CSS contrast pivots around half intensity after the element has painted
    // its source over its own background.
    return Math.max(0, Math.min(255, Math.round((composite - 127.5) * contrast + 127.5)));
  };
  for (let i = 0; i < n; i += 4) {
    if (lead[i + 3] === 0 || trail[i + 3] === 0) continue;
    overlap++;
    // Compare what the two RGBA samples visibly paint over the frame's real
    // backdrop, not their unpremultiplied storage. Different RGBA tuples can
    // produce the same frame pixel, and the heatmap must call those identical.
    const d = Math.max(
      Math.abs(painted(lead, i, 0) - painted(trail, i, 0)),
      Math.abs(painted(lead, i, 1) - painted(trail, i, 1)),
      Math.abs(painted(lead, i, 2) - painted(trail, i, 2)),
    );
    if (d > threshold) {
      changed++;
      mask[i] = 255;
      mask[i + 1] = 0;
      mask[i + 2] = 230;
      mask[i + 3] = 170;
    }
  }
  return { changed, overlap, mask };
}

/** The counts alone -- the shape the percentage is computed from. */
export function countChanged(lead, trail, threshold) {
  const { changed, overlap } = heatMapDiff(lead, trail, threshold);
  return { changed, overlap };
}

/** A sparse real edit must never wear the identical pair's `0%` label. */
export function formatHeatMapPercent(changed, overlap) {
  if (!overlap) return null;
  if (!changed) return 0;
  const percent = (changed / overlap) * 100;
  if (percent < 0.1) return '<0.1';
  if (changed < overlap && percent > 99.9) return '>99.9';
  return Math.round(percent * 10) / 10;
}

/**
 * Canvas transform for the CSS placement `translate(dx, dy) scale(k)`.
 *
 * CSS applies the scale about the chosen origin but does not scale the
 * translation that precedes it. Keeping this arithmetic pure makes that
 * ordering explicit -- a sequence of canvas `translate`/`scale` calls makes
 * it deceptively easy to multiply the reader's correction by `k`.
 */
export function heatMapCanvasTransform({ k, dx, dy, originX, originY }, sx, sy) {
  return [
    k, 0, 0, k,
    (originX * (1 - k) + dx) * sx,
    (originY * (1 - k) + dy) * sy,
  ];
}

/** The CSS transform that undoes `t`: for T(p) = k*p + d, T-inverse(q) = (q - d) / k. */
function invertOffset(t) {
  return { dx: -t.dx / t.k, dy: -t.dy / t.k, k: 1 / t.k };
}

/**
 * Whether a translation sits inside the range its geometry allows.
 *
 * With slack, because the answer decides PROVENANCE and not appearance: a
 * value on its boundary is reached by different arithmetic depending on which
 * version decoded first -- the same correction arrives as 725 down one path
 * and 725.0000000000001 down the other -- and an exact comparison calls one
 * of those inside and the other already outside. A later operation then
 * repairs one order and not the other, and the two diverge by hundreds of box
 * pixels from a difference of one ulp.
 *
 * Scaled to the magnitudes involved rather than absolute, since a box can be a
 * few thousand pixels across and the ranges scale with it. At 1e-6 the slack is
 * thousandths of a pixel at any size this component can produce -- far below
 * anything visible, and far above the rounding it exists to absorb.
 */
const BOUND_EPSILON = 1e-6;

function withinRange(value, range) {
  const slack = BOUND_EPSILON
    * Math.max(1, Math.abs(range.min), Math.abs(range.max), Math.abs(value));
  return value >= range.min - slack && value <= range.max + slack;
}

/**
 * Add `delta` to `current` without snapping a value that is already outside.
 *
 * A flip derives the inverse of the reader's correction, and the inverse of an
 * extreme correction is legitimately more extreme than this side's own bound --
 * `dx = 600, k = 0.25` inverts to `dx = -2400, k = 4`. Clamping that on the
 * first arrow press would move the image by the whole difference instead of by
 * one pixel, silently destroying an alignment the reader had made.
 *
 * So the range is widened to include wherever the value already is. From
 * outside, a nudge travels toward the range and stops at its near edge -- it
 * never refuses an inward gesture, and never overshoots through the range and
 * out the far side, which a "must reduce the distance" rule allows: from 100
 * against a range of [-10, 10], a delta of -150 satisfies that rule at -50.
 */
function boundedNudge(current, delta, range) {
  const low = Math.min(range.min, current);
  const high = Math.max(range.max, current);
  return Math.max(low, Math.min(high, current + delta));
}

/**
 * `invertOffset` for a translation request, carrying its per-axis anchors.
 *
 * An anchor is a position along one axis, so it inverts exactly the way `dx`
 * does -- and it has to, or a flip landing between the nudge that recorded it
 * and the report that resolves it would leave the anchor describing the other
 * side. `null` means that axis has no anchor, and stays null through any
 * number of flips.
 */
function invertRequest(r) {
  const t = invertOffset(r);
  t.ax = r.ax === null ? null : -r.ax / r.k;
  t.ay = r.ay === null ? null : -r.ay / r.k;
  return t;
}

/** `+12` / `-4` / `0` -- the sign is information, and a bare `12` hides it. */
function signed(n) {
  const r = Math.round(n);
  return r > 0 ? `+${r}` : `${r}`;
}

export function imageCompare({
  leftUrl, rightUrl, leftLabel, rightLabel, leftSize, rightSize,
  leftContentType, rightContentType,
}) {
  // The shared box as the server's stored dimensions draw it. The order-
  // independent frame: the load window's transient box is `max` of one real
  // measurement and one stored placeholder, so it differs per decode order
  // exactly when the corrected widths differ, while this does not.
  const storedBox = (a, b) => (a && b && a.w && a.h && b.w && b.h)
    ? Math.max(a.w, b.w)
    : null;
  const storedBoxW = storedBox(leftSize, rightSize);
  return {
    mode: 'side-by-side',
    // How the two images are measured into the shared box. `relative` is true
    // relative scale and was the only policy the page ever had; it is still the
    // default, because a pair with identical dimensions renders the same under
    // all three and the divergent minority has no measurement saying which of a
    // rescan and a crop is more common.
    scale: 'relative',
    // Where an image sits in whatever slack the box leaves it. Centring is right
    // for a photograph and wrong for a document or a screenshot, where content
    // is flush to a corner and centring throws the page out by half the size
    // difference.
    anchor: 'center',
    // `swapped` is the only piece of swap state. Exchanging the URLs and labels
    // in place left the panel colours and the server-rendered alt text describing
    // whichever side had originally been there, so the red "older" panel could
    // hold the newer file and a screen reader was told the opposite of the truth.
    // Everything that varies is derived from this flag instead.
    swapped: false,
    // Manual alignment. Package 1's three scale policies are whole-pair
    // decisions computed from intrinsic dimensions; none of them can act on a
    // pair that is the right size and simply not in register -- a scan placed
    // differently on the glass, a re-photograph at 103%. This is the correction
    // the reader drives, and it is armed rather than always-live because the
    // arrow keys are already spent on the slider and the onion opacity.
    aligning: false,
    // `dx`/`dy` in **box pixels** -- the intrinsic units of the shared
    // max(w1,w2) x max(h1,h2) box, so a window resize changes nothing -- and
    // `k` a scale factor. Indexed like `_sizes` and `_urls`: this describes
    // slot 1 relative to slot 0, never "the trail", because Flip exchanges
    // which version trails and an offset keyed on the side would transpose on
    // every press. The trail's transform is derived, so a flip mutates nothing.
    _offset: { dx: 0, dy: 0, k: 1 },
    // Written on each keyboard nudge and once at the end of a drag, never
    // during one: a live region updated per `pointermove` produces a queue a
    // screen reader spends minutes reading.
    offsetAnnouncement: '',
    _announceParity: false,
    _alignDragEndedAt: 0,
    _announceTimer: null,
    // Which slots a loaded image has reported a size for, which is not the same
    // question as whether `_sizes` changed: a load that confirms the stored
    // value writes nothing and still means that version has been measured.
    _measured: [false, false],
    /**
     * The per-axis bound test as of the reader's last act, canonically -- what
     * a size report's claim is measured against.
     *
     * Deliberately not re-read per report. A report can move a bound so that
     * an axis the reader legitimately left outside is momentarily inside
     * again, and the next report then finds it crossing outward and claims it,
     * clamping a position no report ever broke. Which report does that is the
     * decode order: the same two files and the same presses keep the
     * correction in one order and snap it in the other.
     *
     * So a claim measures what the reports did to the position the reader
     * chose, cumulatively, rather than what the last one did to whatever the
     * one before it left. It is taken at the first report of a provisional
     * pair and held until the pay, which is the repair those claims asked
     * for; the reader restates it only once the pair is whole
     * (`_restateReportBaseline`), and Reset retires it with everything else.
     */
    _reportBefore: null,
    // The half-measured nudge records here the translation the reader asked
    // for -- including everything a display-side clamp deferred -- in the same
    // slot-relative shape `_offset` uses, plus a per-axis `ax`/`ay` anchor
    // (below). The translation is denominated in the request's own canonical
    // pixels rather than the transient box's: a step taken against the
    // transient box inherits whichever geometry happened to be standing, so
    // the same two files and the same key would resolve differently per decode
    // order. Box-width reports therefore leave it alone; the deferred rebound
    // converts it into the final box's pixels when it resolves. Null once
    // resolved or discarded; it never drives a render.
    _requestedOffset: null,
    /**
     * Which axes the reader actually asked to move inside the window.
     *
     * The request carries both axes because it carries a position, but only a
     * moved axis is a *request*: the other one is the correction that was
     * already standing, and it must resolve the way it would have with no
     * request at all -- kept where it is unless a bound-mover owes it. Clamped
     * instead, one ArrowDown erases an x correction made before the window,
     * which is one axis resolving another.
     *
     * Flip-invariant, like the claim flags: `invertOffset` maps dx to dx.
     */
    _requestMovedX: false,
    _requestMovedY: false,
    // The stored box's width, in its own pixels -- the frame both decode
    // orders share before either image has spoken. Null when the server had no
    // dimensions for the pair: then the box itself does not exist until one
    // version has reported, only that version's report can open a nudgeable
    // window, and there is no second order to diverge from.
    _storedBoxW: storedBoxW,
    // The width the outstanding request is denominated in, captured once when
    // that request is created: the stored box's, or -- when the server had no
    // dimensions -- the box standing at creation. A missing stored box is not
    // a missing conversion. Falling the whole thing back to a factor of 1
    // means "no conversion at all" rather than "no stored frame": the
    // displayed offset scales with the corrected box and the request does
    // not, so the reader watches their correction shrink when the second
    // version arrives. Null while no request is outstanding.
    _requestUnitsW: null,
    _endAlignDrag: null,
    // Removes an active align drag's *move* listeners only, leaving the
    // enders: `toggleAligning` calls it on a mid-gesture disarm.
    _disarmAlignDrag: null,
    sliderPos: 50,
    opacity: 50,
    showLeft: true,
    isDragging: false,
    // The pixel-diff heatmap. Armed rather than automatic: the mask is noise
    // on an unregistered pair, and the reader decides when to look at it.
    // `heatMapPercent` is null whenever nothing has been computed -- unarmed,
    // half-measured, or a pair that does not overlap.
    heatMapOn: false,
    heatMapNeedsConfirm: false,
    heatMapConfirmed: false,
    heatMapPercent: null,
    heatMapOverlapEmpty: false,
    heatMapAnnouncement: '',
    _heatMapFrame: null,
    _heatMapAnnounceAfterRepaint: false,
    _heatMapAnnounceParity: false,
    // The compose canvases and the sample resolution they were built for.
    _heatMapScratch: null,
    // The blink comparator. Starts stopped, always: the reader asked for the
    // page, not for an animation. `blinkRate` is flips per second, bounded by
    // BLINK_RATE_MIN/MAX; `_blinkTimer` holds the interval handle.
    blinking: false,
    blinkRate: BLINK_RATE_DEFAULT,
    _blinkTimer: null,
    _blinkAppliedRate: BLINK_RATE_DEFAULT,
    // Resolved lazily from `prefers-reduced-motion` and updated by its change
    // listener, so a reader who toggles the OS preference mid-play is answered
    // too. Null until first asked; the unit suite (no DOM) never asks.
    _reducedMotion: false,
    _reducedMotionQuery: null,
    _reducedMotionListener: null,
    blinkAnnouncement: '',
    _blinkAnnounceParity: false,
    _urls: [leftUrl, rightUrl],
    _labels: [leftLabel || '', rightLabel || ''],
    _contentTypes: [leftContentType || '', rightContentType || ''],
    // Intrinsic sizes, so the overlay modes can place both images in one
    // coordinate space. Missing values fall back to the container, which is the
    // old behaviour and the best guess available.
    _sizes: [leftSize || null, rightSize || null],
    _keyHandler: null,
    _endDrag: null,

    /** The image drawn on the leading (old-coloured) side, after any swap. */
    get leadUrl() {
      return this._urls[this.swapped ? 1 : 0];
    },
    get trailUrl() {
      return this._urls[this.swapped ? 0 : 1];
    },
    get leadLabel() {
      return this._labels[this.swapped ? 1 : 0];
    },
    get trailLabel() {
      return this._labels[this.swapped ? 0 : 1];
    },
    get leadAlt() {
      return this.leadLabel;
    },
    get trailAlt() {
      return this.trailLabel;
    },

    /**
     * Aspect ratio for the shared overlay box, as a CSS `aspect-ratio` value.
     *
     * Slider and onion skin lay both images over one another. Sizing each to the
     * container width and letting its own ratio decide its height means two
     * differently-shaped versions occupy different boxes, so the overlay covers
     * part of the frame and aligns with none of it. One box built from the widest
     * and tallest of the pair, with both images contained inside it, keeps the two
     * in register and preserves their relative scale.
     */
    get overlayRatio() {
      const box = this.overlayBox;
      return box ? `${box.w} / ${box.h}` : null;
    },

    /** The shared box in intrinsic pixels, or null when there is nothing to measure into. */
    get overlayBox() {
      const [a, b] = this._sizes;
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) return null;
      return { w: Math.max(a.w, b.w), h: Math.max(a.h, b.h) };
    },

    /**
     * One version's untransformed size inside that box, in box pixels.
     *
     * The arithmetic `overlayScale` turns into a percentage, named separately
     * because the alignment bound needs the same number: how far a version may
     * travel before it leaves the frame is a fact about that version's rendered
     * size, and a second copy of this would drift from the sizing it describes.
     */
    elementSize(index) {
      const box = this.overlayBox;
      if (!box) return null;
      // Under Stretch the element *is* the box.
      if (this.scale === 'stretch') return { w: box.w, h: box.h };
      const img = this._sizes[index === 0 ? 0 : 1];
      if (this.scale === 'fit') {
        // Grow the image until an edge touches the box, so two versions of one
        // aspect ratio both fill a box built from the larger and a pure
        // resolution change registers exactly.
        const k = Math.min(box.w / img.w, box.h / img.h);
        return { w: img.w * k, h: img.h * k };
      }
      return { w: img.w, h: img.h };
    },

    /**
     * Per-image size inside the shared overlay box.
     *
     * An **object**, not a CSS string, and that is load-bearing rather than a
     * matter of taste. Alpine's `x-bind:style` replaces the entire `style`
     * attribute when given a string, while `x-show` works by writing
     * `display: none` onto that same attribute. Every element here carries both
     * -- the toggle-mode images are `x-show`n and styled, and so are the three
     * overlay boxes. So the moment this value became reactive, a re-render blew
     * away `x-show`'s work and every mode's box painted at once. The object form
     * sets only the properties it names and leaves `display` alone.
     *
     * The key set is therefore fixed: a key dropped between two renders keeps
     * whatever it was last set to, so a value that no longer applies is emitted
     * as an empty string rather than omitted.
     */
    overlayScale(index) {
      const box = this.overlayBox;
      // Nothing to measure into. The `[style*="aspect-ratio"]` branch in
      // index.css owns the layout here and puts the images back in the flow, so
      // an inline `height: 100%` from the other branches below would beat its
      // `height: auto` and break a case that currently works.
      if (!box) {
        return { width: '', height: '', objectFit: '', margin: '', transform: '', transformOrigin: '' };
      }

      // Stretch distorts each image onto the whole box. Right for a re-encode
      // that changed aspect, wrong for a crop, which is why this mode's
      // accessible name says so rather than leaving the reader to notice.
      if (this.scale === 'stretch') {
        // The element *is* the box here, so that is what a box-pixel offset is
        // a percentage of. Alignment is not refused under Stretch the way
        // anchoring is: Anchor refuses because it provably cannot act -- there
        // is no slack to take up -- while a translate acts under every policy.
        return {
          width: '100%', height: '100%', objectFit: 'fill', margin: '',
          ...this.alignStyle(index, box.w, box.h),
        };
      }
      // Sized on the *element* rather than left to `object-fit: contain` on a
      // full-box element, which would paint identically. Two reasons, and
      // neither is arithmetic for its own sake: a contained letterbox is
      // invisible to `getBoundingClientRect`, so Fit and Stretch would render
      // to indistinguishable geometry and nothing outside a screenshot could
      // tell them apart; and with the element equal to the painted rectangle,
      // anchoring is `margin` here exactly as it is under relative scale,
      // instead of `margin` in one mode and `object-position` in the other.
      const { w, h } = this.elementSize(index);

      return {
        width: `${(w / box.w) * 100}%`,
        height: `${(h / box.h) * 100}%`,
        objectFit: '',
        // `margin: auto` on the class centres an absolutely-positioned element
        // pinned on all four sides. Zeroing it over-constrains the box, which
        // CSS resolves in a left-to-right document by ignoring `right` and
        // `bottom` -- the top-left corner.
        margin: this.anchor === 'top-left' ? '0' : '',
        ...this.alignStyle(index, w, h),
      };
    },

    /**
     * The reader's own correction, as a transform on the trailing image.
     *
     * Sized against the **element**, not the box: a CSS percentage translate
     * resolves against the element's own border box, and `w`/`h` here are that
     * element's size in box pixels. So twelve box pixels is 3% of a 400-wide
     * element and 1.5% of an 800-wide one, which is what makes the stored value
     * mean the same thing under every scale policy.
     *
     * Both keys are returned on every path, empty where they do not apply --
     * `overlayScale`'s fixed-key-set contract, which a spread into its return
     * would otherwise be the easiest place in the file to break.
     */
    alignStyle(index, elementW, elementH) {
      const none = { transform: '', transformOrigin: '' };
      // The lead is the reference and is never moved; only the trail carries
      // the correction, whichever slot Flip has put there.
      if (index !== (this.swapped ? 0 : 1)) return none;
      const { dx, dy, k } = this.trailOffset;
      if (dx === 0 && dy === 0 && k === 1) return none;
      const tx = elementW ? (dx / elementW) * 100 : 0;
      const ty = elementH ? (dy / elementH) * 100 : 0;
      return {
        transform: `translate(${tx}%, ${ty}%) scale(${k})`,
        // The anchor is the reader's assertion that the two line up at the
        // corner. Scaling from the centre after that walks the corner off by
        // half the scale change and undoes what they just said.
        transformOrigin: this.anchor === 'top-left' ? '0 0' : '',
      };
    },

    /**
     * The correction as it applies to whichever image is currently trailing.
     *
     * `_offset` is slot 1 relative to slot 0. After a flip the trailing image
     * is slot 0, and preserving the alignment means applying the inverse to it
     * rather than resetting -- a flip is how you check the correction took, so
     * throwing it away is exactly the wrong moment. Derived rather than stored,
     * so a flip mutates nothing and flipping twice is bit-for-bit a no-op.
     */
    get trailOffset() {
      return this.swapped ? invertOffset(this._offset) : this._offset;
    },

    _setTrailOffset(t) {
      this._restateReportBaseline();
      // `invertOffset` is an involution, so writing back through it is the same
      // operation as reading through it.
      this._offset = this.swapped ? invertOffset(t) : t;
      // Every correction moves what the mask should show. The heatmap's
      // watchers also fire on this write under Alpine; this explicit request
      // is what covers harnesses without reactivity, and both coalesce.
      this._scheduleHeatMapRepaint();
    },

    /**
     * The trailing-side view of the reader's translation request.
     *
     * Before a half-measured nudge has created a divergence this *is* the
     * displayed offset; afterwards the view derives through the same inversion
     * a flip applies to `_offset`, so a flip mutates the request no more than
     * it mutates the display.
     */
    _rawRequest() {
      if (this._requestedOffset === null) {
        const t = this.trailOffset;
        return { dx: t.dx, dy: t.dy, k: t.k, ax: null, ay: null };
      }
      return this.swapped ? invertRequest(this._requestedOffset) : this._requestedOffset;
    },

    _setRawRequest(t) {
      this._requestedOffset = this.swapped
        ? invertRequest(t)
        : { dx: t.dx, dy: t.dy, k: t.k, ax: t.ax, ay: t.ay };
    },

    /**
     * Whether manual alignment can act.
     *
     * The same predicate the scale group and the anchor use, not a second copy
     * of it. With no usable dimensions the `index.css` fallback puts the lead
     * back in the flow and pins the trail to the top: the two are not in a
     * shared coordinate space at all, and the formats that actually reach that
     * branch -- HEIC, TIFF -- paint nothing to align.
     */
    get alignAvailable() {
      return this.scaleAvailable;
    },

    get offsetIsIdentity() {
      const { dx, dy, k } = this._offset;
      return dx === 0 && dy === 0 && k === 1;
    },

    /**
     * Whether the correction displaces the version at all.
     *
     * What decides whether an operation that moves the bound has anything to
     * repair. Zero displacement is inside every range `translateRange` can
     * produce: `min` is `-(rest + rendered - keep)`, which is never above zero,
     * and `max` is `box - keep - rest`, which is never below it, at any anchor,
     * scale or zoom (the rendered length is at most four times the box, and
     * `keep` is a quarter of it).
     *
     * Distinct from `offsetIsIdentity`, which is also false at a bare zoom --
     * and a bare zoom displaces nothing either.
     */
    get offsetIsPlaced() {
      return this._offset.dx !== 0 || this._offset.dy !== 0;
    },

    /**
     * Whether Reset has nothing to do.
     *
     * The readout shows only what is displayed, so `offsetIsIdentity` alone
     * under-reports here: the half-measured window's clamps can leave an
     * unshown remainder under an identity readout, and pressing Reset would
     * clear it. A control drawn `aria-disabled` must mean what it says.
     */
    get resetIdle() {
      return this.offsetIsIdentity && this._requestedOffset === null;
    },

    /** `+12, -4, 103%`, reporting the clamped values actually in effect. */
    get offsetLabel() {
      const { dx, dy, k } = this.trailOffset;
      return `${signed(dx)}, ${signed(dy)}, ${Math.round(k * 100)}%`;
    },

    toggleAligning() {
      if (!this.alignAvailable) return;
      this.aligning = !this.aligning;
      // Disarming mid-gesture ends the drag's movement: the move listeners
      // are removed, so nothing nudges while disarmed and a re-arm before the
      // release cannot resurrect the gesture. The enders stay -- the release
      // still fires the button's click, and the suppression has to recognise
      // that click as the drag's.
      if (!this.aligning && this._disarmAlignDrag) this._disarmAlignDrag();
    },

    /**
     * Move the trailing image by a whole number of box pixels.
     *
     * Does not announce: the drag calls this many times a second and the
     * announcement is made once, at the end. Keyboard callers announce for
     * themselves.
     *
     * `physical` says the increment is a distance on the *screen*, already
     * converted into the pixels of whichever box is standing -- which is what
     * a drag hands in, and what no keyboard caller does. It changes nothing
     * about the display and only how the load window records the increment;
     * see there.
     */
    nudge(dx, dy, physical = false) {
      if (!this.alignAvailable) return;
      const box = this.overlayBox;
      // The bound is a property of the version being moved, so it is measured
      // from that version's own element -- the trailing slot, whichever Flip
      // has put there.
      const element = this.elementSize(this.swapped ? 0 : 1);
      if (!box || !element) return;
      const t = this.trailOffset;
      const x = this.translateRange(box.w, element.w * t.k);
      const y = this.translateRange(box.h, element.h * t.k);
      const next = {
        dx: boundedNudge(t.dx, dx, x),
        dy: boundedNudge(t.dy, dy, y),
        k: t.k,
      };
      // While exactly one version is measured the bound is transient -- the
      // box is `max` of a real measurement and a stored placeholder, and the
      // trail is whichever slot decoded first. The *display* still obeys it:
      // a nudge must leave the pair watchable even if the second image never
      // decodes. What is deferred is the resolution. Clamping the increment
      // against the transient bound and keeping nothing more would bind the
      // result to whichever bound happened to be standing mid-decode, and the
      // later width conversion then scales that already-clamped value, so the
      // same two files and input land on different canonical offsets
      // depending on decode order. Instead every increment rides onto
      // `_requestedOffset`, and the deferred rebound resolves that request
      // against the final bound once both versions have reported.
      if (this._measured[0] !== this._measured[1]) {
        const fresh = this._requestedOffset === null;
        const request = this._rawRequest();
        // The canonical width is captured once, with the request: the stored
        // box's when the server had dimensions for the pair, and otherwise the
        // box standing right now, which is the only frame there is.
        if (fresh) this._requestUnitsW = this._storedBoxW || box.w;
        // Out of the box standing now and into that frame. A request born here
        // takes the displayed position over as its base and converts it; an
        // increment converts only when it is `physical`.
        const intoUnits = this._requestUnitsW / box.w;
        // A keyboard step is an abstract unit, and the window redefines it as
        // canonical pixels: recording it raw is what makes the same key mean
        // the same correction whichever version decoded first. Converting it
        // would telescope straight back to order-dependence. A drag increment
        // is the opposite -- a physical distance the hand covered, already
        // expressed in the standing box's pixels -- so recording *that* raw
        // changes what it measures, and the resolution then scales a distance
        // that was never in the canonical frame. It is converted, and the
        // resolution's conversion telescopes it back to the screen distance
        // the reader drew.
        // Either way it is the whole increment the reader asked for. One that
        // met the clamp contributes the part still owed on top of the travel
        // the display made, and one that moved freely travelled the whole of
        // it -- so walking the image back retires exactly what running it out
        // incurred, without the two cases needing to be told apart.
        const rode = physical ? intoUnits : 1;
        // Where the reader stood when the window's first nudge landed, in the
        // request's own canonical pixels and its slot-relative shape, so a
        // flip carries it. The resolution walks from here by everything the
        // window accumulated, against the final bound widened to include it:
        // the whole interaction replayed as one nudge from where the reader
        // started.
        //
        // Recorded once and never re-sampled. Every other moment to read it
        // at is contaminated: the transient bound belongs to whichever version
        // decoded, so "is the display outside it" and "did the display move"
        // both answer differently per decode order, and the value at
        // resolution cannot tell a flip-derived outside from one the
        // completing report has just created. This one is order-independent by
        // construction -- the reader's script before the window is the same in
        // either order, and denominating in `_requestUnitsW` cancels exactly
        // the box conversion a report in between applies.
        //
        // The resolution drops it for an axis the reports pushed out of a
        // bound it was inside: that outside-ness is theirs to repair, not a
        // position to preserve.
        const anchor = (base, kept) => (fresh ? base * intoUnits : kept);
        this._setRawRequest({
          dx: request.dx * (fresh ? intoUnits : 1) + dx * rode,
          dy: request.dy * (fresh ? intoUnits : 1) + dy * rode,
          k: request.k,
          ax: anchor(request.dx, request.ax),
          ay: anchor(request.dy, request.ay),
        });
        if (dx !== 0) this._requestMovedX = true;
        if (dy !== 0) this._requestMovedY = true;
      }
      this._setTrailOffset(next);
    },

    /**
     * How far the trailing version may travel along one axis.
     *
     * Expressed so that `ALIGN_KEEP_VISIBLE` of the version is still inside the
     * frame at either end. A fraction of the *box* would not do that: a small
     * version anchored to the corner of a large box clears the frame entirely
     * well before it has travelled half a box, which is the case a half-box
     * bound silently fails.
     *
     * `rest` is where the rendered rectangle's leading edge sits before any
     * offset. Centred, the scale happens about the element's own centre, so
     * that centre stays at the box's; anchored, the element's leading edge is
     * the box's and `transform-origin: 0 0` keeps it there.
     */
    translateRange(boxLength, renderedLength) {
      const rest = this.anchor === 'top-left' ? 0 : (boxLength - renderedLength) / 2;
      const keep = renderedLength * ALIGN_KEEP_VISIBLE;
      return {
        min: -(rest + renderedLength - keep),
        max: boxLength - keep - rest,
      };
    },

    zoomBy(delta) {
      if (!this.alignAvailable) return;
      const within = this.offsetAxesWithinBound;
      const t = this.trailOffset;
      const k = Math.max(ALIGN_ZOOM_MIN, Math.min(ALIGN_ZOOM_MAX, t.k + delta));
      // Pressing + at the 400% bound changes no geometry -- the clamp holds
      // k -- so nothing is owed, exactly as a no-op scale activation arms
      // nothing.
      if (k === t.k) return;
      this._setTrailOffset({ dx: t.dx, dy: t.dy, k });
      // Zooming leaves the translation where it was; an outstanding raw
      // request keeps its translation too, with only its zoom factor following
      // the display's, so the deferred resolution compares the request against
      // the bound the reader is actually looking at.
      if (this._requestedOffset !== null) {
        const r = this._rawRequest();
        this._setRawRequest({ dx: r.dx, dy: r.dy, k, ax: r.ax, ay: r.ay });
      }
      this._reboundOrDefer(within);
    },

    /**
     * Answer `fn` as though no flip were in effect.
     *
     * `_offset` is slot 1 relative to slot 0 and a flip does not touch it, so
     * the canonical frame is the one frame the reader's flips cannot move.
     * Everything the load window **defers** is decided there, for the
     * reader's own operations as much as for the reports: a report arrives in
     * whatever order the two decodes finish, and an operation's crossing has
     * to survive until the both-measured moment answers it, by which time the
     * view may be the other one. Asked of the displayed orientation, either
     * lets the decode order pick which version's bound the answer is about.
     *
     * The *immediate* repair is not deferred and stays in the displayed
     * frame: the reader is looking at that view and is owed the guarantee
     * there.
     */
    /**
     * The reader has stated a position, so a later report's claim is measured
     * against this one -- but only while the pair is theirs to state it in.
     *
     * With a version still missing, the geometry they are acting in is
     * whichever one decoded rather than the pair's, so a baseline re-dated
     * there carries the decode order into every claim that follows: the same
     * two files and the same presses leave one order clamping a correction and
     * the other keeping it. Their intent inside that window rides on the
     * request and its anchors instead, denominated in canonical pixels and
     * resolved against the final bound. See `_reportBefore`.
     */
    _restateReportBaseline() {
      if (this._measured[0] && this._measured[1]) this._reportBefore = null;
    },

    _canonically(fn) {
      const held = this.swapped;
      this.swapped = false;
      try {
        return fn();
      } finally {
        this.swapped = held;
      }
    },

    _reboundOrDefer(before) {
      this._restateReportBaseline();
      // A bound moved underneath a version that is not displaced broke
      // nothing: zero is inside every range `translateRange` can produce, so
      // the per-axis test below would refuse this too. It is here as the fast
      // path -- the wheel calls this once per notch, and the test below is a
      // full pass of the range arithmetic -- and as the statement of why an
      // undisplaced correction can never owe anything.
      if (!this.offsetIsPlaced) return;
      // Asked per axis, because the answer differs per axis: a flip can leave
      // x legitimately outside while y sits inside, and an anchor change then
      // makes x legal and throws y clear of the frame. Asked of the offset as
      // a whole -- "was it inside before?" -- x answers for y, the repair never
      // runs, and the reader is left looking at nothing at all, which is the
      // state the bound exists to prevent.
      //
      // Only with the pair whole. While a version is missing this operation
      // has no bound worth asking about -- the one standing belongs to
      // whichever version decoded -- so it records nothing at all and the pay
      // works out what is owed when the pair is complete.
      if (this._measured[0] !== this._measured[1]) return;
      this._reboundOffset(undefined, before);
    },

    /**
     * Whether the offset currently sits inside the bound its geometry implies.
     *
     * Snapshotted *before* an operation that moves the bound, because a rebound
     * corrects what that operation broke and never what was already true. The
     * distinction is what protects a flip: the inverse of an extreme correction
     * is legitimately outside this side's bound, and without this, re-selecting
     * the scale that is already selected quietly rewrites the reader's original
     * number. Nothing else can leave it outside, so refusing to "repair" one is
     * refusing to repair a state no path produces.
     */
    get offsetWithinBound() {
      const axes = this.offsetAxesWithinBound;
      return axes.x && axes.y;
    },

    /**
     * The same question, per axis.
     *
     * Whether an operation broke anything is asked of one axis at a time --
     * a report that corrects only the height moves the y bound, and the offset
     * converts through the *width* ratio alone, so it leaves the x bound and
     * the x value exactly where they were. `offsetWithinBound` is this
     * conjoined rather than a second copy of the range arithmetic.
     */
    get offsetAxesWithinBound() {
      const box = this.overlayBox;
      const element = this.elementSize(this.swapped ? 0 : 1);
      if (!box || !element) return { x: true, y: true };
      const t = this.trailOffset;
      const x = this.translateRange(box.w, element.w * t.k);
      const y = this.translateRange(box.h, element.h * t.k);
      return { x: withinRange(t.dx, x), y: withinRange(t.dy, y) };
    },

    /**
     * Bring a translation back inside the bound the current geometry implies.
     *
     * Without arguments this is the load-time rebound proper: it repairs the
     * displayed offset after zooming out, changing scale policy or anchoring
     * moved the bound while leaving the offset where it was. Without this, a
     * correction that was legal at 100% puts the version entirely outside the
     * frame at 25%: in an 800-wide box, `dx = 600` renders at x = 900..1100.
     *
     * With a request it is the deferred form instead: the request -- what the
     * half-measured window's clamps swallowed, denominated in the canonical
     * pixels `_requestUnitsW` names -- is converted into the final box's pixels
     * and resolved against the bound now standing. Each axis clamps to that
     * bound, widened to include the axis's anchor if the window recorded one:
     * an axis the reader never moved has the display's own value as its
     * request, and the display's value can sit legitimately outside -- a
     * flip-derived inverse, which `boundedNudge` deliberately widened the
     * display's range around. Clamping that axis outright would snap the far
     * side of the correction by the whole difference because one ArrowDown
     * happened to touch the other axis.
     *
     * The anchor rather than wherever the display sits *now*, because by now
     * the completing report has run: it can have shrunk or moved the trailing
     * version and pushed the display outside on its own account, and that
     * outside-ness is the debt this rebound exists to pay, not a position to
     * preserve. Widening around it left one decode order entirely out of frame
     * and the other short of the quarter the bound promises.
     *
     * A flip deliberately does **not** call this. Every one of the three above
     * is the reader changing something and being answered; a flip's whole
     * purpose is to show the same alignment the other way round, so altering it
     * there is the one thing that would make it useless. It costs exactness in
     * one direction -- a zoom while flipped rewrites the stored offset, so
     * flipping back does not restore the original number -- which is the same
     * price zooming already charges unflipped.
     */
    _reboundOffset(request, before) {
      if (!this.alignAvailable) return;
      // At identity nothing the displayed-offset form repairs was broken; the
      // deferred form carries its own value and must resolve even so.
      if (request === undefined && this.offsetIsIdentity) return;
      const box = this.overlayBox;
      const element = this.elementSize(this.swapped ? 0 : 1);
      if (!box || !element) return;
      // The request's translation is in the canonical pixels it was created
      // in; the bound is a fact about the box standing now, so the two must
      // meet in one frame.
      const outOfUnits = request !== undefined && this._requestUnitsW
        ? box.w / this._requestUnitsW
        : 1;
      const t = request === undefined
        ? this.trailOffset
        : { dx: request.dx * outOfUnits, dy: request.dy * outOfUnits, k: request.k };
      const x = this.translateRange(box.w, element.w * t.k);
      const y = this.translateRange(box.h, element.h * t.k);
      const shown = this.trailOffset;
      // What is owed, decided here and only here, and by one rule for both
      // forms: an axis inside its bound at the reference moment and outside it
      // now was put there by what happened in between, and only that is owed.
      // One already outside then is the reader's -- a flip derives it -- and
      // pulling it in would rewrite a correction they made.
      //
      // The reference moment is what differs. For the displayed form it is
      // just before the operation that called; for the deferred form it is the
      // window's opening, taken ahead of the first report's own effect. It is
      // deliberately not any moment *inside* the window: every one of those is
      // a geometry belonging to whichever version decoded, so a claim recorded
      // there would carry the decode order with it.
      const owedX = before.x && !withinRange(t.dx, x);
      const owedY = before.y && !withinRange(t.dy, y);
      let dx, dy;
      if (request === undefined) {
        // Only where the outside-ness is owed. An axis still inside clamps to
        // itself, and one that was outside before the operation ran is a
        // flip's doing, not this operation's -- pulling it in is the rewrite
        // of the reader's own correction that `offsetWithinBound` has always
        // existed to refuse, asked at the grain the question has.
        dx = owedX ? Math.max(x.min, Math.min(x.max, t.dx)) : t.dx;
        dy = owedY ? Math.max(y.min, Math.min(y.max, t.dy)) : t.dy;
      } else {
        // The request's *unclamped* value meets the range widened by the
        // anchor: from an anchored position a swallowed inward increment stops
        // at the near edge exactly as `nudge` itself would, an outward one
        // stops at the anchor, and an untouched axis keeps it. Clamping first
        // and widening after would instead read the clamp's own pull-in as an
        // inward gesture and snap a legitimately-outside axis onto the
        // boundary. With no anchor there is nothing legitimate outside the
        // bound to protect, and the value simply clamps.
        const axis = (value, held, range) => (held === null
          ? Math.max(range.min, Math.min(range.max, value))
          : boundedNudge(held, value - held, range));
        // An anchor means the axis was legitimately outside at nudge time
        // *and* nothing has claimed it since. A report that pushed it out of a
        // bound it was inside is such a claim, and widening around the older
        // anchor would launder the outside-ness that report created.
        const held = (anchor, owed) => (owed || anchor === null ? null : anchor * outOfUnits);

        // An axis the reader never moved in the window is not a request at
        // all: it is the correction that was already standing, and it resolves
        // exactly as it would have with no request -- kept unless a
        // bound-mover owes it. Clamping it because the *other* axis was nudged
        // is one axis resolving another.
        const still = (value, owed, range) => (owed
          ? Math.max(range.min, Math.min(range.max, value))
          : value);
        dx = this._requestMovedX
          ? axis(t.dx, held(request.ax, owedX), x)
          : still(shown.dx, owedX, x);
        dy = this._requestMovedY
          ? axis(t.dy, held(request.ay, owedY), y)
          : still(shown.dy, owedY, y);
      }
      // Nothing to bring back: either the clamp held the value or the screen
      // already shows exactly what the request resolves to. Writing anyway
      // would re-derive `_offset` through the inverse and back on every wheel
      // notch while flipped, which is work for no change and the only place
      // this component could accumulate floating-point drift.
      if (dx === shown.dx && dy === shown.dy) return;
      this._setTrailOffset({ dx, dy, k: t.k });
    },

    /**
     * Pay the deferred rebound: resolve any outstanding translation request
     * first -- that is what a half-measured nudge's swallowed increments ride
     * on -- then fall back to repairing the displayed offset itself. The
     * resolved request becomes the position shown, so what arrives is where
     * the reader asked to be, bounded by the geometry both versions imply.
     */
    _resolveDeferred(before) {
      const request = this._requestedOffset === null ? undefined : this._rawRequest();
      this._reboundOffset(request, before);
      this._requestedOffset = null;
      this._requestUnitsW = null;
      this._requestMovedX = false;
      this._requestMovedY = false;
    },

    /**
     * Clear the translation and the zoom together.
     *
     * Both halves, because a reset that left a 103% zoom behind would be the
     * same invisible state in a smaller box. The arming survives: the reason
     * you reset is almost always that you are about to try again.
     */
    resetAlignment() {
      // Retired by any Reset press, not only ones that visibly move something:
      // an armed debt or an outstanding window remainder dies with the
      // correction it belongs to -- left alone, a later confirming report
      // would rewrite alignment the reader makes after the reset.
      this._reportBefore = null;
      // A refused Reset stays silent: the control is drawn `aria-disabled`
      // while idle rather than removed, so it stays reachable and pressable,
      // and announcing "offset 0, 0, 100%" at someone who changed nothing is
      // the live-region equivalent of a control that looks like it acted.
      // Idle means no displayed correction AND no outstanding request -- a
      // readout sitting at identity can still have one underneath it, and
      // clearing it is something done, not a refusal.
      if (this.resetIdle) return;
      this._requestedOffset = null;
      this._requestUnitsW = null;
      this._requestMovedX = false;
      this._requestMovedY = false;
      this._offset = { dx: 0, dy: 0, k: 1 };
      this.announceOffset();
    },

    announceOffset() {
      // A wheel burst may have one of these pending. Letting it fire after a
      // Reset or a keyboard nudge announces a value the reader has moved past.
      if (this._announceTimer) {
        clearTimeout(this._announceTimer);
        this._announceTimer = null;
      }
      this._announceParity = !this._announceParity;
      this.offsetAnnouncement = `Offset ${this.offsetLabel}${this._announceParity ? ANNOUNCE_MARK : ''}`;
      // The correction has already requested a silent repaint. Upgrade that
      // pending frame to announce after it computes, so a drag stays quiet per
      // move and its one end event speaks the final percentage, not the stale
      // one from before the frame.
      this._scheduleHeatMapRepaint(true);
    },

    /**
     * The alignment's keys, from either of the two places they can arrive.
     *
     * `_keyHandler` deliberately ignores events targeted at anything focusable,
     * on the grounds that a focusable element answers its own arrow keys -- and
     * the Align toggle is a button, so arming it by keyboard leaves focus on the
     * one element that would swallow every press. Giving that button its own
     * handler satisfies the rule rather than carving an exception out of it, and
     * both entry points land here. The interlock against a double step is
     * `_keyHandler`'s existing `defaultPrevented` guard.
     *
     * Returns whether the key was the alignment's, so a caller that shares the
     * event with other handlers can tell.
     */
    handleAlignKey(e) {
      if (!this.aligning || !this.alignAvailable) return false;
      // Modified keys are the browser's: Ctrl+R reloads, Ctrl+/- zooms the
      // page, Alt+ArrowLeft goes back. The alignment's shortcuts are the
      // plain keys (Shift steps them up), so a modified press is not ours --
      // and refusing here covers the Align button's own handler, which calls
      // this directly without the container's guard.
      if (e.ctrlKey || e.metaKey || e.altKey) return false;
      const step = e.shiftKey ? ALIGN_STEP_LARGE : ALIGN_STEP;
      // `e.shiftKey` rather than the shifted character, so the step means the
      // same on a layout where `+` is not Shift and `=`.
      const zoom = e.shiftKey ? ALIGN_ZOOM_STEP_LARGE : ALIGN_ZOOM_STEP;
      switch (e.key) {
        case 'ArrowLeft': this.nudge(-step, 0); break;
        case 'ArrowRight': this.nudge(step, 0); break;
        case 'ArrowUp': this.nudge(0, -step); break;
        case 'ArrowDown': this.nudge(0, step); break;
        case '+': case '=': this.zoomBy(zoom); break;
        case '-': case '_': this.zoomBy(-zoom); break;
        // `resetAlignment` announces for itself, so this leaves the switch
        // rather than falling through to a second announcement.
        case 'r': case 'R':
          this.resetAlignment();
          if (typeof e.preventDefault === 'function') e.preventDefault();
          return true;
        default: return false;
      }
      if (typeof e.preventDefault === 'function') e.preventDefault();
      this.announceOffset();
      return true;
    },

    /**
     * Drag the trailing image while armed.
     *
     * The surface is the overlay box itself: in slider mode the trail `<img>`
     * is `pointer-events-none` and the lead sits inside a `pointer-events-none`
     * clip wrapper, with the slider handle above at `z-10` -- so the handle
     * keeps its own drag and everything else in the box is alignment's.
     *
     * Rendered pixels are converted into box pixels through the box's measured
     * width, which is what keeps a drag and an arrow press speaking the same
     * units. Teardown follows `startSliderDrag`: a mouse released outside the
     * window delivers no `mouseup` here, and the OS takes a touch gesture away
     * without a `touchend`.
     */
    startAlignDrag(e) {
      if (!this.aligning || !this.alignAvailable) return;
      // Only the primary button drags. A right-button release generates no
      // click, so a right-drag must not move the image, must not arm the
      // toggle-click suppression, and must not be preventDefaulted -- that
      // would suppress the context menu.
      if (e.type === 'mousedown' && e.button !== 0) return;
      // The slider handle is inside the box and its own `mousedown` bubbles to
      // here, so without this an armed drag on the handle would move the reveal
      // position and the trailing image at the same time.
      const from = e.target;
      if (from && typeof from.closest === 'function' && from.closest('.compare-slider-handle')) return;
      const box = e.currentTarget;
      if (!box || typeof box.getBoundingClientRect !== 'function') return;
      const rect = box.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      // On `mousedown` this stops the browser starting a native image drag,
      // which would take the gesture over entirely. On `touchstart` it also
      // suppresses the synthesized `click`, so while armed a *tap* on toggle
      // mode's button would do nothing at all -- and it is not needed there:
      // `.compare-box-aligning` sets `touch-action: manipulation`, which
      // leaves the browser's pinch zoom alone while still declaring the
      // gesture ours to cancel, and the first `touchmove` prevents as well.
      if (e.type !== 'touchstart') e.preventDefault();
      const isTouch = e.type === 'touchstart';
      // A gesture that starts with two fingers is a pinch, the browser's own
      // magnification (`touch-action: manipulation` keeps it available), not
      // a drag: refuse to claim it.
      if (isTouch && e.touches && e.touches.length > 1) return;

      const point = (ev) => ({
        x: ev.clientX ?? ev.touches?.[0]?.clientX,
        y: ev.clientY ?? ev.touches?.[0]?.clientY,
      });
      let last = point(e);
      if (last.x === undefined || last.y === undefined) return;
      // Whether the *move handler* actually processed movement. Both the
      // announcement and the click suppression key on this: an offset can
      // change while the button is held without any drag -- a load converts it
      // mid-press -- and neither the live region nor the suppression may read
      // that as the reader's own gesture.
      let anyMove = false;

      // Defined before the move handler, which ends the gesture when the
      // reader disarms mid-drag. The removeEventListener calls name `moveHandler`
      // and resolve when this runs, by which time both exist.
      const upHandler = (mouseUpEvent) => {
        this._endAlignDrag = null;
        this._disarmAlignDrag = null;
        document.removeEventListener('mousemove', moveHandler);
        document.removeEventListener('mouseup', upHandler);
        document.removeEventListener('touchmove', moveHandler);
        document.removeEventListener('touchend', upHandler);
        document.removeEventListener('touchcancel', upHandler);
        window.removeEventListener('blur', upHandler);
        document.body.style.userSelect = '';
        // One announcement for the whole gesture, and only for an actual
        // drag: a press-and-release with no movement is not a drag, and an
        // offset that changed from a load mid-press is not the gesture's
        // doing -- announcing it would read a conversion as the reader's own
        // action.
        if (anyMove) this.announceOffset();
        // Only a drag that ended on the toggle-mode button can be followed by
        // the click that button will fire. Stamping regardless let a drag in
        // onion skin swallow the first click after switching to toggle.
        //
        // The stamp is armed by *movement*, not by the offset having changed:
        // a load that converts the offset mid-press is not a drag, and its
        // click is the reader's own. The click only follows when the release
        // landed on the button: released outside, no click is generated, and a
        // stamp would eat the reader's next deliberate click inside the
        // window. A touch drag is excluded too, because its click never comes:
        // the `preventDefault` the move handler calls on `touchmove` cancels
        // that gesture's compatibility click, and a gesture that moved has
        // necessarily cancelled it.
        if (anyMove && this.mode === 'toggle' && !isTouch
          && mouseUpEvent && mouseUpEvent.type === 'mouseup' && box.contains(mouseUpEvent.target)) {
          this._alignDragEndedAt = Date.now();
        }
      };

      const moveHandler = (moveE) => {
        // Disarming mid-gesture -- Space on the focused Align button while the
        // mouse is still held -- ends the drag: `toggleAligning` already
        // removed the move listeners, so this is a backstop for any other path
        // that could flip `aligning`.
        if (!this.aligning || !this.alignAvailable) {
          upHandler(moveE);
          return;
        }
        // A second finger turns the gesture into the browser's pinch zoom,
        // which `touch-action: manipulation` keeps ours to cancel by
        // preventing. Stop handling it and stop preventing -- and stop moving
        // the image -- so the pinch is the browser's.
        //
        // For the rest of the gesture, not just for this event: a finger
        // lifting must not hand the drag back. `last` is only written by a
        // move this handler processed, so resuming would move the image by
        // however far the remaining finger travelled during the pinch -- and
        // the browser's magnification has changed what a client coordinate
        // measures in the meantime. The move listeners go, exactly as a
        // mid-gesture disarm removes them, and the enders stay so the release
        // still finishes and announces the gesture.
        if (moveE.touches && moveE.touches.length > 1) {
          removeMoveListeners();
          return;
        }
        const next = point(moveE);
        if (next.x === undefined || next.y === undefined) return;
        // Prevented whatever the pointer did: a single-finger touchmove that
        // reports the same point is still a pan the browser would otherwise
        // scroll with.
        moveE.preventDefault();
        // ONLY the counting stops below: keep that call above this guard, or a
        // single-finger pan scrolls the page out from under the gesture.
        //
        // A move event that lands on the point it started from is not a
        // drag. Nothing travelled, so nothing can have moved the offset, and
        // counting it announces a correction nobody made and swallows the
        // click that would have switched versions.
        if (next.x === last.x && next.y === last.y) return;
        anyMove = true;
        // Measured per move, as `startSliderDrag`'s own handler measures its
        // container per move: a `load` landing mid-gesture corrects `_sizes`,
        // and the box can be re-laid-out under the pointer. Freezing either at
        // drag start converts the rest of the gesture with a stale ratio.
        //
        // `clientWidth` rather than the bounding rect: an absolutely positioned
        // child resolves its percentages against the padding box, and the rect
        // includes the border this box carries.
        const measured = this.overlayBox;
        const width = box.clientWidth || rect.width;
        if (!measured || !width) return;
        // Both axes convert through the *width* ratio: the box is width-driven,
        // so one box pixel is the same screen distance in either axis
        // (`renderedWidth / box.w`). The rendered height is a rounded integer,
        // so converting the y-axis through it breaks that for extreme aspect
        // ratios. `noteSizeFrom` converts through the width for the same
        // reason.
        const ratio = measured.w / width;
        // `physical`: this increment is a distance the hand covered on the
        // screen, and the conversion above put it in the pixels of whichever
        // box is standing -- not in the abstract units a key press means.
        this.nudge((next.x - last.x) * ratio, (next.y - last.y) * ratio, true);
        last = next;
      };

      const removeMoveListeners = () => {
        document.removeEventListener('mousemove', moveHandler);
        document.removeEventListener('touchmove', moveHandler);
      };
      this._endAlignDrag = upHandler;
      this._disarmAlignDrag = removeMoveListeners;

      document.body.style.userSelect = 'none';
      document.addEventListener('mousemove', moveHandler);
      document.addEventListener('mouseup', upHandler);
      document.addEventListener('touchmove', moveHandler, { passive: false });
      document.addEventListener('touchend', upHandler);
      document.addEventListener('touchcancel', upHandler);
      window.addEventListener('blur', upHandler);
    },

    /**
     * Wheel over the box zooms while armed.
     *
     * The box is a `div`, so the listener is non-passive by default and the
     * `preventDefault` actually takes -- without it the page scrolls under a
     * gesture the reader meant for the image.
     */
    onAlignWheel(e) {
      if (!this.aligning || !this.alignAvailable) return;
      // Ctrl+wheel is the browser's own magnification, and a trackpad pinch
      // arrives as exactly that. Taking it would leave the page unzoomable for
      // as long as the reader is aligning, which costs more than this gains.
      if (e.ctrlKey || e.metaKey) return;
      // A sideways trackpad swipe reports `deltaY` 0. Reading that through
      // `deltaY < 0` calls it "not up" and zooms *out* on a horizontal gesture.
      if (!e.deltaY) return;
      e.preventDefault();
      this.zoomBy(e.deltaY < 0 ? ALIGN_ZOOM_STEP : -ALIGN_ZOOM_STEP);
      this.announceOffsetWhenSettled();
    },

    /**
     * Announce once the gesture stops, not once per event.
     *
     * One wheel notch delivers a burst, so announcing per event is the
     * `pointermove` mistake in another shape: a queue the reader then sits
     * through. This is the wheel's equivalent of announcing at the end of a
     * drag.
     */
    announceOffsetWhenSettled() {
      if (this._announceTimer) clearTimeout(this._announceTimer);
      this._announceTimer = setTimeout(() => {
        this._announceTimer = null;
        this.announceOffset();
      }, ALIGN_ANNOUNCE_SETTLE_MS);
    },

    /**
     * Whether a scale choice would do anything.
     *
     * With no usable dimensions on either side every mode returns the same empty
     * style and the CSS fallback draws the pair, so the control refuses rather
     * than being drawn as though it worked.
     */
    get scaleAvailable() {
      return this.overlayRatio !== null;
    },

    /**
     * Pixel diff deliberately refuses HEIC/TIFF (the Package 3 format
     * contract), even in a browser that happens to gain a decoder. TIFF often
     * has stored dimensions from Go, so geometry alone cannot enforce that
     * contract. Strip media-type parameters and accept common aliases so an
     * upload cannot arm a computation that this feature promises to refuse.
     */
    get heatMapUnsupportedFormat() {
      return this._contentTypes.some(heatMapUnsupportedContentType);
    },

    get heatMapAvailable() {
      return this.scaleAvailable && !this.heatMapUnsupportedFormat;
    },

    get heatMapUnavailableTitle() {
      if (this.heatMapUnsupportedFormat) {
        return 'Pixel diff does not support HEIC or TIFF; Difference and Blink remain available.';
      }
      if (!this.scaleAvailable) {
        return 'One of the two versions reports no dimensions, so there is nothing to compare pixel by pixel.';
      }
      return 'Show which parts of the pair changed, and how much.';
    },

    setScale(value, e) {
      if (!this.scaleAvailable) {
        this.returnFocusToCheckedScale(e);
        return;
      }
      // Fit and Stretch render the version at a different size, which moves the
      // bound the offset was checked against. A press that selects what is
      // already selected changes no geometry, so it must not arm the deferred
      // rebound either: nothing was broken, so nothing is owed.
      if (value === this.scale) return;
      const within = this.offsetAxesWithinBound;
      this.scale = value;
      this._reboundOrDefer(within);
      this._scheduleHeatMapRepaint(true);
    },

    /**
     * The roving tabindex invariant, enforced where activation happens.
     *
     * `aria-disabled` suppresses neither focus nor pointer events, so focus can
     * come to rest on a refused `tabindex="-1"`, `aria-checked="false"` radio
     * while the checked one still holds `tabindex="0"`. The group's focus is
     * supposed to be on the checked radio, and Shift+Tab back into it would
     * otherwise skip the group entirely.
     *
     * On activation, not on one input path. Guarding `mousedown` alone leaves
     * the hole open for programmatic focus, for an assistive technology moving
     * focus directly, and for any touch implementation that focuses without a
     * compatibility `mousedown`: Enter and Space on such a radio produce a
     * refused click and leave focus exactly where it should not be. Every one
     * of those paths ends in the click handler, so that is where the invariant
     * is restored.
     */
    returnFocusToCheckedScale(e) {
      const target = e && e.currentTarget;
      const group = target && typeof target.closest === 'function' && target.closest('[role="radiogroup"]');
      if (!group) return;
      const active = typeof document !== 'undefined' ? document.activeElement : null;
      // Correct the invariant only where it is actually broken -- focus resting
      // on an unchecked radio of this group. Restoring unconditionally would
      // steal focus on the commonest path of all: the pointer press never moved
      // it (`refuseFocusIfUnavailable` saw to that), so the reader is still in
      // whatever field or control they were using, and this would yank them into
      // a group they only clicked at.
      if (!active || !group.contains(active)) return;
      if (active.getAttribute && active.getAttribute('aria-checked') === 'true') return;
      const checked = group.querySelector('[role="radio"][aria-checked="true"]');
      if (checked && typeof checked.focus === 'function' && checked !== active) checked.focus();
    },

    /**
     * Avoid the focus visibly landing on a refused radio before it is taken back.
     *
     * Purely cosmetic, and deliberately not the mechanism: `mousedown` and
     * `click` are separate tasks with a frame between them, so without this the
     * pointer path shows a focus ring on a dead control for one frame before
     * `returnFocusToCheckedScale` moves it. Discoverability is unaffected either
     * way -- roving tabindex means an unchecked radio was never a tab stop, and
     * Tab still enters the group at the checked one, where the group's own
     * `aria-disabled` is announced.
     */
    refuseFocusIfUnavailable(e) {
      if (!this.scaleAvailable) e.preventDefault();
    },

    /**
     * Whether an anchor choice would do anything.
     *
     * Stretch leaves no slack -- both versions already fill the frame exactly --
     * so there is nothing for an anchor to take up, and a control that looked
     * pressed while changing nothing would be worse than one that says it cannot
     * act. Kept visible and marked disabled rather than hidden: one button
     * vanishing out of a toolbar the reader is looking at is more disorienting
     * than a visibly unavailable one, and it keeps the row from twitching on
     * every scale change.
     */
    get anchorAvailable() {
      return this.scaleAvailable && this.scale !== 'stretch';
    },

    toggleAnchor() {
      if (!this.anchorAvailable) return;
      // The anchor decides where the version rests before any offset, so it
      // decides how far the offset may carry it.
      const within = this.offsetAxesWithinBound;
      this.anchor = this.anchor === 'top-left' ? 'center' : 'top-left';
      this._reboundOrDefer(within);
      this._scheduleHeatMapRepaint(true);
    },

    /**
     * The refusal has to cover the keyboard too.
     *
     * `onRadiogroupKeydown` assigns straight into state, so arrow keys, Home and
     * End would move a control that announces itself unavailable -- a guard on
     * `@click` alone leaves the group working for anyone not using a mouse.
     */
    onScaleKeydown(e) {
      if (!this.scaleAvailable) {
        // Refuse the change, but still swallow the keys the pattern owns. The
        // checked radio stays focusable and is the group's tab stop, so letting
        // ArrowDown, Home or End through hands them to the browser's default
        // scrolling -- Home jumps the reader to the top of the page as the answer
        // to pressing a key on a control that told them it could not act.
        if (RADIOGROUP_KEYS.includes(e.key)) e.preventDefault();
        return;
      }
      // With the rebound as the callback, not left to `setScale`: this path
      // assigns `this.scale` through the generic radiogroup handler and never
      // touches `setScale` at all, so a scale changed by arrow key kept an
      // offset the new sizing had put outside the frame. The within-bound
      // snapshot has to be taken here, before the assignment the callback runs
      // after -- and the callback has to go through `_reboundOrDefer` like the
      // mouse path, and only when the value actually changed (Home on the
      // selected radio changes nothing and must not arm a deferred rebound).
      const within = this.offsetAxesWithinBound;
      const previous = this.scale;
      this.onRadiogroupKeydown(e, 'scale', ['relative', 'fit', 'stretch'],
        () => {
          if (this.scale !== previous) {
            this._reboundOrDefer(within);
            this._scheduleHeatMapRepaint(true);
          }
        });
    },

    /**
     * The overlay box's own style, for the same reason as `overlayScale`.
     *
     * `index.css` keys its no-dimensions fallback on `[style*="aspect-ratio"]`,
     * which an object binding still satisfies: setting the property serialises
     * into the attribute exactly as a string binding did.
     */
    get overlayBoxStyle() {
      return { aspectRatio: this.overlayRatio || '' };
    },

    /**
     * Every `_sizes` / `_urls` slot whose bytes an image is currently displaying.
     *
     * Resolved from the image's own `currentSrc`, never from `swapped`. Those
     * arrays are indexed by the server's left/right while `leadUrl` is
     * `_urls[swapped ? 1 : 0]`, so asking `swapped` means asking a flag that can
     * move independently of the pixels being measured: a `load` already queued
     * for the previous `src` can run after a flip, and the element still reports
     * the old image's `naturalWidth` while the new request is pending. That
     * records a real measurement against the wrong version, and it stays wrong
     * if the replacement never loads.
     *
     * `currentSrc` is defined as the URL of the image data *currently in use*,
     * so it and `naturalWidth` describe the same bytes by construction. Both
     * sides are resolved against one base because `currentSrc` is absolute and
     * `_urls` are not.
     *
     * *Every* matching slot, not the first: `sameVersion` keeps a same-resource
     * self-comparison from rendering this component at all, but it is
     * `!crossResource && v1 == v2`, so a cross-resource URL naming one version
     * on both sides reaches it with two identical URLs. Filling only the first
     * slot would leave the second unknown, which for a format that stores no
     * dimensions means the scale controls stay refused while both images have
     * decoded perfectly well.
     */
    slotsForImage(img) {
      if (!img.currentSrc) return [];
      const base = (typeof document !== 'undefined' && document.baseURI) || 'http://compare.invalid/';
      const resolve = (u) => {
        try {
          return new URL(u, base).href;
        } catch {
          return String(u);
        }
      };
      const showing = resolve(img.currentSrc);
      const slots = [];
      this._urls.forEach((u, i) => {
        if (resolve(u) === showing) slots.push(i);
      });
      return slots;
    },

    /**
     * Fill `_sizes` from what the browser actually painted.
     *
     * The stored `Width`/`Height` are whatever Go's `image.DecodeConfig` could
     * read at upload, and they are wrong in two directions. They are *absent*
     * for a format no Go decoder exists for -- AVIF is an accepted content type
     * with none anywhere in the tree -- which drops the pair into the CSS branch
     * that stacks both images at one origin with no registration at all. And
     * they *disagree* for anything carrying EXIF orientation, which nothing in
     * this tree reads: a rotated JPEG stores its dimensions transposed against
     * what every browser paints and reports.
     *
     * So the loaded image wins whenever it has something to say. The stored
     * values remain the first-paint placeholder, which is what keeps the box
     * from resizing under the reader between markup and load. Which version a
     * measurement belongs to is decided by `slotsForImage`, from the image
     * itself.
     */
    noteSizeFrom(img) {
      // Duck-typed rather than `instanceof HTMLImageElement`: that identity is
      // per-realm, so an image adopted from another document is not an instance
      // of *this* realm's constructor and would be silently ignored. What is
      // actually required of the argument is the two numbers read below.
      if (!img || typeof img.naturalWidth !== 'number' || typeof img.naturalHeight !== 'number') return;
      const w = img.naturalWidth;
      const h = img.naturalHeight;
      // Zero means the image has not decoded, or cannot: a dimensionless SVG,
      // a HEIC no browser renders, a fetch that failed. Recording that would
      // erase a usable stored value in exchange for nothing.
      if (!w || !h) return;
      // An image showing neither version has nothing to attribute the size to.
      const slots = this.slotsForImage(img);
      if (slots.length === 0) return;
      for (const slot of slots) this._measured[slot] = true;
      // A new array rather than an index write, and only when something actually
      // changed: the getters derived from this have to re-run, and eight images
      // report into two slots, so this runs far more often than it changes
      // anything.
      let next = null;
      for (const slot of slots) {
        const known = this._sizes[slot];
        if (known && known.w === w && known.h === h) continue;
        if (!next) next = this._sizes.slice();
        next[slot] = { w, h };
      }
      // What the reports have done to the reader's position, measured against
      // that position rather than against whatever the previous report left:
      // taken once, at the reader's last act, and held until they act again --
      // see `_reportBefore`. So an axis a flip legitimately left outside stays
      // the reader's whatever a report does to the bound in between. In the
      // canonical frame, because the answer must not depend on which version
      // decoded first -- see `_canonically`.
      const withinAxes = this._reportBefore
        || (this._reportBefore = this._canonically(() => this.offsetAxesWithinBound));
      const before = this.overlayBox;
      // The offset is in box pixels, so correcting the box's own dimensions
      // changes what an existing one means. A reader who nudged against the
      // stored placeholder and then saw the real dimensions arrive would watch
      // their correction change size on its own.
      //
      // **Both** components scale by the *width* ratio. The box is
      // width-driven: it takes the container's width and derives its height
      // from `aspect-ratio`, so one box pixel is the same distance on screen in
      // either axis -- `renderedWidth / box.w`. Scaling each axis by its own
      // ratio looks right and is not: correcting only the height moves no
      // physical distance at all, and would double a vertical offset.
      if (next) {
        this._sizes = next;
        const after = this.overlayBox;
        if (before && after && before.w !== after.w) {
          const ratio = after.w / before.w;
          // At identity there is nothing for the conversion to move.
          if (!this.offsetIsIdentity) {
            this._offset = { dx: this._offset.dx * ratio, dy: this._offset.dy * ratio, k: this._offset.k };
          }
          // The request is *not* converted here: it is denominated in
          // stored-box pixels, which the box's own growth does not touch, and
          // the deferred rebound converts it once, into the final box, when
          // both versions have reported. Converting it report by report would
          // chain the ratio onto a value whose meaning never lived in the
          // transient box in the first place -- the order-dependence the
          // stored-box denomination exists to remove.
        }
      }
      // The versions changed size too, so the bound moved with them -- but only
      // once **both** have reported. The box is `max` of the two, so while one
      // is a real measurement and the other still the stored placeholder it
      // describes a pair that does not exist, and clamping against that makes
      // the result depend on which image happened to decode first: the same two
      // files land on a different offset in a different order.
      //
      // The repair waits for the pair rather than following the size change,
      // because the report that completes the pair may be one that only
      // *confirms* a stored size: it flips `_measured` without touching
      // `_sizes`, and it is still the moment the geometry becomes real.
      const both = this._measured[0] && this._measured[1];
      // Paid when both have reported -- whenever there is a correction or an
      // outstanding request for the pay to have an opinion about, since it is
      // the pay that works out whether anything is owed. One image showing on
      // both sides fills both slots in a single report, so the completing call
      // can also be the size change whose repair it performs.
      if (both && (this.offsetIsPlaced || this._requestedOffset !== null)) {
        // Canonically, always, and that is the whole of the rule rather than
        // a choice made per pay. A request resolved in the view the reader
        // made it in honours what they watched land, but *which* view that is
        // stops being the only input the moment a bound-mover also has a
        // claim outstanding -- and whether a report crosses a bound depends on
        // the transient geometry it lands in, which is the decode order. One
        // frame for everything deferred is what makes the canonical result a
        // function of the reader's script and the final geometry alone. It
        // costs the flipped window its exactness: a correction that landed on
        // the flipped bound moves when the second version arrives, which is
        // the price already charged for a zoom taken while flipped. The flip
        // itself still never reapplies the bound.
        this._canonically(() => this._resolveDeferred(withinAxes));
        // The repair those claims asked for has run, so what stands now is
        // what a later report is measured against.
        this._reportBefore = null;
      }
      // Either way the pair's geometry has moved closer to final: repaint the
      // mask if the reader armed it. While half-measured this is a no-op (the
      // guard refuses to compute against a placeholder), and the report that
      // completes the pair -- even one that only confirms a stored size and
      // writes no sizes -- is the moment the heatmap computes.
      this._scheduleHeatMapRepaint();
    },

    get leadScale() {
      return this.overlayScale(this.swapped ? 1 : 0);
    },
    get trailScale() {
      return this.overlayScale(this.swapped ? 0 : 1);
    },

    /**
     * Where one version's rectangle sits in the frame, in box pixels.
     *
     * The heatmap re-derives what the CSS actually paints -- scale policy,
     * anchor, and the reader's own correction -- from the numbers the style
     * bindings already use (`elementSize`, the anchor's rest position,
     * `trailOffset`), so the canvas and the screen can never disagree about
     * what is being compared. A pixel maps to `origin + k(p - origin) +
     * (dx, dy)`, which is exactly the CSS transform `translate(dx%, dy%)
     * scale(k)` about `transform-origin`: the translate resolves against the
     * element's own size, so `dx` box pixels of a `w`-wide element cancel the
     * percentage and come back out as `dx` canvas pixels.
     *
     * Per *slot*, like `elementSize`: `swapped` decides which slot trails,
     * and a flip moves the correction between slots inverted rather than
     * mutating anything, exactly as the styles do.
     */
    heatMapPlacement(index) {
      const box = this.overlayBox;
      if (!box) return null;
      const el = this.elementSize(index);
      if (!el) return null;
      const anchored = this.anchor === 'top-left';
      const restX = anchored ? 0 : (box.w - el.w) / 2;
      const restY = anchored ? 0 : (box.h - el.h) / 2;
      // One origin rule for both slots -- the point the CSS transform-origin
      // names for this element. The lead is never transformed, so its origin
      // is inert, but it is computed rather than zeroed so the object cannot
      // carry a value that would be wrong if anything ever read it.
      const originX = anchored ? restX : restX + el.w / 2;
      const originY = anchored ? restY : restY + el.h / 2;
      if (index !== (this.swapped ? 0 : 1)) {
        // The reference version: at rest, untransformed.
        return { x: restX, y: restY, w: el.w, h: el.h, k: 1, dx: 0, dy: 0, originX, originY };
      }
      const { dx, dy, k } = this.trailOffset;
      return {
        x: restX, y: restY, w: el.w, h: el.h, k, dx, dy,
        originX, originY,
      };
    },

    /**
     * The heatmap's sampling resolution for the current frame.
     *
     * The box scaled down until neither side exceeds the cap, never up. Both
     * compose canvases share it, so the per-repaint pixel work is bounded by
     * the cap whatever the source images measure.
     */
    heatMapSampleSize() {
      const box = this.overlayBox;
      if (!box) return null;
      const s = Math.min(HEATMAP_SAMPLE_SIDE / box.w, HEATMAP_SAMPLE_SIDE / box.h, 1);
      return { w: Math.max(1, Math.round(box.w * s)), h: Math.max(1, Math.round(box.h * s)) };
    },

    /**
     * Whether computing the heatmap asks first.
     *
     * The cost that grows without bound is the source decode-and-downsample,
     * which scales with the versions' own megapixels -- the sampling canvas
     * above already bounds the per-repaint work. Both sides count, since both
     * are drawn every repaint. False when either size is unknown: the pair
     * refuses on `scaleAvailable` before this is ever asked.
     */
    heatMapRequiresConfirm() {
      const [a, b] = this._sizes;
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) return false;
      return (a.w * a.h + b.w * b.h) / 1e6 > HEATMAP_CONFIRM_ABOVE_MEGAPIXELS;
    },

    /** "13.2 megapixels" -- the real number the gate's notice carries, so the
     *  person deciding can see what they are agreeing to (textDiff's rule). */
    heatMapMegapixelsLabel() {
      const [a, b] = this._sizes;
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) return '';
      return `${((a.w * a.h + b.w * b.h) / 1e6).toFixed(1)} megapixels`;
    },

    /**
     * Whether the blink comparator may act at all.
     *
     * Blink needs no geometry -- a pair the browser cannot measure still has
     * two images to alternate between -- so the one thing it must honour is
     * the OS's reduced-motion request. A vestibular reader's browser says
     * "no self-moving UI" and this control obeys it, drawn unavailable with
     * the reason attached rather than silently swallowing a press.
     */
    get blinkAvailable() {
      return !this._reducedMotion;
    },

    // Above the WCAG three-flashes boundary, the image is deliberately
    // flattened into a narrow luminance band. It still alternates at the rate
    // the reader selected, without an arbitrary pair becoming a seizure risk.
    get blinkFlashSafe() {
      return this.blinking && this.blinkRate > 3;
    },

    get reducedMotionTitle() {
      return this.blinkAvailable
        ? 'Alternate the two versions automatically.'
        : 'Your system requests reduced motion, so the blink comparator is unavailable.';
    },

    get blinkRateMin() {
      return BLINK_RATE_MIN;
    },

    get blinkRateMax() {
      return BLINK_RATE_MAX;
    },

    _resolveReducedMotion() {
      if (this._reducedMotionQuery) return;
      if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
      // Duck-typed rather than assumed: the query object, its listener
      // registration and its `addEventListener` are all recent additions to
      // some embedders, and a missing one must degrade to "allowed", not to
      // an exception at init.
      const query = window.matchMedia('(prefers-reduced-motion: reduce)');
      this._reducedMotionQuery = query;
      this._reducedMotion = !!query.matches;
      if (typeof query.addEventListener !== 'function') return;
      this._reducedMotionListener = (e) => {
        this._reducedMotion = e.matches;
        // A preference arriving mid-play stops the blink: the request is
        // about what the screen does, not about who started it.
        if (e.matches && this.blinking) this.pauseBlink();
      };
      query.addEventListener('change', this._reducedMotionListener);
    },

    toggleBlink() {
      if (this.blinking) {
        this.pauseBlink();
        return;
      }
      this._resolveReducedMotion();
      if (!this.blinkAvailable) return;
      this.blinking = true;
      this._startBlinkInterval();
      if (this.blinkFlashSafe) this._scheduleHeatMapRepaint(true);
      this._announceBlinkState();
    },

    /** One flip per tick, straight to `showLeft`. Not `toggleSide`: that
     *  carries the pointer click's drag-suppression and `detail` logic, and
     *  an interval tick is neither. */
    _startBlinkInterval() {
      const rate = Math.max(BLINK_RATE_MIN, Math.min(BLINK_RATE_MAX, this.blinkRate || BLINK_RATE_DEFAULT));
      this._blinkAppliedRate = rate;
      this._blinkTimer = setInterval(() => { this.showLeft = !this.showLeft; }, Math.round(1000 / rate));
    },

    pauseBlink() {
      if (!this.blinking) return;
      const wasFlashSafe = this.blinkFlashSafe;
      this.blinking = false;
      if (this._blinkTimer !== null) {
        clearInterval(this._blinkTimer);
        this._blinkTimer = null;
      }
      if (wasFlashSafe) this._scheduleHeatMapRepaint(true);
      this._announceBlinkState();
    },

    blinkRateChanged() {
      // Restart only an interval that is running; a rate set while stopped
      // takes effect at the next start.
      if (this.blinking) {
        const wasFlashSafe = this._blinkAppliedRate > 3;
        if (this._blinkTimer !== null) clearInterval(this._blinkTimer);
        this._startBlinkInterval();
        if (wasFlashSafe !== this.blinkFlashSafe) this._scheduleHeatMapRepaint(true);
      }
      // A rate change is one discrete event, announced once -- never once
      // per `input` the range fires while the reader drags it. While paused,
      // name the new setting without falsely saying playback started.
      if (this.blinking) {
        this._announceBlinkState();
      } else {
        this._blinkAnnounceParity = !this._blinkAnnounceParity;
        this.blinkAnnouncement = `Blink rate set to ${Math.round(this.blinkRate)} flashes per second${this._blinkAnnounceParity ? ANNOUNCE_MARK : ''}`;
      }
    },

    _announceBlinkState() {
      this._blinkAnnounceParity = !this._blinkAnnounceParity;
      const message = this.blinking
        ? `Blinking at ${Math.round(this.blinkRate)} flashes per second${this.blinkFlashSafe ? '; contrast reduced for flash safety' : ''}`
        : 'Blink paused';
      this.blinkAnnouncement = `${message}${this._blinkAnnounceParity ? ANNOUNCE_MARK : ''}`;
    },

    /**
     * Arm or disarm the pixel-diff mask.
     *
     * Armed state survives a mode switch (the alignment's rule): the reader
     * who aligned in onion skin and checks the result in toggle mode should
     * not have to re-arm the mask per mode. Disarming is what the toggle is
     * for.
     *
     * Over the confirm gate, arming stops at the notice -- textDiff's shape:
     * the computation has not started. `heatMapConfirmed` remembers the
     * consent for the session, so a disarmed-then-rearmed pair does not ask
     * twice.
     */
    toggleHeatMap() {
      if (!this.heatMapAvailable) return;
      if (this.heatMapOn) {
        this.heatMapOn = false;
        this.heatMapPercent = null;
        this.heatMapOverlapEmpty = false;
        this._emitHeatMapPercent();
        this._announceHeatMapState();
        return;
      }
      if (this.heatMapRequiresConfirm() && !this.heatMapConfirmed) {
        this.heatMapNeedsConfirm = true;
        // The gate appears away from the toolbar button. Move keyboard focus
        // to its primary action after Alpine reveals it, so the request and
        // its real megapixel count cannot appear silently off-focus.
        if (typeof this.$nextTick === 'function') {
          this.$nextTick(() => {
            this._root?.querySelector('[data-heatmap-confirm]')?.focus();
          });
        }
        return;
      }
      this.heatMapOn = true;
      this.heatMapNeedsConfirm = false;
      this.heatMapPercent = null;
      this.heatMapOverlapEmpty = false;
      // The completing-report path computes on its own while half-measured;
      // a whole pair computes now. The announcement waits for that paint: in
      // a browser this request runs in rAF, so announcing here would see null.
      this._scheduleHeatMapRepaint(true);
    },

    confirmHeatMap() {
      this.heatMapConfirmed = true;
      this.heatMapNeedsConfirm = false;
      this.toggleHeatMap();
      this._restoreHeatMapToggleFocus();
    },

    dismissHeatMapConfirm() {
      this.heatMapNeedsConfirm = false;
      this._restoreHeatMapToggleFocus();
    },

    _restoreHeatMapToggleFocus() {
      if (typeof this.$nextTick !== 'function') return;
      this.$nextTick(() => {
        this._root?.querySelector('[data-heatmap-toggle]')?.focus();
      });
    },

    /**
     * One event for the banner, whatever changed: a percentage, "no
     * overlap", or "hidden" (percent null). The banner is outside this
     * component's scope and shared by every comparator category, so it owns
     * its own state and listens on window.
     */
    _emitHeatMapPercent() {
      if (typeof window === 'undefined' || typeof window.dispatchEvent !== 'function') return;
      window.dispatchEvent(new CustomEvent('compare-pixel-diff', {
        detail: { percent: this.heatMapPercent, overlapEmpty: this.heatMapOverlapEmpty },
      }));
    },

    /**
     * Repaint the mask for the pair as it stands.
     *
     * Silent: during a drag this runs per animation frame, and a live region
     * that reads the percentage per frame is the pointermove mistake. The
     * announcements happen at the discrete ends (`_announceHeatMapState`).
     */
    _repaintHeatMap() {
      if (!this.heatMapOn || !this.heatMapAvailable) return false;
      // A half-measured pair has no final frame: the box is max of one real
      // measurement and one stored placeholder, so a number computed there
      // depends on which version decoded first. The completing report paints.
      if (!this._measured[0] || !this._measured[1]) return false;
      const result = this._repaintHeatMapDom();
      if (!result) return false;
      const { changed, overlap } = result;
      this.heatMapOverlapEmpty = overlap === 0;
      this.heatMapPercent = formatHeatMapPercent(changed, overlap);
      this._emitHeatMapPercent();
      return true;
    },

    /**
     * Coalesce repaint requests into one frame.
     *
     * A drag fires this per move event; without coalescing the compose +
     * getImageData runs per event, which is the phone that hangs. Runs
     * directly when no rAF exists (the unit suite), so the writers' explicit
     * requests are always honoured somewhere.
     */
    _scheduleHeatMapRepaint(announceAfter = false) {
      // Alignment is the default path and the mask is off by default. Do not
      // allocate one rAF per drag frame for a computation that will refuse.
      if (!this.heatMapOn) return;
      if (announceAfter) this._heatMapAnnounceAfterRepaint = true;
      const repaint = () => {
        const complete = this._repaintHeatMap();
        // A half-measured arm keeps the request pending. The report that makes
        // the pair whole schedules another repaint and is the one that speaks.
        if (complete && this._heatMapAnnounceAfterRepaint) {
          this._heatMapAnnounceAfterRepaint = false;
          this._announceHeatMapState();
        }
      };
      if (typeof requestAnimationFrame === 'function') {
        if (this._heatMapFrame !== null) return;
        this._heatMapFrame = requestAnimationFrame(() => {
          this._heatMapFrame = null;
          repaint();
        });
        return;
      }
      repaint();
    },

    /**
     * The live region for the percentage, written on discrete events only.
     * Silent when nothing is computed: an uncomputed mask has no number to
     * say, and repeating an unchanged one announces nothing anyway.
     */
    _announceHeatMapState() {
      this._heatMapAnnounceParity = !this._heatMapAnnounceParity;
      if (!this.heatMapOn) {
        this.heatMapAnnouncement = `Pixel diff hidden${this._heatMapAnnounceParity ? ANNOUNCE_MARK : ''}`;
        return;
      }
      if (this.heatMapPercent === null && !this.heatMapOverlapEmpty) return;
      const message = this.heatMapOverlapEmpty
        ? 'Pixel diff: the two versions do not overlap, so nothing was compared'
        : `Pixel diff: ${this.heatMapPercent}% of the overlap changed`;
      this.heatMapAnnouncement = `${message}${this._heatMapAnnounceParity ? ANNOUNCE_MARK : ''}`;
    },

    /**
     * The canvas half of the heatmap: compose each version the way the CSS
     * paints it, diff, paint the mask.
     *
     * The compose canvases are `overlayBox` scaled to the sampling resolution
     * and re-used while it holds. Each version is drawn through
     * `heatMapPlacement`'s numbers -- the same rest rectangle, transform and
     * origin the style bindings emit -- so what is compared is what is on
     * screen, in frame pixels. `getImageData` is clean: the version files are
     * served same-origin.
     *
     * Returns the change counts, or null when a slot's bytes cannot be drawn
     * (the image element has not decoded, or shows neither version).
     */
    _repaintHeatMapDom() {
      if (typeof document === 'undefined') return null;
      const box = this.overlayBox;
      const sample = this.heatMapSampleSize();
      if (!box || !sample) return null;
      const el = this._root;
      if (!el || typeof el.querySelectorAll !== 'function') return null;

      const sx = sample.w / box.w;
      const sy = sample.h / box.h;
      let scratch = this._heatMapScratch;
      if (!scratch || scratch.w !== sample.w || scratch.h !== sample.h) {
        scratch = {
          w: sample.w, h: sample.h,
          compose: [document.createElement('canvas'), document.createElement('canvas')],
        };
        for (const c of scratch.compose) { c.width = sample.w; c.height = sample.h; }
        this._heatMapScratch = scratch;
      }

      // One element per slot: the first data-compare-image whose currentSrc
      // names that slot. The eight-plus images across the modes all hold the
      // same two decodes; whichever loaded is fine to draw from.
      const sources = [null, null];
      for (const imgEl of el.querySelectorAll('img[data-compare-image]')) {
        for (const slot of this.slotsForImage(imgEl)) {
          if (!sources[slot] && imgEl.complete && imgEl.naturalWidth > 0) sources[slot] = imgEl;
        }
      }
      if (!sources[0] || !sources[1]) return null;

      const contexts = scratch.compose.map((c) => c.getContext('2d', { willReadFrequently: true }));
      for (let slot = 0; slot < 2; slot++) {
        const ctx = contexts[slot];
        ctx.setTransform(1, 0, 0, 1, 0, 0);
        ctx.clearRect(0, 0, sample.w, sample.h);
        const p = this.heatMapPlacement(slot);
        if (!p) return null;
        ctx.setTransform(...heatMapCanvasTransform(p, sx, sy));
        ctx.drawImage(sources[slot], p.x * sx, p.y * sy, p.w * sx, p.h * sy);
        ctx.setTransform(1, 0, 0, 1, 0, 0);
      }

      const leadData = contexts[0].getImageData(0, 0, sample.w, sample.h);
      const trailData = contexts[1].getImageData(0, 0, sample.w, sample.h);
      const rendering = this.blinkFlashSafe
        ? { backdrop: BLINK_FLASH_BACKDROP, contrast: BLINK_FLASH_CONTRAST }
        : undefined;
      const { changed, overlap, mask } = heatMapDiff(
        leadData.data, trailData.data, HEATMAP_THRESHOLD, rendering,
      );

      // The mask is already in pixel order. Pick the canvas by component state,
      // not rendered visibility: Alpine applies `x-show` after the state change,
      // so an rAF can observe every mode box hidden even though the selected one
      // becomes visible later in the same update. Painting the selected mode's
      // canvas first makes that ordering irrelevant.
      const maskData = new ImageData(mask, sample.w, sample.h);
      const canvas = el.querySelector(`canvas[data-compare-heatmap="${this.mode}"]`);
      if (canvas) {
        if (canvas.width !== sample.w || canvas.height !== sample.h) { canvas.width = sample.w; canvas.height = sample.h; }
        canvas.getContext('2d').putImageData(maskData, 0, 0);
      }
      return { changed, overlap };
    },

    init() {
      // Resolved here rather than at the first press, because the control's
      // availability is rendered before the reader ever touches it: a reduced
      // motion preference must show the button unavailable on first paint,
      // not announce it healthy and then refuse.
      this._resolveReducedMotion();

      // The component's own root, captured while `init` runs -- the only time
      // `$el` is guaranteed to name it. Read from a method, `$el` is whichever
      // element's expression made the call: the Pixel diff button's click
      // hands the heatmap a root with no images in it. textDiff carries the
      // same capture for the same reason.
      this._root = this.$el;

      // Arrow keys nudge the slider or the onion opacity. The handler is on the
      // container rather than the document, and skips events the mode radiogroup
      // is already handling: on the document it also fired for the radiogroup's
      // own arrow keys, so one press changed the mode and then moved whatever the
      // new mode's control was.
      this._keyHandler = (e) => {
        if (e.defaultPrevented) return;
        // Ctrl/Cmd/Alt keys are the browser's -- Ctrl+R reloads, Ctrl+/- zooms
        // the page, Alt+ArrowLeft goes back -- not the shortcuts below, which
        // are the plain keys with Shift as their step modifier. Refusing here
        // (and never preventing) leaves the browser's own action alone.
        if (e.ctrlKey || e.metaKey || e.altKey) return;
        const target = e.target;
        // A text field owns every key: `R` in one means the letter, not a
        // reset. This has to be its own guard, because the alignment's non-
        // arrow keys pass the one below. A range input is *not* a text field:
        // it answers the arrows and Home/End itself, but `+`, `-` and `R` are
        // not its keys, so they still reach the armed alignment from it.
        if (target instanceof HTMLElement && target.closest('input:not([type="range"]), select, textarea')) return;
        // Anything else focusable answers its own arrow keys. Without this the
        // reveal position moved while Flip had focus, which is a control that
        // has nothing to do with it. The guard is deliberately scoped to the
        // keys a focusable control can own -- arrows plus the radiogroup
        // pattern's Home and End -- so `+`, `-` and `R`, which no button or
        // radio answers, still reach the alignment when it is armed: a reader
        // who aligned, then clicked Flip or Anchor, can reset without moving
        // focus back first.
        const focusable = target instanceof HTMLElement
          && target.closest('button, a[href], input, select, textarea, [role="radiogroup"], [tabindex]');
        if (focusable && RADIOGROUP_KEYS.includes(e.key)) return;
        // While armed, the alignment owns the arrow keys outright. That is the
        // whole point of arming: they are otherwise spent on the slider
        // position and the onion opacity, and no modifier is free to
        // disambiguate them.
        if (this.aligning && this.handleAlignKey(e)) return;
        const step = e.shiftKey ? 10 : 2;
        if (this.mode === 'slider') {
          if (e.key === 'ArrowLeft') { this.nudgeSlider(-step); e.preventDefault(); }
          else if (e.key === 'ArrowRight') { this.nudgeSlider(step); e.preventDefault(); }
          else if (e.key === 'Home') { this.sliderPos = 1; e.preventDefault(); }
          else if (e.key === 'End') { this.sliderPos = 99; e.preventDefault(); }
        } else if (this.mode === 'onion') {
          if (e.key === 'ArrowLeft') { this.opacity = Math.max(0, this.opacity - step); e.preventDefault(); }
          else if (e.key === 'ArrowRight') { this.opacity = Math.min(100, this.opacity + step); e.preventDefault(); }
        }
      };
      this.$el.addEventListener('keydown', this._keyHandler);

      // Leaving toggle mode pauses the blink: it alternates the toggle box's
      // own images, and a blink the reader can no longer see is a timer that
      // runs for nothing. Duck-typed because the unit suite drives `init`
      // with no Alpine behind it, and the pause itself is tested directly.
      // The same watcher repaints the heatmap mask -- a new mode means a new
      // overlay box, whose mask canvas has to be painted.
      if (typeof this.$watch === 'function') {
        this.$watch('mode', () => {
          if (this.mode !== 'toggle') this.pauseBlink();
          this._scheduleHeatMapRepaint(true);
        });
        // The repaint net for the heatmap: every state that moves the pair is
        // watched, so a future writer that forgets to ask for a repaint still
        // gets one. The current writers also ask directly, because the unit
        // suite -- and any embedded harness without Alpine -- runs without
        // reactivity, and there the explicit calls are the only ones there.
        for (const key of ['_offset', '_sizes', 'scale', 'anchor', 'swapped']) {
          this.$watch(key, () => this._scheduleHeatMapRepaint());
        }
      }

      // An image that finished before its `@load` was bound never fires one.
      // Alpine's own directive order puts `x-bind` ahead of `x-on`, so `:src`
      // is assigned first -- today that is still safe, because a `load` is
      // dispatched in a later task and the walk binds the listener inside this
      // one. Sweeping what is already complete makes that independent of an
      // ordering this component does not control and cannot see change.
      this.$el.querySelectorAll('img[data-compare-image]').forEach((img) => this.noteSizeFrom(img));
    },

    destroy() {
      if (this._keyHandler) this.$el.removeEventListener('keydown', this._keyHandler);
      if (this._endDrag) this._endDrag();
      if (this._endAlignDrag) this._endAlignDrag();
      if (this._announceTimer) clearTimeout(this._announceTimer);
      if (this._blinkTimer !== null) {
        clearInterval(this._blinkTimer);
        this._blinkTimer = null;
      }
      if (this._heatMapFrame !== null) {
        if (typeof cancelAnimationFrame === 'function') cancelAnimationFrame(this._heatMapFrame);
        this._heatMapFrame = null;
      }
      this._heatMapAnnounceAfterRepaint = false;
      if (this._reducedMotionQuery && this._reducedMotionListener) {
        if (typeof this._reducedMotionQuery.removeEventListener === 'function') {
          this._reducedMotionQuery.removeEventListener('change', this._reducedMotionListener);
        }
        this._reducedMotionListener = null;
      }
    },

    nudgeSlider(delta) {
      this.sliderPos = Math.max(1, Math.min(99, this.sliderPos + delta));
    },

    swapSides() {
      this.swapped = !this.swapped;
      this._scheduleHeatMapRepaint(true);
    },

    toggleSide(e) {
      // Toggle mode's box is a real button, so an alignment drag across it ends
      // in a click that would also flip which version is showing.
      //
      // Bounded in time rather than by a flag the next click clears: a drag in
      // onion skin, a disarm, then a click here is a different click entirely,
      // and a sticky flag eats it. `detail > 0` separates a pointer click from
      // Enter or Space, which report 0 and cannot have followed a drag.
      if (e && e.detail > 0 && Date.now() - this._alignDragEndedAt < ALIGN_CLICK_SUPPRESS_MS) {
        // Spent on the one click it was for. Left standing, it would also eat a
        // second click arriving inside the same window.
        this._alignDragEndedAt = 0;
        return;
      }
      this.showLeft = !this.showLeft;
    },

    /**
     * WAI-ARIA radiogroup keyboard pattern.
     * ArrowRight / ArrowLeft cycle through `values`; Home / End jump to the
     * first / last. Selecting a value moves focus onto the now-checked radio
     * so tabindex stays on the active one (roving tabindex invariant).
     */
    onRadiogroupKeydown(e, stateKey, values, onChange) {
      if (!RADIOGROUP_KEYS.includes(e.key)) return;
      e.preventDefault();
      // Down and Up as well as Right and Left: the pattern specifies both pairs,
      // and which one a reader reaches for depends on how they read the control.
      const forward = e.key === 'ArrowRight' || e.key === 'ArrowDown';
      const back = e.key === 'ArrowLeft' || e.key === 'ArrowUp';
      const currentIdx = values.indexOf(this[stateKey]);
      let nextIdx = currentIdx;
      if (forward) nextIdx = (currentIdx + 1) % values.length;
      else if (back) nextIdx = (currentIdx - 1 + values.length) % values.length;
      else if (e.key === 'Home') nextIdx = 0;
      else if (e.key === 'End') nextIdx = values.length - 1;
      this[stateKey] = values[nextIdx];
      if (onChange) onChange();
      const group = e.currentTarget;
      this.$nextTick(() => {
        const checked = group.querySelector('[role="radio"][aria-checked="true"]');
        // Duck-typed for the same reason as `noteSizeFrom`: `HTMLElement` is a
        // per-realm identity, and what is required of this value is that it can
        // take focus.
        if (checked && typeof checked.focus === 'function') checked.focus();
      });
    },

    startSliderDrag(e) {
      e.preventDefault();
      this.isDragging = true;
      const container = this.$refs.sliderContainer;
      if (!container) return;

      // Prevent text selection during drag
      document.body.style.userSelect = 'none';
      document.body.style.cursor = 'ew-resize';

      const moveHandler = (moveE) => {
        if (!this.isDragging) return;
        moveE.preventDefault();
        const rect = container.getBoundingClientRect();
        const clientX = moveE.clientX ?? moveE.touches?.[0]?.clientX;
        if (clientX === undefined) return;
        this.sliderPos = Math.max(1, Math.min(99, ((clientX - rect.left) / rect.width) * 100));
      };

      // Kept on the component so `destroy()` can end a drag the page is leaving
      // in the middle of, which otherwise leaves the document listeners attached
      // and the whole page stuck with no text selection and a resize cursor.
      const upHandler = () => {
        this._endDrag = null;
        this.isDragging = false;
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        document.removeEventListener('mousemove', moveHandler);
        document.removeEventListener('mouseup', upHandler);
        document.removeEventListener('touchmove', moveHandler);
        document.removeEventListener('touchend', upHandler);
        // touchcancel too: the OS takes the gesture away on an incoming call or
        // a system swipe, and touchend never arrives.
        document.removeEventListener('touchcancel', upHandler);
        window.removeEventListener('blur', upHandler);
      };
      this._endDrag = upHandler;

      document.addEventListener('mousemove', moveHandler);
      document.addEventListener('mouseup', upHandler);
      document.addEventListener('touchmove', moveHandler, { passive: false });
      document.addEventListener('touchend', upHandler);
      document.addEventListener('touchcancel', upHandler);
      // A mouse released outside the window delivers no mouseup here, so a drag
      // interrupted by an alt-tab would otherwise leave the whole page
      // unselectable under a resize cursor until it was reloaded.
      window.addEventListener('blur', upHandler);
    }
  };
}
