// Image cropper Alpine component for the crop modal.
//
// Exposes drag-to-select on an <img> plus numeric X/Y/W/H inputs as the
// canonical keyboard-accessible path. Sends a form POST to
// /v1/resources/crop with rect coordinates in the image's natural pixels.

export function imageCropper({ resourceId, imageUrl, initialWidth = 0, initialHeight = 0 }) {
  return {
    resourceId,
    imageUrl,
    naturalW: initialWidth || 0,
    naturalH: initialHeight || 0,
    rect: { x: 0, y: 0, width: 0, height: 0 },
    aspect: 'free',
    comment: '',
    isSubmitting: false,
    errorMessage: '',
    _drag: null, // { startX, startY } in natural pixels

    onImageLoad() {
      const img = this.$refs.image;
      if (!img) return;
      this.naturalW = img.naturalWidth || this.naturalW;
      this.naturalH = img.naturalHeight || this.naturalH;
    },

    _imageRect() {
      return this.$refs.image ? this.$refs.image.getBoundingClientRect() : null;
    },

    _eventToNatural(event) {
      const rect = this._imageRect();
      if (!rect || rect.width <= 0 || rect.height <= 0 || !this.naturalW || !this.naturalH) {
        return null;
      }
      const px = Math.max(0, Math.min(rect.width, event.clientX - rect.left));
      const py = Math.max(0, Math.min(rect.height, event.clientY - rect.top));
      return {
        x: Math.round((px / rect.width) * this.naturalW),
        y: Math.round((py / rect.height) * this.naturalH),
      };
    },

    onPointerDown(event) {
      if (event.button !== undefined && event.button !== 0) return;
      const nat = this._eventToNatural(event);
      if (!nat) return;
      this.errorMessage = '';
      this._drag = { startX: nat.x, startY: nat.y };
      this.rect = { x: nat.x, y: nat.y, width: 0, height: 0 };
      if (event.target && event.target.setPointerCapture && event.pointerId !== undefined) {
        try { event.target.setPointerCapture(event.pointerId); } catch (_) { /* ignore */ }
      }
    },

    onPointerMove(event) {
      if (!this._drag) return;
      const nat = this._eventToNatural(event);
      if (!nat) return;
      const minX = Math.min(this._drag.startX, nat.x);
      const minY = Math.min(this._drag.startY, nat.y);
      const maxX = Math.max(this._drag.startX, nat.x);
      const maxY = Math.max(this._drag.startY, nat.y);
      let w = maxX - minX;
      let h = maxY - minY;
      const ratio = this._aspectRatio();
      if (ratio) {
        // Fit the larger of (w, h/ratio) as width, keeping the rect anchored at minX/minY
        const fromW = w;
        const fromH = h / ratio;
        const useW = Math.max(fromW, fromH);
        w = Math.round(useW);
        h = Math.round(useW * ratio);
      }
      this.rect = { x: minX, y: minY, width: w, height: h };
      this.clampRect();
    },

    onPointerUp(event) {
      if (!this._drag) return;
      this._drag = null;
      if (event && event.target && event.target.releasePointerCapture && event.pointerId !== undefined) {
        try { event.target.releasePointerCapture(event.pointerId); } catch (_) { /* ignore */ }
      }
    },

    _aspectRatio() {
      // Returns height / width for the current aspect selection, or null for free.
      switch (this.aspect) {
        case '1:1': return 1;
        case '16:9': return 9 / 16;
        case '4:3': return 3 / 4;
        case 'original':
          if (this.naturalW > 0 && this.naturalH > 0) return this.naturalH / this.naturalW;
          return null;
        default: return null;
      }
    },

    applyAspect() {
      const ratio = this._aspectRatio();
      if (!ratio || !this.hasSelection()) return;
      // Resize around the rect center, clamped to image bounds.
      const cx = this.rect.x + this.rect.width / 2;
      const cy = this.rect.y + this.rect.height / 2;
      const currentW = this.rect.width;
      const currentH = this.rect.height;
      const widthFromH = currentH / ratio;
      const useW = Math.max(currentW, widthFromH);
      let w = Math.round(useW);
      let h = Math.round(useW * ratio);
      let x = Math.round(cx - w / 2);
      let y = Math.round(cy - h / 2);
      this.rect = { x, y, width: w, height: h };
      this.clampRect();
    },

    clampRect() {
      if (!this.naturalW || !this.naturalH) return;
      let { x, y, width, height } = this.rect;
      x = Math.max(0, Math.min(this.naturalW - 1, Math.floor(x || 0)));
      y = Math.max(0, Math.min(this.naturalH - 1, Math.floor(y || 0)));
      width = Math.max(0, Math.floor(width || 0));
      height = Math.max(0, Math.floor(height || 0));

      const ratio = this._aspectRatio();
      if (ratio && width > 0 && height > 0) {
        // Aspect locked: trim both dimensions proportionally so the
        // submitted rect actually matches the preset. Without this, hitting
        // an edge clips one axis and breaks the ratio.
        const maxW = Math.max(0, this.naturalW - x);
        const maxHFromBounds = Math.max(0, this.naturalH - y);
        const maxWFromH = maxHFromBounds / ratio;
        width = Math.floor(Math.min(width, maxW, maxWFromH));
        height = Math.floor(width * ratio);
      } else {
        if (x + width > this.naturalW) width = this.naturalW - x;
        if (y + height > this.naturalH) height = this.naturalH - y;
      }
      this.rect = { x, y, width, height };
    },

    hasSelection() {
      return this.rect.width > 0 && this.rect.height > 0;
    },

    selectionStyle() {
      const rect = this._imageRect();
      if (!rect || !this.naturalW || !this.naturalH) return 'display: none';
      const scaleX = rect.width / this.naturalW;
      const scaleY = rect.height / this.naturalH;
      const left = this.rect.x * scaleX;
      const top = this.rect.y * scaleY;
      const width = this.rect.width * scaleX;
      const height = this.rect.height * scaleY;
      return `left: ${left}px; top: ${top}px; width: ${width}px; height: ${height}px; outline: 2px dashed #fff; box-shadow: 0 0 0 2px rgba(0,0,0,0.6); background: rgba(255,255,255,0.1);`;
    },

    async submit() {
      if (this.isSubmitting) return;
      if (!this.hasSelection()) {
        this.errorMessage = 'Select a crop area first.';
        return;
      }
      this.errorMessage = '';
      this.isSubmitting = true;
      try {
        const body = new URLSearchParams();
        body.set('id', String(this.resourceId));
        body.set('x', String(this.rect.x));
        body.set('y', String(this.rect.y));
        body.set('width', String(this.rect.width));
        body.set('height', String(this.rect.height));
        if (this.comment && this.comment.trim()) body.set('comment', this.comment.trim());

        const response = await fetch('/v1/resources/crop', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Accept': 'application/json',
          },
          body: body.toString(),
        });

        if (!response.ok) {
          let message = `Crop failed (HTTP ${response.status})`;
          try {
            const text = await response.text();
            if (text) message = text;
          } catch (_) { /* ignore */ }
          this.errorMessage = message;
          this.isSubmitting = false;
          return;
        }

        window.location.reload();
      } catch (err) {
        this.errorMessage = err && err.message ? err.message : 'Crop failed.';
        this.isSubmitting = false;
      }
    },

    close() {
      const dialog = this.$root;
      if (dialog && typeof dialog.close === 'function') {
        dialog.close();
      }
    },

    reset() {
      this.rect = { x: 0, y: 0, width: 0, height: 0 };
      this.aspect = 'free';
      this.comment = '';
      this.errorMessage = '';
      this.isSubmitting = false;
      this._drag = null;
    },
  };
}
