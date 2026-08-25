export function imageCompare({ leftUrl, rightUrl, leftLabel, rightLabel, leftSize, rightSize }) {
  return {
    mode: 'side-by-side',
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

    /** Per-image size inside the shared overlay box, as a percentage pair. */
    overlayScale(index) {
      const [a, b] = this._sizes;
      if (!a || !b || !a.w || !a.h || !b.w || !b.h) return '';
      const box = { w: Math.max(a.w, b.w), h: Math.max(a.h, b.h) };
      const img = index === 0 ? a : b;
      return `width:${(img.w / box.w) * 100}%;height:${(img.h / box.h) * 100}%;`;
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
        const target = e.target;
        if (target instanceof HTMLElement) {
          const tag = target.tagName;
          if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
          if (target.closest('[role="radiogroup"]')) return;
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
        if (checked instanceof HTMLElement) checked.focus();
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
      };
      this._endDrag = upHandler;

      document.addEventListener('mousemove', moveHandler);
      document.addEventListener('mouseup', upHandler);
      document.addEventListener('touchmove', moveHandler, { passive: false });
      document.addEventListener('touchend', upHandler);
      document.addEventListener('touchcancel', upHandler);
    }
  };
}
