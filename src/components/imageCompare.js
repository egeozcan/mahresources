export function imageCompare({ leftUrl, rightUrl, leftLabel, rightLabel, leftSize, rightSize }) {
  return {
    mode: 'side-by-side',
    // How the two images are measured into the shared box. `relative` is true
    // relative scale and was the only policy the page ever had; it is still the
    // default, because a pair with identical dimensions renders the same under
    // all three and the divergent minority has no measurement saying which of a
    // rescan and a crop is more common.
    scale: 'relative',
    // `swapped` is the only piece of swap state. Exchanging the URLs and labels
    // in place left the panel colours and the server-rendered alt text describing
    // whichever side had originally been there, so the red "older" panel could
    // hold the newer file and a screen reader was told the opposite of the truth.
    // Everything that varies is derived from this flag instead.
    swapped: false,
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
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) return { width: '', height: '', objectFit: '' };

      // Distort each image onto the whole box. Right for a re-encode that
      // changed aspect, wrong for a crop, which is why this mode's accessible
      // name says so rather than leaving the reader to notice.
      if (this.scale === 'stretch') {
        return { width: '100%', height: '100%', objectFit: 'fill' };
      }

      const box = { w: Math.max(a.w, b.w), h: Math.max(a.h, b.h) };
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
      };
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

    setScale(value) {
      if (!this.scaleAvailable) return;
      this.scale = value;
    },

    /**
     * The refusal has to cover the keyboard too.
     *
     * `onRadiogroupKeydown` assigns straight into state, so arrow keys, Home and
     * End would move a control that announces itself unavailable -- a guard on
     * `@click` alone leaves the group working for anyone not using a mouse.
     */
    onScaleKeydown(e) {
      if (!this.scaleAvailable) return;
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
     * The `_sizes` / `_urls` slot a side is currently showing.
     *
     * Those arrays are indexed by the server's left/right, while `leadUrl` is
     * `_urls[swapped ? 1 : 0]`. Recording a measured size against the *side* it
     * came from rather than the slot it belongs to transposes the whole box on
     * every flip, which looks like a rendering bug and is a bookkeeping one.
     */
    slotFor(side) {
      const swapped = this.swapped ? 1 : 0;
      return side === 'lead' ? swapped : 1 - swapped;
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
     * from resizing under the reader between markup and load.
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
      const index = this.slotFor(img.dataset?.compareSide === 'lead' ? 'lead' : 'trail');
      const known = this._sizes[index];
      if (known && known.w === w && known.h === h) return;
      // A new array rather than an index write: the getters derived from this
      // have to re-run, and eight images report into two slots, so this runs
      // far more often than it changes anything.
      const next = this._sizes.slice();
      next[index] = { w, h };
      this._sizes = next;
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
      this.$el.querySelectorAll('img[data-compare-side]').forEach((img) => this.noteSizeFrom(img));
    },

    destroy() {
      if (this._keyHandler) this.$el.removeEventListener('keydown', this._keyHandler);
      if (this._endDrag) this._endDrag();
    },

    nudgeSlider(delta) {
      this.sliderPos = Math.max(1, Math.min(99, this.sliderPos + delta));
    },

    swapSides() {
      this.swapped = !this.swapped;
    },

    toggleSide() {
      this.showLeft = !this.showLeft;
    },

    /**
     * WAI-ARIA radiogroup keyboard pattern.
     * ArrowRight / ArrowLeft cycle through `values`; Home / End jump to the
     * first / last. Selecting a value moves focus onto the now-checked radio
     * so tabindex stays on the active one (roving tabindex invariant).
     */
    onRadiogroupKeydown(e, stateKey, values) {
      // Down and Up as well as Right and Left: the pattern specifies both pairs,
      // and which one a reader reaches for depends on how they read the control.
      const forward = e.key === 'ArrowRight' || e.key === 'ArrowDown';
      const back = e.key === 'ArrowLeft' || e.key === 'ArrowUp';
      if (!forward && !back && e.key !== 'Home' && e.key !== 'End') {
        return;
      }
      e.preventDefault();
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
