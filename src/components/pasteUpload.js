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
    for (const item of clipboardData.items) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile();
        if (!file) continue;
        const ext = item.type.split('/')[1] || 'png';
        const name = timestampedName('pasted-image', ext);
        return [{
          file,
          name,
          previewUrl: URL.createObjectURL(file),
          type: 'image',
          error: null,
          _snippet: null,
        }];
      }
    }
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
      _snippet: text.length > 120 ? text.slice(0, 120) + '\u2026' : text,
    }];
  }

  return [];
}

// ---------------------------------------------------------------------------
// Alpine store
// ---------------------------------------------------------------------------

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

    /** @type {number|null} */
    _infoTimer: null,

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
      if (this._infoTimer) clearTimeout(this._infoTimer);
      this._infoTimer = setTimeout(() => {
        this.infoMessage = '';
        this._infoTimer = null;
      }, 4000);
    },
  });
}
