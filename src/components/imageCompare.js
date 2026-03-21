export function imageCompare({ leftUrl, rightUrl, leftLabel, rightLabel }) {
  return {
    mode: 'side-by-side',
    leftUrl,
    rightUrl,
    leftLabel: leftLabel || '',
    rightLabel: rightLabel || '',
    sliderPos: 50,
    opacity: 50,
    showLeft: true,
    isDragging: false,
    _keyHandler: null,

    init() {
      this._keyHandler = (e) => {
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') return;
        const step = e.shiftKey ? 10 : 2;
        if (this.mode === 'slider') {
          if (e.key === 'ArrowLeft') { this.sliderPos = Math.max(1, this.sliderPos - step); e.preventDefault(); }
          else if (e.key === 'ArrowRight') { this.sliderPos = Math.min(99, this.sliderPos + step); e.preventDefault(); }
        } else if (this.mode === 'onion') {
          if (e.key === 'ArrowLeft') { this.opacity = Math.max(0, this.opacity - step); e.preventDefault(); }
          else if (e.key === 'ArrowRight') { this.opacity = Math.min(100, this.opacity + step); e.preventDefault(); }
        }
      };
      document.addEventListener('keydown', this._keyHandler);
    },

    destroy() {
      if (this._keyHandler) document.removeEventListener('keydown', this._keyHandler);
    },

    swapSides() {
      [this.leftUrl, this.rightUrl] = [this.rightUrl, this.leftUrl];
      [this.leftLabel, this.rightLabel] = [this.rightLabel, this.leftLabel];
    },

    toggleSide() {
      this.showLeft = !this.showLeft;
    },

    startSliderDrag(e) {
      e.preventDefault();
      this.isDragging = true;
      const container = e.target.closest('.relative');

      // Prevent text selection during drag
      document.body.style.userSelect = 'none';
      document.body.style.cursor = 'ew-resize';

      const moveHandler = (moveE) => {
        if (!this.isDragging) return;
        moveE.preventDefault();
        const rect = container.getBoundingClientRect();
        const x = (moveE.clientX || moveE.touches?.[0]?.clientX) - rect.left;
        this.sliderPos = Math.max(1, Math.min(99, (x / rect.width) * 100));
      };

      const upHandler = () => {
        this.isDragging = false;
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        document.removeEventListener('mousemove', moveHandler);
        document.removeEventListener('mouseup', upHandler);
        document.removeEventListener('touchmove', moveHandler);
        document.removeEventListener('touchend', upHandler);
      };

      document.addEventListener('mousemove', moveHandler);
      document.addEventListener('mouseup', upHandler);
      document.addEventListener('touchmove', moveHandler, { passive: false });
      document.addEventListener('touchend', upHandler);
    }
  };
}
