export function imageCompare({ leftUrl, rightUrl }) {
  return {
    mode: 'side-by-side',
    leftUrl,
    rightUrl,
    sliderPos: 50,
    opacity: 50,
    showLeft: true,
    isDragging: false,

    swapSides() {
      const temp = this.leftUrl;
      this.leftUrl = this.rightUrl;
      this.rightUrl = temp;
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
