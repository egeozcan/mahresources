// The keys the WAI-ARIA radiogroup pattern owns. Named once because two places
// need the same set: the handler that acts on them, and the refusal that must
// still swallow them.
const RADIOGROUP_KEYS = ['ArrowRight', 'ArrowLeft', 'ArrowUp', 'ArrowDown', 'Home', 'End'];

// Manual alignment bounds and steps, in box pixels and scale factors.
//
// The translation is clamped to half the box in each axis for the reason
// `nudgeSlider` clamps to 1-99 rather than 0-100: a state that shows nothing at
// all reads as a broken page rather than as a choice the reader made. The bound
// is applied where the reader acts, not on the derived inverse a flip produces
// -- the inverse of an extreme correction is legitimately extreme, and clamping
// it there would silently change an alignment instead of preserving it.
const ALIGN_TRANSLATE_LIMIT = 0.5;
const ALIGN_ZOOM_MIN = 0.25;
const ALIGN_ZOOM_MAX = 4;
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

/** `+12` / `-4` / `0` -- the sign is information, and a bare `12` hides it. */
function signed(n) {
  const r = Math.round(n);
  return r > 0 ? `+${r}` : `${r}`;
}

export function imageCompare({ leftUrl, rightUrl, leftLabel, rightLabel, leftSize, rightSize }) {
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
    _alignDragMoved: false,
    _endAlignDrag: null,
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
      const [a, b] = this._sizes;
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) return null;
      const width = Math.max(a.w, b.w);
      const height = Math.max(a.h, b.h);
      return `${width} / ${height}`;
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
      const [a, b] = this._sizes;
      // Nothing to measure into. The `[style*="aspect-ratio"]` branch in
      // index.css owns the layout here and puts the images back in the flow, so
      // an inline `height: 100%` from the other branches below would beat its
      // `height: auto` and break a case that currently works.
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) {
        return { width: '', height: '', objectFit: '', margin: '', transform: '', transformOrigin: '' };
      }

      // Distort each image onto the whole box. Right for a re-encode that
      // changed aspect, wrong for a crop, which is why this mode's accessible
      // name says so rather than leaving the reader to notice.
      const box = { w: Math.max(a.w, b.w), h: Math.max(a.h, b.h) };

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
      const img = index === 0 ? a : b;
      let { w, h } = img;

      if (this.scale === 'fit') {
        // Grow each image until an edge touches the box. Two versions of one
        // aspect ratio both end up filling a box built from the larger, so a
        // pure resolution change registers exactly.
        //
        // Sized on the *element* rather than left to `object-fit: contain` on a
        // full-box element, which would paint identically. Two reasons, and
        // neither is arithmetic for its own sake: a contained letterbox is
        // invisible to `getBoundingClientRect`, so Fit and Stretch would render
        // to indistinguishable geometry and nothing outside a screenshot could
        // tell them apart; and with the element equal to the painted rectangle,
        // anchoring is `margin` here exactly as it is under relative scale,
        // instead of `margin` in one mode and `object-position` in the other.
        const k = Math.min(box.w / img.w, box.h / img.h);
        w = img.w * k;
        h = img.h * k;
      }

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

    /** `+12, -4, 103%`, reporting the clamped values actually in effect. */
    get offsetLabel() {
      const { dx, dy, k } = this.trailOffset;
      return `${signed(dx)}, ${signed(dy)}, ${Math.round(k * 100)}%`;
    },

    toggleAligning() {
      if (!this.alignAvailable) return;
      this.aligning = !this.aligning;
    },

    /**
     * Move the trailing image by a whole number of box pixels.
     *
     * Does not announce: the drag calls this many times a second and the
     * announcement is made once, at the end. Keyboard callers announce for
     * themselves.
     */
    nudge(dx, dy) {
      if (!this.alignAvailable) return;
      const [a, b] = this._sizes;
      const limitX = Math.max(a.w, b.w) * ALIGN_TRANSLATE_LIMIT;
      const limitY = Math.max(a.h, b.h) * ALIGN_TRANSLATE_LIMIT;
      const t = this.trailOffset;
      this._setTrailOffset({
        dx: Math.max(-limitX, Math.min(limitX, t.dx + dx)),
        dy: Math.max(-limitY, Math.min(limitY, t.dy + dy)),
        k: t.k,
      });
    },

    zoomBy(delta) {
      if (!this.alignAvailable) return;
      const t = this.trailOffset;
      this._setTrailOffset({
        dx: t.dx,
        dy: t.dy,
        k: Math.max(ALIGN_ZOOM_MIN, Math.min(ALIGN_ZOOM_MAX, t.k + delta)),
      });
    },

    /**
     * Clear the translation and the zoom together.
     *
     * Both halves, because a reset that left a 103% zoom behind would be the
     * same invisible state in a smaller box. The arming survives: the reason
     * you reset is almost always that you are about to try again.
     */
    resetAlignment() {
      // A refused Reset stays silent. The control is drawn `aria-disabled` at
      // identity rather than removed, so it is reachable and pressable, and
      // announcing "offset 0, 0, 100%" at someone who changed nothing is the
      // live-region equivalent of a control that looks like it acted.
      if (this.offsetIsIdentity) return;
      this._offset = { dx: 0, dy: 0, k: 1 };
      this.announceOffset();
    },

    announceOffset() {
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
      // The slider handle is inside the box and its own `mousedown` bubbles to
      // here, so without this an armed drag on the handle would move the reveal
      // position and the trailing image at the same time.
      const from = e.target;
      if (from && typeof from.closest === 'function' && from.closest('.compare-slider-handle')) return;
      const box = e.currentTarget;
      if (!box || typeof box.getBoundingClientRect !== 'function') return;
      const rect = box.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      e.preventDefault();
      // A fresh gesture: whatever the last one did to the toggle-mode click
      // guard is spent.
      this._alignDragMoved = false;

      const [a, b] = this._sizes;
      const perPixelX = Math.max(a.w, b.w) / rect.width;
      const perPixelY = Math.max(a.h, b.h) / rect.height;
      const point = (ev) => ({
        x: ev.clientX ?? ev.touches?.[0]?.clientX,
        y: ev.clientY ?? ev.touches?.[0]?.clientY,
      });
      let last = point(e);
      if (last.x === undefined || last.y === undefined) return;

      const moveHandler = (moveE) => {
        const next = point(moveE);
        if (next.x === undefined || next.y === undefined) return;
        moveE.preventDefault();
        this._alignDragMoved = true;
        this.nudge((next.x - last.x) * perPixelX, (next.y - last.y) * perPixelY);
        last = next;
      };

      const upHandler = () => {
        this._endAlignDrag = null;
        document.removeEventListener('mousemove', moveHandler);
        document.removeEventListener('mouseup', upHandler);
        document.removeEventListener('touchmove', moveHandler);
        document.removeEventListener('touchend', upHandler);
        document.removeEventListener('touchcancel', upHandler);
        window.removeEventListener('blur', upHandler);
        document.body.style.userSelect = '';
        // One announcement for the whole gesture. Announcing per `pointermove`
        // would queue hundreds of them.
        if (this._alignDragMoved) this.announceOffset();
      };
      this._endAlignDrag = upHandler;

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
      e.preventDefault();
      this.zoomBy(e.deltaY < 0 ? ALIGN_ZOOM_STEP : -ALIGN_ZOOM_STEP);
      this.announceOffset();
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
      this.scale = value;
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
      this.anchor = this.anchor === 'top-left' ? 'center' : 'top-left';
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
      this.onRadiogroupKeydown(e, 'scale', ['relative', 'fit', 'stretch']);
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
      if (next) this._sizes = next;
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
        // Anything focusable answers its own arrow keys. Without this the
        // reveal position moved while Flip had focus, which is a control that
        // has nothing to do with it.
        const target = e.target;
        if (target instanceof HTMLElement
          && target.closest('button, a[href], input, select, textarea, [role="radiogroup"], [tabindex]')) {
          return;
        }
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
    },

    nudgeSlider(delta) {
      this.sliderPos = Math.max(1, Math.min(99, this.sliderPos + delta));
    },

    swapSides() {
      this.swapped = !this.swapped;
    },

    toggleSide(e) {
      // Toggle mode's box is a real button, so an alignment drag across it ends
      // in a click that would also flip which version is showing. `detail > 0`
      // is what separates a pointer click from Enter or Space, which report 0
      // and cannot have been preceded by a drag.
      if (this._alignDragMoved && e && e.detail > 0) {
        this._alignDragMoved = false;
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
    onRadiogroupKeydown(e, stateKey, values) {
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
