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

// The zero-width space that makes a repeated announcement a *changed* string.
// `aria-live` fires on text change, so nudging away from a value and back would
// otherwise land on the text already in the region and announce nothing. It is
// alternated rather than accumulated, which is all the property requires: two
// consecutive announcements always differ. Screen readers do not voice it.
const ANNOUNCE_MARK = '\u200B';

/** The CSS transform that undoes `t`: for T(p) = k*p + d, T-inverse(q) = (q - d) / k. */
function invertOffset(t) {
  return { dx: -t.dx / t.k, dy: -t.dy / t.k, k: 1 / t.k };
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

export function imageCompare({ leftUrl, rightUrl, leftLabel, rightLabel, leftSize, rightSize }) {
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
    // A size correction moved an offset that was inside the old bound, or a
    // control operation (zoom, scale, anchor) moved the bound while the pair
    // was half-measured, but the rebound has to wait for the other version to
    // report. Owed rather than immediate because the report that completes
    // the pair may only *confirm* a stored size -- see `noteSizeFrom`. The
    // orientation at arming is kept so the pay can complete the correction in
    // the orientation that incurred it, whatever a flip did in between.
    _sizeReboundDue: false,
    _sizeReboundDueSwapped: false,
    // Whether the outstanding debt was incurred by something that *moved the
    // bound* -- a size report, or a control operation the load window
    // deferred -- as opposed to by a nudge. `_sizeReboundDue` cannot answer
    // that: a nudge sets it too, because it is the resolution trigger and not
    // only the bound-repair debt. The difference decides provenance. A
    // display sitting outside its bound is legitimate when a flip derived it
    // and *owed* when a bound-mover put it there, and only the legitimate one
    // may be preserved through the deferred resolution.
    //
    // Armed exactly where an operation took an axis from inside its bound to
    // outside it -- see `_noteSizeOwed` -- which is one statement doing two
    // jobs, because they are the same statement: the displayed rebound may
    // repair an axis exactly when that axis's outside-ness is owed, and a
    // window nudge may anchor one exactly when it is not. Anything looser claims outside-ness
    // the operation did not create and then suppresses the anchor of one a
    // flip legitimately derives afterwards, which is how the same defect
    // arrived three times: armed while nothing was displaced at all, armed on
    // the axis a height-only report never touched, and armed on an axis a
    // zoom left inside. One flag per axis because that is the grain the
    // question has; the axes do not transpose under a flip (`invertOffset`
    // maps dx to dx and dy to dy), so no orientation is recorded with them.
    _sizeOwedX: false,
    _sizeOwedY: false,
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
    // The stored box's width, in its own pixels -- the frame both decode
    // orders share before either image has spoken. Null when the server had no
    // dimensions for the pair: then the box itself does not exist until one
    // version has reported, only that version's report can open a nudgeable
    // window, and there is no second order to diverge from.
    _storedBoxW: storedBoxW,
    // The width the outstanding request is denominated in, captured once when
    // that request is created: the stored box's, or -- when the server had no
    // dimensions -- the box standing at creation. A missing stored box used to
    // fall the whole conversion back to 1, which is not "no stored frame" but
    // "no conversion at all": the displayed offset scaled with the corrected
    // box and the request did not, so the reader watched their correction
    // shrink when the second version arrived. Null while no request is
    // outstanding.
    _requestUnitsW: null,
    _endAlignDrag: null,
    // Removes an active align drag's *move* listeners only, leaving the
    // enders: `toggleAligning` calls it on a mid-gesture disarm.
    _disarmAlignDrag: null,
    sliderPos: 50,
    opacity: 50,
    showLeft: true,
    isDragging: false,
    _urls: [leftUrl, rightUrl],
    _labels: [leftLabel || '', rightLabel || ''],
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
      // `invertOffset` is an involution, so writing back through it is the same
      // operation as reading through it.
      this._offset = this.swapped ? invertOffset(t) : t;
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
     * repair -- and so whether it may arm a debt. Zero displacement is inside
     * every range `translateRange` can produce: `min` is `-(rest + rendered -
     * keep)`, which is never above zero, and `max` is `box - keep - rest`,
     * which is never below it, at any anchor, scale or zoom (the rendered
     * length is at most four times the box, and `keep` is a quarter of it).
     *
     * Distinct from `offsetIsIdentity`, which is also false at a bare zoom --
     * and a bare zoom displaces nothing either. Arming there costs more than a
     * wasted debt: it records that whatever the display shows is a
     * bound-mover's doing, so a nudge taken later reads a legitimately-outside
     * axis -- a flip's inverse -- as owed and lets the resolution snap it.
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
        // An increment that met the clamp contributes its full travel -- that
        // is the part still owed; one that moved freely contributes only the
        // distance the display actually travelled, so walking the image back
        // retires what running it out incurred.
        const rode = physical ? intoUnits : 1;
        // Where the display legitimately sits outside its bound, recorded in
        // the same frame and shape as the translation so a flip carries it.
        // Snapshotted here rather than read at resolution because that is
        // where the provenance is still known: outside-ness the reader is
        // looking at *now*, with no bound-mover's debt outstanding, is a
        // flip-derived inverse and must survive; outside-ness the completing
        // report is about to create is the debt itself.
        const anchor = (value, range, owed) => (owed
          || (value >= range.min && value <= range.max))
          ? null
          : value * intoUnits;
        this._setRawRequest({
          dx: request.dx * (fresh ? intoUnits : 1)
            + (next.dx === t.dx + dx ? next.dx - t.dx : dx) * rode,
          dy: request.dy * (fresh ? intoUnits : 1)
            + (next.dy === t.dy + dy ? next.dy - t.dy : dy) * rode,
          k: request.k,
          ax: anchor(next.dx, x, this._sizeOwedX),
          ay: anchor(next.dy, y, this._sizeOwedY),
        });
        this._sizeReboundDue = true;
        this._sizeReboundDueSwapped = this.swapped;
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
     * Rebound now, unless the pair is half-measured.
     *
     * The load window -- exactly one version measured -- is a geometry that
     * does not exist: the box is `max` of a real measurement and a stored
     * placeholder, and the trail is whichever slot decoded first. Clamping
     * against it makes the final offset depend on decode order, so the clamp
     * is owed at the both-measured moment instead, where the load-time
     * rebound is paid.
     */
    _reboundOrDefer(before) {
      // A bound moved underneath a version that is not displaced broke
      // nothing: zero is inside every range `translateRange` can produce, so
      // the per-axis test below would refuse this too. It is here as the fast
      // path -- the wheel calls this once per notch, and the test below is a
      // full pass of the range arithmetic -- and as the statement of why an
      // undisplaced correction can never owe anything.
      if (!this.offsetIsPlaced) return;
      this._noteSizeOwed(before);
      // Asked per axis, because the answer differs per axis: a flip can leave
      // x legitimately outside while y sits inside, and an anchor change then
      // makes x legal and throws y clear of the frame. Asked of the offset as
      // a whole -- "was it inside before?" -- x answers for y, the repair never
      // runs, and the reader is left looking at nothing at all, which is the
      // state the bound exists to prevent.
      if (!this._sizeOwedX && !this._sizeOwedY) return;
      if (this._measured[0] === this._measured[1]) {
        this._reboundOffset();
        // Consumed here: with both versions measured there is no later
        // resolution to clear them, and a claim left standing would let the
        // next operation repair an axis it did not break.
        this._sizeOwedX = false;
        this._sizeOwedY = false;
      } else {
        this._sizeReboundDue = true;
        this._sizeReboundDueSwapped = this.swapped;
      }
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
      return {
        x: t.dx >= x.min && t.dx <= x.max,
        y: t.dy >= y.min && t.dy <= y.max,
      };
    },

    /**
     * Record which axes an operation just pushed out of bounds.
     *
     * `before` is the per-axis snapshot taken before that operation ran. An
     * axis that was inside and is now outside was put there by the operation,
     * so whatever a later nudge finds there is owed rather than a flip's
     * doing. An axis still inside owes nothing, and one already outside was
     * already outside -- claiming either would suppress the anchor of an
     * outside a flip legitimately derives later, and resolve that axis out of
     * a position the reader made.
     *
     * Called after the operation, so the ranges it reads are the new ones.
     */
    _noteSizeOwed(before) {
      const now = this.offsetAxesWithinBound;
      if (before.x && !now.x) this._sizeOwedX = true;
      if (before.y && !now.y) this._sizeOwedY = true;
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
    _reboundOffset(request) {
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
      let dx, dy;
      if (request === undefined) {
        // Only where the outside-ness is owed. An axis still inside clamps to
        // itself, and one that was outside before the operation ran is a
        // flip's doing, not this operation's -- pulling it in is the rewrite
        // of the reader's own correction that `offsetWithinBound` has always
        // existed to refuse, asked at the grain the question has.
        dx = this._sizeOwedX ? Math.max(x.min, Math.min(x.max, t.dx)) : t.dx;
        dy = this._sizeOwedY ? Math.max(y.min, Math.min(y.max, t.dy)) : t.dy;
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
        dx = axis(t.dx, request.ax === null ? null : request.ax * outOfUnits, x);
        dy = axis(t.dy, request.ay === null ? null : request.ay * outOfUnits, y);
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
    _resolveDeferred() {
      const request = this._requestedOffset === null ? undefined : this._rawRequest();
      this._reboundOffset(request);
      this._requestedOffset = null;
      this._requestUnitsW = null;
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
      this._sizeReboundDue = false;
      this._sizeReboundDueSwapped = false;
      this._sizeOwedX = false;
      this._sizeOwedY = false;
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
        if (moveE.touches && moveE.touches.length > 1) return;
        const next = point(moveE);
        if (next.x === undefined || next.y === undefined) return;
        anyMove = true;
        moveE.preventDefault();
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
        // ratios -- `noteSizeFrom` made the same mistake and fixed it the same
        // way.
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
     * On activation, not on one input path: a first attempt guarded `mousedown`
     * alone, which leaves the hole open for programmatic focus, for an assistive
     * technology moving focus directly, and for any touch implementation that
     * focuses without a compatibility `mousedown`. Enter and Space on such a
     * radio produce a refused click and leave focus exactly where it should not
     * be. Every one of those paths ends in the click handler, so that is where
     * the invariant is restored.
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
        () => { if (this.scale !== previous) this._reboundOrDefer(within); });
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
      // Snapshotted before this call's own correction, so that what the
      // report broke can be told from what was already true: an axis a flip
      // legitimately left outside was outside before this report too.
      const withinAxes = this.offsetAxesWithinBound;
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
      // The rebound is **owed** rather than tied to the size change itself,
      // because the report that completes the pair may be one that only
      // *confirms* a stored size: it flips `_measured` without touching
      // `_sizes`, and it still has to pay the debt an earlier report set up.
      const both = this._measured[0] && this._measured[1];
      // Per axis: a report that pushed an axis out of a bound it was inside
      // owns that axis's position, so a nudge taken after it must not read the
      // outside as a flip's doing, and the rebound may repair exactly that
      // axis. A report that only confirms, that moved an axis still inside, or
      // that found the axis already outside claims nothing. Run for the
      // completing report too -- one image can fill both slots, and then this
      // call is also the size change whose repair the pay below performs.
      if (next) this._noteSizeOwed(withinAxes);
      // A confirm can produce no claim of its own -- it changes no geometry,
      // so no axis crosses -- which is what keeps a flip made after it from
      // being clamped by the next confirm, while the debt an earlier report
      // armed still gets paid.
      if (!this._sizeReboundDue && !both && (this._sizeOwedX || this._sizeOwedY)) {
        this._sizeReboundDue = true;
        this._sizeReboundDueSwapped = this.swapped;
      }
      // Paid when both have reported. One image showing on both sides fills
      // both slots in a single report, so the completing call is also the size
      // change and must rebound directly rather than only through the debt.
      if (both && (this._sizeReboundDue || this._sizeOwedX || this._sizeOwedY)) {
        // The debt was incurred by a size change in a specific orientation.
        // If a flip landed between the change and the confirming load, the
        // pay completes the correction in that same orientation -- paying in
        // the flipped one clamps the inverse and rewrites the canonical
        // offset differently than the same measurements without the flip
        // would have, which is the order-dependence the load-time rebound
        // exists to remove. The flip itself never reapplies the bound.
        if (this._sizeReboundDue && this.swapped !== this._sizeReboundDueSwapped) {
          const held = this.swapped;
          this.swapped = this._sizeReboundDueSwapped;
          this._resolveDeferred();
          this.swapped = held;
        } else {
          this._resolveDeferred();
        }
        this._sizeReboundDue = false;
        this._sizeOwedX = false;
        this._sizeOwedY = false;
      }
    },

    get leadScale() {
      return this.overlayScale(this.swapped ? 1 : 0);
    },
    get trailScale() {
      return this.overlayScale(this.swapped ? 0 : 1);
    },

    init() {
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
    },

    nudgeSlider(delta) {
      this.sliderPos = Math.max(1, Math.min(99, this.sliderPos + delta));
    },

    swapSides() {
      this.swapped = !this.swapped;
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
