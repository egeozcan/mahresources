/**
 * Crop/rotate state and methods for the lightbox store.
 *
 * Exposes the two image-editing operations that otherwise only live on the
 * resource details page: a one-click "Rotate 90°" and a "Crop" overlay (which
 * reuses the shared `imageCropper` component). Both POST to the same
 * `/v1/resources/{rotate,crop}` endpoints used by the details page. Rotate, and
 * crop in its default "new version" mode, create a new resource version
 * (Hash/Width/Height change, thumbnails cleared), so we refresh the affected item
 * in place rather than reloading the page. Crop in "new resource" mode touches
 * nothing on screen — see `onCropSavedAsNewResource`.
 *
 * All methods use `this` bound to the Alpine store.
 */
export const cropPanelState = {
  // Crop overlay open/closed.
  cropOpen: false,
  // True while a rotate POST is in flight (guards against double-submit).
  rotating: false,
};

// Content types the pixel editors (crop and rotate) support, mirroring
// models.RasterImageContentTypes on the server and the `isRasterImage` gate on
// the resource details page. Excludes SVG, ICO and video: the server answers
// those with a 415, so offering the buttons only produces a failed request.
const RASTER_EDITABLE_CONTENT_TYPES = new Set([
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
  // Canonical predicate for both pixel-editing actions.
  _isRasterImage(contentType) {
    return RASTER_EDITABLE_CONTENT_TYPES.has(contentType);
  },

  _isCroppable(contentType) {
    return this._isRasterImage(contentType);
  },

  openCrop() {
    const item = this.getCurrentItem();
    if (!item || !this._isRasterImage(item.contentType)) return;
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

  // `croppedId` is the resource the cropper was opened for. It matters because
  // the overlay can be closed and the viewer navigated on while the POST is
  // still out: the new version belongs to that resource, not to whatever is on
  // screen when the response lands. Reading the current item here instead — as
  // this used to — refreshed the wrong image and announced a crop that had not
  // happened to it. refreshCurrentItem addresses items by id, so a target that
  // is no longer on screen updates in the list without disturbing the view.
  async onCropSuccess(croppedId) {
    const targetId = croppedId || this.getCurrentItem()?.id;
    // Only close the overlay that submitted. A response arriving after the user
    // reopened crop on a different image must not close that one.
    if (this.cropOpen && this.getCurrentItem()?.id === targetId) {
      this.closeCrop();
    }
    await this.refreshCurrentItem(targetId);
    this.announce('Image cropped');
  },

  // The crop was saved as a separate resource. The item on screen is byte-for-byte
  // what it was, so refreshCurrentItem must not run — that path exists to re-point
  // an item at a new version and would refetch for nothing. The overlay also stays
  // open, so no focus or panel state moves here. The gallery behind the lightbox is
  // what went stale: it has a resource it never listed.
  //
  // Deliberately silent: the cropper carries its own always-mounted role="status"
  // region for this outcome, so announcing here too would read it twice.
  onCropSavedAsNewResource() {
    this.needsRefreshOnClose = true;
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

    // A crop or rotate rewrites the hash, dimensions and content type, so any details GET
    // in flight for this resource now describes the pre-edit version. Without this fence it
    // resolves after refreshCurrentItem has committed and puts the old metadata back —
    // including the old Hash, which is what the viewer's cache-busting URL is built from.
    const writeGeneration = this._beginDetailsWrite(targetId);
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
      this._endDetailsWrite(targetId);
      return;
    }

    const r = data.resource ?? data;
    const idx = this.items.findIndex(i => i.id === targetId);
    if (idx === -1) {
      this._endDetailsWrite(targetId);
      return; // resource navigated away and dropped from the list
    }

    // Re-point the item at the new version. forceMedia: the edit produced a new file even in
    // the rare case where every field compares equal, so the zoom reset and the spinner must
    // happen regardless — the shared helper takes care of both when this is the current item.
    this._syncItemFromDetails(r, { forceMedia: true });

    // Keep cached/open panel details consistent with the new version, unless another write
    // to this resource landed while the refetch was out — its result is newer than ours.
    const committed = this._settleDetailsCache(targetId, writeGeneration, r);

    // Only paint the snapshot the cache accepted, and only while the edited resource is still
    // on screen. If another write landed while this refetch was out, `r` predates it and
    // showing it would visibly revert that edit; _endDetailsWrite's convergence refetch
    // supplies the authoritative version instead.
    if (this.currentIndex === idx && committed &&
        (this.editPanelOpen || this.quickTagPanelOpen)) {
      this.resourceDetails = r;
    }

    // The underlying gallery thumbnail is now stale; refresh it when the
    // lightbox (or its last panel) closes, mirroring the tag/name edit path.
    this.needsRefreshOnClose = true;

    // Closed only now, after the cache and panel have been updated, so no read can slip
    // between the fetch returning and the commit landing.
    this._endDetailsWrite(targetId);
  },
};
