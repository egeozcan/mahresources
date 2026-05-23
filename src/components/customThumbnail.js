// Custom thumbnail Alpine component.
//
// Lives on the resource details page (templates/displayResource.tpl). Lets
// the user upload an image to replace the auto-generated thumbnail or clear
// custom/auto previews so the next request regenerates from source.
//
// After a successful upload or regenerate, mutate any <img> elements on the
// page that point at /v1/resource/preview?id=<this-resource> so the browser
// re-fetches them. We can't invalidate the HTTP cache from JS, so we change
// the URL (cache key) by appending a fresh _t timestamp.

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
    resourceId,
    isBusy: false,
    errorMessage: '',
    statusMessage: '',

    triggerFilePick() {
      const input = this.$refs.fileInput;
      if (input) input.click();
    },

    async onFileChosen(event) {
      const files = event.target.files;
      if (!files || files.length === 0) return;
      await this.upload(files[0]);
      event.target.value = '';
    },

    async onPaste(event) {
      const items = (event.clipboardData && event.clipboardData.items) || [];
      for (const item of items) {
        if (item.kind === 'file' && item.type.startsWith('image/')) {
          const file = item.getAsFile();
          if (file) {
            event.preventDefault();
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
