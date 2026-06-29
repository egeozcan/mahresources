/**
 * Crop/rotate state and methods for the lightbox store.
 *
 * Exposes the two image-editing operations that otherwise only live on the
 * resource details page: a one-click "Rotate 90°" and a "Crop" overlay (which
 * reuses the shared `imageCropper` component). Both POST to the same
 * `/v1/resources/{rotate,crop}` endpoints used by the details page; each creates
 * a new resource version (Hash/Width/Height change, thumbnails cleared), so we
 * refresh the affected item in place rather than reloading the page.
 *
 * All methods use `this` bound to the Alpine store.
 */
export const cropPanelState = {
  // Crop overlay open/closed.
  cropOpen: false,
  // True while a rotate POST is in flight (guards against double-submit).
  rotating: false,
};

// Content types the crop tool supports, mirroring the raster allowlist used on
// the resource details page (displayResource.tpl). Excludes SVG and video.
const CROPPABLE_CONTENT_TYPES = new Set([
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/webp',
  'image/bmp',
  'image/tiff',
  'image/heic',
  'image/heif',
  'image/avif',
]);

export const cropPanelMethods = {
  _isCroppable(contentType) {
    return CROPPABLE_CONTENT_TYPES.has(contentType);
  },

  openCrop() {
    const item = this.getCurrentItem();
    if (!item || !this._isCroppable(item.contentType)) return;
    // On narrow viewports the side panels and the crop overlay compete for the
    // same space; close them so the crop UI gets the full width.
    if (window.innerWidth < 1024) {
      if (this.quickTagPanelOpen) this.closeQuickTagPanel();
      if (this.editPanelOpen) this.closeEditPanel();
    }
    this.cropOpen = true;
    this.announce('Crop image dialog opened');
  },

  closeCrop() {
    if (!this.cropOpen) return;
    this.cropOpen = false;
    // Focus is restored by the overlay's x-trap teardown (returns to the Crop
    // button), so we deliberately do not move focus here.
    this.announce('Crop image dialog closed');
  },

  async onCropSuccess() {
    const targetId = this.getCurrentItem()?.id;
    this.closeCrop();
    await this.refreshCurrentItem(targetId);
    this.announce('Image cropped');
  },

  async rotateCurrent(degrees = 90) {
    const item = this.getCurrentItem();
    if (!item || !this.isImage(item.contentType) || this.rotating) return;

    const targetId = item.id;
    this.rotating = true;
    // Reuse the media spinner for feedback during the re-encode round-trip; it
    // clears via @load once the new image swaps in (or below on error).
    this.loading = true;

    try {
      const body = new URLSearchParams();
      body.set('id', String(targetId));
      body.set('degrees', String(degrees));

      const response = await fetch('/v1/resources/rotate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        body: body.toString(),
      });

      if (!response.ok) {
        // Only clear the spinner if we're still showing the item we rotated —
        // the user may have navigated away mid-request, and that item owns its
        // own loading state (mirrors the currentIndex discipline in refreshCurrentItem).
        if (this.getCurrentItem()?.id === targetId) this.loading = false;
        let message = `Rotate failed (HTTP ${response.status})`;
        try {
          const text = await response.text();
          if (text) message = text;
        } catch (_) { /* ignore */ }
        this.announce(message);
        return;
      }

      await this.refreshCurrentItem(targetId);
      this.announce('Image rotated');
    } catch (err) {
      if (this.getCurrentItem()?.id === targetId) this.loading = false;
      console.error('Failed to rotate image:', err);
      this.announce('Failed to rotate image');
    } finally {
      this.rotating = false;
    }
  },

  // Re-fetch a resource's metadata after an in-place edit (crop/rotate) and
  // update its lightbox item so the new version's image is displayed. The id is
  // captured by the caller before any await so a mid-flight navigation cannot
  // misdirect the update onto a different resource.
  async refreshCurrentItem(targetId) {
    if (!targetId) return;

    let data;
    try {
      const response = await fetch(`/resource.json?id=${targetId}`, {
        headers: { 'Accept': 'application/json' },
      });
      if (!response.ok) {
        throw new Error(`Failed to refresh resource: ${response.status}`);
      }
      data = await response.json();
    } catch (err) {
      console.error('Failed to refresh resource after edit:', err);
      return;
    }

    const r = data.resource ?? data;
    const idx = this.items.findIndex(i => i.id === targetId);
    if (idx === -1) return; // resource navigated away and dropped from the list

    const hash = r.Hash || '';
    const versionParam = hash ? `&v=${hash}` : '';
    this.items[idx] = {
      ...this.items[idx],
      hash,
      viewUrl: `/v1/resource/view?id=${r.ID}${versionParam}`,
      // Crop/rotate re-encode the image, so the content type can change (e.g.
      // rotate always re-encodes to JPEG); keep the item in sync.
      contentType: r.ContentType || this.items[idx].contentType,
      width: r.Width || 0,
      height: r.Height || 0,
    };

    // Keep cached/open panel details consistent with the new version.
    this.detailsCache.set(targetId, r);

    // Only touch the viewer if the edited resource is still the one on screen.
    if (this.currentIndex === idx) {
      if (this.editPanelOpen || this.quickTagPanelOpen) {
        this.resourceDetails = r;
      }
      // The image is a different size now — drop any stale zoom/pan and show the
      // spinner until the new bitmap loads.
      this.resetZoom();
      this.loading = true;
      this.scheduleMediaCheck();
    }

    // The underlying gallery thumbnail is now stale; refresh it when the
    // lightbox (or its last panel) closes, mirroring the tag/name edit path.
    this.needsRefreshOnClose = true;
  },
};
