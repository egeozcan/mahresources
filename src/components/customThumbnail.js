// Custom thumbnail Alpine component.
//
// Lives on the resource details page (templates/displayResource.tpl), as a
// sidebar button that opens a popup holding the upload and regenerate controls.
// It used to be three controls in the sidebar itself, with a `@paste.window`
// handler live on the whole page: an image pasted anywhere on the resource
// replaced the thumbnail with nothing on screen saying so. A page-wide side
// effect needs a visible owner, so the paste now belongs to the popup.
//
// After a successful upload or regenerate, mutate any <img> elements on the
// page that point at /v1/resource/preview?id=<this-resource> so the browser
// re-fetches them. We can't invalidate the HTTP cache from JS, so we change
// the URL (cache key) by appending a fresh _t timestamp.
//
// `refreshPreviewImages` is module-level on purpose: `close()` is allowed while
// a request is in flight, and dismissing the dialog must not cancel the
// refresh. The status line is what is lost by closing early, not the image.

import { sidebarPopup } from './sidebarPopup.js';

function refreshPreviewImages(resourceId) {
  const prefix = `/v1/resource/preview?id=${resourceId}`;
  const images = document.querySelectorAll('img');
  const stamp = Date.now();
  images.forEach((img) => {
    const src = img.getAttribute('src');
    if (!src || !src.startsWith(prefix)) return;
    const cleaned = src.replace(/([?&])_t=\d+&?/g, '$1').replace(/[?&]$/, '');
    const sep = cleaned.includes('?') ? '&' : '?';
    img.setAttribute('src', `${cleaned}${sep}_t=${stamp}`);
  });
}

export function customThumbnail({ resourceId }) {
  return {
    ...sidebarPopup(),
    resourceId,
    isBusy: false,
    errorMessage: '',
    statusMessage: '',

    // The last outcome described a popup that no longer exists.
    onOpen() {
      this.errorMessage = '';
      this.statusMessage = '';
    },

    triggerFilePick() {
      if (this.isBusy) return;
      const input = this.$refs.fileInput;
      if (input) input.click();
    },

    async onFileChosen(event) {
      const files = event.target.files;
      if (!files || files.length === 0) return;
      await this.upload(files[0]);
      event.target.value = '';
    },

    /**
     * A pasted image is this popup's, and only while it is open.
     *
     * Bound on `window` in the CAPTURE phase (see displayResource.tpl), because
     * `setupPasteListener()` shares that target and would handle the same paste:
     * its guard 2 — a file input is on the page and the clipboard has files —
     * merges the clipboard into whichever input that is, which is the popup's
     * own, and dispatches `change`, uploading the same bytes a second time.
     * Which listener runs first is decided by registration order (Alpine binds
     * `@paste.window` inside `Alpine.start()`, before `setupPasteListener()`),
     * and a capture-phase listener does not depend on that: it reaches `window`
     * before any bubble-phase listener on it, so `stopImmediatePropagation` is
     * the one that makes the popup the single owner of its own paste.
     *
     * A clipboard with no image is none of our business and is left entirely
     * alone, stop propagation included — that is still somebody else's paste.
     */
    async onPaste(event) {
      if (!this.isOpen) return;
      const items = (event.clipboardData && event.clipboardData.items) || [];
      for (const item of items) {
        if (item.kind === 'file' && item.type.startsWith('image/')) {
          const file = item.getAsFile();
          if (file) {
            event.preventDefault();
            event.stopImmediatePropagation();
            await this.upload(file);
            return;
          }
        }
      }
    },

    async upload(file) {
      if (!file) return;
      this.isBusy = true;
      this.errorMessage = '';
      this.statusMessage = '';
      try {
        const form = new FormData();
        form.append('thumbnail', file, file.name || 'thumbnail');
        const res = await fetch(`/v1/resource/preview?id=${this.resourceId}`, {
          method: 'POST',
          body: form,
          headers: { Accept: 'application/json' },
        });
        if (!res.ok) {
          const text = await res.text();
          throw new Error(text || `Upload failed: HTTP ${res.status}`);
        }
        refreshPreviewImages(this.resourceId);
        this.statusMessage = 'Custom thumbnail saved.';
      } catch (err) {
        this.errorMessage = err && err.message ? err.message : String(err);
      } finally {
        this.isBusy = false;
      }
    },

    async regenerate() {
      if (this.isBusy) return;
      this.isBusy = true;
      this.errorMessage = '';
      this.statusMessage = '';
      try {
        const res = await fetch(`/v1/resource/preview?id=${this.resourceId}`, {
          method: 'DELETE',
          headers: { Accept: 'application/json' },
        });
        if (!res.ok) {
          const text = await res.text();
          throw new Error(text || `Regenerate failed: HTTP ${res.status}`);
        }
        refreshPreviewImages(this.resourceId);
        this.statusMessage = 'Thumbnails cleared. The next view regenerates from source.';
      } catch (err) {
        this.errorMessage = err && err.message ? err.message : String(err);
      } finally {
        this.isBusy = false;
      }
    },
  };
}
