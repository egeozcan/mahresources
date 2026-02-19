/**
 * Paste Upload Alpine store and content extraction utilities.
 *
 * Provides a modal-style workflow: clipboard content is extracted into preview
 * items, the user can review/tag/remove them, and then upload.
 */

// ---------------------------------------------------------------------------
// Helpers (module-private)
// ---------------------------------------------------------------------------

/**
 * Generate a timestamped filename.
 * @param {string} prefix  e.g. "pasted-image"
 * @param {string} ext     e.g. "png"
 * @returns {string}       e.g. "pasted-image-2026-02-19T12-00-00.png"
 */
function timestampedName(prefix, ext) {
  const stamp = new Date()
    .toISOString()
    .replace(/\.\d{3}Z$/, '')   // drop milliseconds + Z
    .replace(/:/g, '-');         // colons are not filename-safe
  return `${prefix}-${stamp}.${ext}`;
}

/**
 * Map a MIME type to a short human-readable label used for preview cards.
 * @param {string} mime
 * @returns {string}
 */
function friendlyType(mime) {
  if (!mime) return 'file';
  if (mime.startsWith('image/')) return 'image';
  if (mime === 'text/html') return 'html';
  if (mime.startsWith('text/')) return 'text';
  return 'file';
}

/**
 * Strip HTML tags and collapse whitespace, returning at most `maxLen` chars.
 * Used to create the `_snippet` preview for pasted rich-text / plain-text.
 * @param {string} html
 * @param {number} [maxLen=120]
 * @returns {string}
 */
function stripToSnippet(html, maxLen = 120) {
  const text = html.replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim();
  return text.length > maxLen ? text.slice(0, maxLen) + '\u2026' : text;
}

// ---------------------------------------------------------------------------
// Content extraction
// ---------------------------------------------------------------------------

/**
 * Extract uploadable items from a ClipboardEvent's `clipboardData`.
 *
 * Priority order:
 *  1. `clipboardData.files`          -- real files (drag-drop, copy from OS)
 *  2. `clipboardData.items` images   -- screenshots via getAsFile()
 *  3. `text/html` data               -- rich text, wrapped in a Blob
 *  4. `text/plain` data              -- plain text, wrapped in a Blob
 *
 * Each returned item has the shape:
 *   { file: File, name: string, previewUrl: string|null, type: string, error: null, _snippet: string|null }
 *
 * @param {DataTransfer} clipboardData
 * @returns {Array<{file: File, name: string, previewUrl: string|null, type: string, error: null, _snippet: string|null}>}
 */
export function extractPasteContent(clipboardData) {
  if (!clipboardData) return [];

  // --- Priority 1: real files -------------------------------------------------
  if (clipboardData.files && clipboardData.files.length > 0) {
    const items = [];
    for (const file of clipboardData.files) {
      const isImage = file.type.startsWith('image/');
      const name = file.name && file.name !== ''
        ? file.name
        : timestampedName('pasted-file', file.type.split('/')[1] || 'bin');
      items.push({
        file,
        name,
        previewUrl: isImage ? URL.createObjectURL(file) : null,
        type: friendlyType(file.type),
        error: null,
        _snippet: null,
      });
    }
    return items;
  }

  // --- Priority 2: image items (screenshots) ----------------------------------
  if (clipboardData.items) {
    const imageItems = [];
    for (const item of clipboardData.items) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile();
        if (!file) continue;
        const ext = item.type.split('/')[1] || 'png';
        const name = timestampedName('pasted-image', ext);
        imageItems.push({
          file,
          name,
          previewUrl: URL.createObjectURL(file),
          type: 'image',
          error: null,
          _snippet: null,
        });
      }
    }
    if (imageItems.length > 0) return imageItems;
  }

  // --- Priority 3: HTML text --------------------------------------------------
  const html = clipboardData.getData('text/html');
  if (html) {
    const blob = new Blob([html], { type: 'text/html' });
    const file = new File([blob], timestampedName('pasted-html', 'html'), { type: 'text/html' });
    return [{
      file,
      name: file.name,
      previewUrl: null,
      type: 'html',
      error: null,
      _snippet: stripToSnippet(html),
    }];
  }

  // --- Priority 4: plain text -------------------------------------------------
  const text = clipboardData.getData('text/plain');
  if (text) {
    const blob = new Blob([text], { type: 'text/plain' });
    const file = new File([blob], timestampedName('pasted-text', 'txt'), { type: 'text/plain' });
    return [{
      file,
      name: file.name,
      previewUrl: null,
      type: 'text',
      error: null,
      _snippet: stripToSnippet(text),
    }];
  }

  return [];
}

// ---------------------------------------------------------------------------
// Alpine store
// ---------------------------------------------------------------------------

/** Timer handle for the auto-dismissing info message (kept outside the store to avoid Alpine reactivity). */
let _infoTimer = null;

/**
 * Set up the global paste event listener.
 *
 * The handler runs three guard checks before opening the paste-upload modal:
 *  1. If a file input exists AND the clipboard has files, reproduce the legacy
 *     paste-into-file-input behaviour (merge files, flash ring).
 *  2. If the active element is a text input / textarea / contentEditable, bail.
 *  3. If no useful clipboard content can be extracted, bail.
 *
 * When all guards pass the handler tries to determine an upload context from the
 * page (data-paste-context attribute, or ownerId query-param) and opens the
 * paste-upload store accordingly.
 */
export function setupPasteListener() {
  window.addEventListener('paste', async (e) => {
    // --- Guard 1: file input on page + clipboard has files → legacy behaviour -
    const fileInput = document.querySelector("input[type='file']");
    if (fileInput && e.clipboardData?.files && e.clipboardData.files.length > 0) {
      e.preventDefault();
      const dt = new DataTransfer();
      for (const file of fileInput.files) {
        dt.items.add(file);
      }
      for (const file of e.clipboardData.files) {
        dt.items.add(file);
      }
      fileInput.files = dt.files;
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
      fileInput.closest('.flex')?.classList.add('ring-2', 'ring-indigo-500', 'rounded-md');
      setTimeout(() => fileInput.closest('.flex')?.classList.remove('ring-2', 'ring-indigo-500', 'rounded-md'), 1500);
      return;
    }

    // --- Guard 2: focus is inside a text input / textarea / contentEditable ----
    const active = document.activeElement;
    if (active) {
      const tag = active.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || active.isContentEditable) {
        return;
      }
    }

    // --- Guard 3: extract paste content; bail if empty -------------------------
    const items = extractPasteContent(e.clipboardData);
    if (items.length === 0) return;

    e.preventDefault();

    // --- Obtain Alpine store ---------------------------------------------------
    const store = window.Alpine?.store('pasteUpload');
    if (!store) return;

    // --- Context detection: data-paste-context attribute -----------------------
    const ctxEl = document.querySelector('[data-paste-context]');
    if (ctxEl) {
      try {
        const context = JSON.parse(ctxEl.getAttribute('data-paste-context'));
        store.open(items, context);
      } catch (err) {
        console.error('Failed to parse data-paste-context:', err);
        store.showInfo('Invalid paste context on this page.');
        for (const item of items) {
          if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
        }
      }
      return;
    }

    // --- Context detection: ownerId query param → fetch group -----------------
    const ownerId = new URLSearchParams(window.location.search).get('ownerId');
    if (ownerId) {
      try {
        const resp = await fetch(`/v1/group.json?id=${ownerId}`);
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const group = await resp.json();
        store.open(items, { type: 'group', id: group.ID, name: group.Name });
      } catch (err) {
        console.error('Failed to fetch owner group:', err);
        store.showInfo('Could not determine the owner group for pasted content.');
        for (const item of items) {
          if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
        }
      }
      return;
    }

    // --- No context found ------------------------------------------------------
    store.showInfo('To paste and upload, navigate to a group or note detail page, or filter a list by owner.');
    for (const item of items) {
      if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
    }
  });
}

/**
 * Register the `pasteUpload` Alpine store.
 * @param {import('alpinejs').Alpine} Alpine
 */
export function registerPasteUploadStore(Alpine) {
  Alpine.store('pasteUpload', {
    // ----- state ----------------------------------------------------------
    isOpen: false,
    items: [],
    context: null,       // { type, id, ownerId?, name }
    tags: [],
    categoryId: null,
    state: 'idle',       // 'idle' | 'preview' | 'uploading' | 'success' | 'error'
    uploadProgress: '',
    errorMessage: '',
    infoMessage: '',

    // ----- methods --------------------------------------------------------

    /**
     * Open the paste-upload modal with extracted items and page context.
     * @param {Array} items   output of `extractPasteContent`
     * @param {{ type: string, id: number|string, ownerId?: number|string, name?: string }|null} context
     */
    open(items, context) {
      if (!items || items.length === 0) return;
      this.items = items;
      this.context = context || null;
      this.tags = [];
      this.categoryId = null;
      this.state = 'preview';
      this.uploadProgress = '';
      this.errorMessage = '';
      this.infoMessage = '';
      this.isOpen = true;
    },

    /**
     * Close the modal and clean up object URLs to prevent memory leaks.
     */
    close() {
      // Clear any pending info-message timer
      if (_infoTimer) {
        clearTimeout(_infoTimer);
        _infoTimer = null;
      }
      // Revoke every object URL still held by items
      for (const item of this.items) {
        if (item.previewUrl) {
          URL.revokeObjectURL(item.previewUrl);
        }
      }
      this.items = [];
      this.context = null;
      this.tags = [];
      this.categoryId = null;
      this.state = 'idle';
      this.uploadProgress = '';
      this.errorMessage = '';
      this.infoMessage = '';
      this.isOpen = false;
    },

    /**
     * Remove a single item by index. Revokes its object URL.
     * Auto-closes the modal when no items remain.
     * @param {number} index
     */
    removeItem(index) {
      if (index < 0 || index >= this.items.length) return;
      const [removed] = this.items.splice(index, 1);
      if (removed && removed.previewUrl) {
        URL.revokeObjectURL(removed.previewUrl);
      }
      if (this.items.length === 0) {
        this.close();
      }
    },

    /**
     * Display a temporary info message that auto-dismisses after 4 seconds.
     * @param {string} message
     */
    showInfo(message) {
      this.infoMessage = message;
      if (_infoTimer) clearTimeout(_infoTimer);
      _infoTimer = setTimeout(() => {
        this.infoMessage = '';
        _infoTimer = null;
      }, 4000);
    },

    /**
     * Upload items sequentially to the server, tracking progress and errors.
     * Skips items already marked as 'done' (useful for retries).
     */
    async upload() {
      if (this.items.length === 0 || !this.context) return;

      this.state = 'uploading';
      this.errorMessage = '';

      const total = this.items.filter(i => i.error !== 'done').length;
      let successCount = 0;
      let current = 0;

      for (const item of this.items) {
        if (item.error === 'done') continue;

        current++;
        this.uploadProgress = `Uploading ${current} of ${total}...`;

        const formData = new FormData();
        formData.append('resource', item.file, item.name);

        if (this.context.type === 'group') {
          formData.append('ownerId', this.context.id);
          formData.append('groups', this.context.id);
        } else if (this.context.type === 'note') {
          if (this.context.ownerId) {
            formData.append('ownerId', this.context.ownerId);
          }
          formData.append('notes', this.context.id);
        }

        for (const tagId of this.tags) {
          formData.append('tags', tagId);
        }

        if (this.categoryId) {
          formData.append('resourceCategoryId', this.categoryId);
        }

        try {
          const response = await fetch('/v1/resource', {
            method: 'POST',
            body: formData,
          });
          if (!response.ok) {
            const text = await response.text();
            item.error = text || `HTTP ${response.status}`;
          } else {
            item.error = 'done';
            successCount++;
          }
        } catch (err) {
          item.error = err.message || 'Network error';
        }
      }

      if (successCount === total) {
        this.state = 'success';
        this.uploadProgress = `Uploaded ${successCount} file${successCount !== 1 ? 's' : ''} successfully.`;
        setTimeout(() => {
          this.close();
          this._refreshPage();
        }, 800);
      } else if (successCount > 0) {
        for (const item of this.items) {
          if (item.error === 'done' && item.previewUrl) {
            URL.revokeObjectURL(item.previewUrl);
          }
        }
        this.items = this.items.filter(i => i.error !== 'done');
        this.state = 'error';
        this.errorMessage = `${successCount} succeeded, ${total - successCount} failed.`;
      } else {
        this.state = 'error';
        this.errorMessage = `All ${total} upload${total !== 1 ? 's' : ''} failed.`;
      }
    },

    /**
     * Re-fetch the current page HTML and morph the `.main` container in place,
     * preserving Alpine state. Falls back to a full reload on error.
     */
    async _refreshPage() {
      try {
        const response = await fetch(window.location.href, {
          headers: { 'Accept': 'text/html' },
        });
        if (!response.ok) {
          window.location.reload();
          return;
        }
        const html = await response.text();

        const parser = new DOMParser();
        const doc = parser.parseFromString(html, 'text/html');
        const newMain = doc.querySelector('.main');
        const main = document.querySelector('.main');

        if (main && newMain) {
          window.Alpine.morph(main, newMain, {
            updating(el, toEl, childrenOnly, skip) {
              if (el._x_dataStack) {
                toEl._x_dataStack = el._x_dataStack;
              }
            },
          });
          window.Alpine?.store('lightbox')?.initFromDOM();
        }
      } catch (err) {
        console.error('Failed to refresh page after upload:', err);
        window.location.reload();
      }
    },
  });
}
