// Image cropper Alpine component for the crop modal.
//
// Exposes drag-to-select on an <img> plus numeric X/Y/W/H inputs as the
// canonical keyboard-accessible path. Sends a form POST to
// /v1/resources/crop with rect coordinates in the image's natural pixels.
//
// Two outcomes, chosen by `saveMode`: 'version' rewrites the resource in place
// as a new version (the default, and what crop has always done), 'resource'
// saves the crop as a separate resource and leaves the source alone.

export function imageCropper({ resourceId, imageUrl, initialWidth = 0, initialHeight = 0, onSuccess = null, onNewResource = null }) {
  return {
    resourceId,
    imageUrl,
    // Optional success callback for the version path. When provided (e.g. the
    // lightbox), it owns the post-crop refresh; otherwise we fall back to a full
    // page reload (the server-rendered details-page modal relies on this).
    onSuccess,
    // Optional notification for the new-resource path. The cropper stays open in
    // that mode, so this is not a teardown hook — the host uses it to react
    // (announce, mark the gallery behind it stale).
    onNewResource,
    // 'version' | 'resource'. Anything the server has always accepted keeps
    // versioning, so this defaults to the historical behaviour.
    saveMode: 'version',
    // Set after a successful save in 'resource' mode: the cropper stays open so
    // several regions can be lifted out of one source in a row.
    newResourceId: 0,
    successMessage: '',
    naturalW: initialWidth || 0,
    naturalH: initialHeight || 0,
    rect: { x: 0, y: 0, width: 0, height: 0 },
    aspect: 'free',
    comment: '',
    isSubmitting: false,
    errorMessage: '',
    // BH-008: decode-failed signal. Set true when the <img> element fails
    // to load OR loads with naturalWidth/naturalHeight === 0 (e.g. SVG and
    // other formats Go's image decoder can't size server-side, which get
    // stored with Width=0/Height=0 post-BH-039). When true the crop
    // overlay is hidden, the Crop button disabled, and an explanatory
    // banner rendered so users don't submit nonsense rects.
    decodeFailed: false,
    _drag: null, // { startX, startY } in natural pixels
    // Bumped by reset(); a submit that resolves after a reset is discarded.
    _generation: 0,

    onImageLoad() {
      const img = this.$refs.image;
      if (!img) return;
      if (!img.naturalWidth || !img.naturalHeight) {
        this.decodeFailed = true;
        return;
      }
      this.decodeFailed = false;
      this.naturalW = img.naturalWidth || this.naturalW;
      this.naturalH = img.naturalHeight || this.naturalH;
    },

    onImageError() {
      this.decodeFailed = true;
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
      this.successMessage = '';
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

    // clampRect keeps the rect inside the image and, when an aspect preset is
    // active, enforces the ratio. `driver` names the axis the caller just
    // changed ('w' or 'h' from the numeric inputs, 'auto' for drag/preset
    // changes). Height-driven edits derive width from height, and vice versa,
    // so both numeric inputs remain usable under a locked aspect.
    clampRect(driver = 'auto') {
      if (!this.naturalW || !this.naturalH) return;
      let { x, y, width, height } = this.rect;
      x = Math.max(0, Math.min(this.naturalW - 1, Math.floor(x || 0)));
      y = Math.max(0, Math.min(this.naturalH - 1, Math.floor(y || 0)));
      width = Math.max(0, Math.floor(width || 0));
      height = Math.max(0, Math.floor(height || 0));

      const ratio = this._aspectRatio();
      if (ratio && width > 0 && height > 0) {
        const maxW = Math.max(0, this.naturalW - x);
        const maxH = Math.max(0, this.naturalH - y);
        if (driver === 'h') {
          // Height is authoritative — derive width from it.
          const maxHFromW = maxW * ratio;
          height = Math.floor(Math.min(height, maxH, maxHFromW));
          width = Math.floor(height / ratio);
        } else {
          // Width is authoritative (drag, preset apply, 'w' input).
          const maxWFromH = maxH / ratio;
          width = Math.floor(Math.min(width, maxW, maxWFromH));
          height = Math.floor(width * ratio);
        }
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

    savesAsNewResource() {
      return this.saveMode === 'resource';
    },

    submitLabel() {
      return this.savesAsNewResource() ? 'Save as new resource' : 'Crop';
    },

    newResourceUrl() {
      return this.newResourceId ? `/resource?id=${this.newResourceId}` : '';
    },

    async submit() {
      if (this.isSubmitting) return;
      // BH-008: never submit when the image can't be decoded client-side.
      if (this.decodeFailed || !this.naturalW || !this.naturalH) return;
      if (!this.hasSelection()) {
        this.errorMessage = 'Select a crop area first.';
        return;
      }
      this.errorMessage = '';
      this.successMessage = '';
      this.isSubmitting = true;
      // Captured before the await: the radio is live while the request is out,
      // and the response must be handled as the mode it was actually sent in.
      const asNewResource = this.savesAsNewResource();
      // The dialog can be closed mid-request, and closing resets the component
      // without tearing it down — the same instance is reused when it reopens.
      // Without this fence the earlier request's response lands in the new
      // interaction: it would clear a rect the user had just entered and show a
      // banner for a crop they had walked away from. reset() bumps the counter,
      // so a stale response finds it changed and does nothing.
      const generation = this._generation;
      try {
        const body = new URLSearchParams();
        body.set('id', String(this.resourceId));
        body.set('x', String(this.rect.x));
        body.set('y', String(this.rect.y));
        body.set('width', String(this.rect.width));
        body.set('height', String(this.rect.height));
        if (asNewResource) body.set('asNewResource', 'true');
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
          if (generation !== this._generation) return;
          this.errorMessage = message;
          this.isSubmitting = false;
          return;
        }

        if (asNewResource) {
          // The source is untouched, so there is nothing to refresh and no
          // reason to leave: stay open, report where the crop landed, and clear
          // the selection so the next region can be picked straight away.
          let newId = 0;
          try {
            const data = await response.json();
            newId = Number(data && data.id) || 0;
          } catch (_) { /* the id is a convenience, not a success condition */ }
          if (generation !== this._generation) return;
          this.newResourceId = newId;
          this.successMessage = newId
            ? `Saved as a new resource (#${newId}). The original is unchanged.`
            : 'Saved as a new resource. The original is unchanged.';
          this.rect = { x: 0, y: 0, width: 0, height: 0 };
          this.comment = '';
          this.isSubmitting = false;
          if (typeof this.onNewResource === 'function') this.onNewResource(newId);
          return;
        }

        if (generation !== this._generation) return;

        if (typeof this.onSuccess === 'function') {
          // The callback may tear this component down (the lightbox closes the
          // overlay), so don't await it — isSubmitting staying true is harmless.
          // It is told which resource was cropped: the lightbox may have been
          // navigated on while the request was out, and the version now belongs
          // to the resource this cropper was opened for, not to whatever is on
          // screen when the response lands.
          this.onSuccess(this.resourceId);
          return;
        }

        window.location.reload();
      } catch (err) {
        if (generation !== this._generation) return;
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
      this.successMessage = '';
      this.newResourceId = 0;
      this.saveMode = 'version';
      this.isSubmitting = false;
      this._drag = null;
      this._generation++;
      // Keep decodeFailed — it reflects whether the image can be decoded
      // at all, not per-interaction state; resetting would incorrectly
      // re-enable the disabled Crop button.
    },
  };
}
