/**
 * Paste Upload Alpine store and content extraction utilities.
 *
 * Provides a modal-style workflow: clipboard content is extracted into preview
 * items, the user can review/tag/remove them, and then upload.
 */

import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';
import { parseUploadError } from '../utils/uploadError.js';
import { snapshotDrop, walkDrop } from '../utils/dropEntries.js';
import { blockingModal, refuseOverModal } from '../utils/modality.js';

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
 *   { file: File, name: string, previewUrl: string|null, type: string, error: null, errorResourceId: null, _snippet: string|null }
 *
 * @param {DataTransfer} clipboardData
 * @returns {Array<{file: File, name: string, previewUrl: string|null, type: string, error: null, errorResourceId: null, _snippet: string|null}>}
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
        errorResourceId: null,
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
          errorResourceId: null,
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
      errorResourceId: null,
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
      errorResourceId: null,
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

/** Timer handle for the auto-close after successful upload. */
let _autoCloseTimer = null;

/**
 * Return true when paste should be handled by the currently focused editor.
 *
 * Shadow DOM retargets events and makes document.activeElement point at the
 * custom-element host, so check both the composed event path and activeElement
 * at every open shadow root boundary.
 */
function isEditablePasteTarget(event) {
  const isEditable = (element) => {
    if (!(element instanceof Element)) return false;
    return element.matches('input, textarea, [contenteditable]:not([contenteditable="false"])')
      || element.isContentEditable;
  };

  if (event.composedPath().some(isEditable)) return true;

  let active = document.activeElement;
  while (active) {
    if (isEditable(active)) return true;
    active = active.shadowRoot?.activeElement || null;
  }
  return false;
}

function revokePreviews(items) {
  for (const item of items) {
    if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
  }
}

/** True when the page names an upload target without needing a fetch. */
function pageHasUploadTarget() {
  return document.querySelector('[data-paste-context]') !== null
    || new URLSearchParams(window.location.search).get('ownerId') !== null;
}

/**
 * Work out where an upload from this page should go: the page's
 * `data-paste-context`, else the group named by the `ownerId` query param.
 * @returns {Promise<{ context: object|null, message: string }>}
 */
async function resolveUploadContext() {
  const ctxEl = document.querySelector('[data-paste-context]');
  if (ctxEl) {
    try {
      return { context: JSON.parse(ctxEl.getAttribute('data-paste-context')), message: '' };
    } catch (err) {
      console.error('Failed to parse data-paste-context:', err);
      return { context: null, message: 'Invalid paste context on this page.' };
    }
  }

  const ownerId = new URLSearchParams(window.location.search).get('ownerId');
  if (ownerId) {
    try {
      const resp = await fetch(`/v1/group.json?id=${encodeURIComponent(ownerId)}`);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const group = await resp.json();
      return { context: { type: 'group', id: group.ID, name: group.Name }, message: '' };
    } catch (err) {
      console.error('Failed to fetch owner group:', err);
      return { context: null, message: 'Could not determine the owner group for pasted content.' };
    }
  }

  return {
    context: null,
    message: 'To paste and upload, navigate to a group or note detail page, or filter a list by owner.',
  };
}

/**
 * Set up the global paste event listener.
 *
 * The handler runs three guard checks before opening the paste-upload modal:
 *  1. If focus is inside an input / textarea / contentEditable, bail.
 *  2. If a file input exists AND the clipboard has files, reproduce the legacy
 *     paste-into-file-input behaviour (merge files, flash ring).
 *  3. If no useful clipboard content can be extracted, bail.
 *
 * When all guards pass the handler tries to determine an upload context from the
 * page (data-paste-context attribute, or ownerId query-param) and opens the
 * paste-upload store accordingly.
 */
export function setupPasteListener() {
  window.addEventListener('paste', async (e) => {
    // --- Guard 1: let focused editing controls handle paste natively ---------
    if (isEditablePasteTarget(e)) return;

    // --- Guard 2: file input on page + clipboard has files → legacy behaviour -
    // Skipped when the page advertises an explicit paste-upload context (detail
    // and owner-filtered list pages carry a [data-paste-context]). On those the
    // paste-upload modal always wins, so an *unrelated* file input — e.g. one
    // rendered by a category CustomHeader/CustomSidebar or a plugin slot — can't
    // silently swallow the paste. Pages built around a real upload form (e.g.
    // createResource) carry no paste-context, so they keep the legacy behaviour.
    const hasPasteContext = document.querySelector('[data-paste-context]') !== null;
    // `:not([disabled])` matters. The create-resource page disables its picker
    // while a bulk upload is running, and a disabled control should not receive
    // a paste — files merged into it there look selected but belong to no batch,
    // and the run that is already in flight navigates away without them.
    const fileInput = document.querySelector("input[type='file']:not([disabled])");
    if (!hasPasteContext && fileInput && e.clipboardData?.files && e.clipboardData.files.length > 0) {
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

    // --- Guard 3: extract paste content; bail if empty -------------------------
    const items = extractPasteContent(e.clipboardData);
    if (items.length === 0) return;

    e.preventDefault();

    // --- Obtain Alpine store ---------------------------------------------------
    const store = window.Alpine?.store('pasteUpload');
    if (!store) return;

    // --- Context detection ------------------------------------------------------
    const resolved = await resolveUploadContext();
    if (resolved.context) {
      store.open(items, resolved.context);
    } else {
      store.showInfo(resolved.message);
      revokePreviews(items);
    }
  });
}

/** Another painted dialog, ignoring the upload dialog itself (a drop may append to it). */
const otherModal = () => blockingModal(document.querySelector('[aria-labelledby="paste-upload-title"]'));

/** Image thumbnails made per drop; a big folder would otherwise hold thousands of object URLs. */
const DROP_PREVIEW_LIMIT = 60;

/** Each dropped root gets an id (drop.root) so two folders with the same name never share a group. */
let dropSeq = 0;

/**
 * Set up drag-and-drop upload. It is active on the pages that already take a
 * paste (a `[data-paste-context]` or an `ownerId` filter) and leaves every
 * other page's drop to the browser, so a native file input still works.
 *
 * Dropped folders arrive with the folder chain each file was found in; the
 * store decides whether that becomes groups.
 */
export function setupDropListener() {
  let depth = 0;
  const getStore = () => window.Alpine?.store('pasteUpload');
  const carriesFiles = (e) => Array.from(e.dataTransfer?.types ?? []).includes('Files');
  const eligible = (e) => carriesFiles(e)
    && pageHasUploadTarget()
    && !(e.target instanceof Element && e.target.closest("input[type='file']"));
  const hideOverlay = () => {
    depth = 0;
    const store = getStore();
    if (store) store.dragActive = false;
  };

  // Enter and leave are counted on the same terms, whatever the target, or
  // crossing a native file input (which gets no preventDefault) skews the count.
  const counted = (e) => carriesFiles(e) && pageHasUploadTarget();

  window.addEventListener('dragenter', (e) => {
    if (!counted(e)) return;
    depth++;
    const store = getStore();
    if (!store || store.state === 'uploading' || otherModal()) return;
    let name = '';
    try {
      name = JSON.parse(document.querySelector('[data-paste-context]')?.getAttribute('data-paste-context') || '{}').name || '';
    } catch (_) { /* the drop itself reports an unreadable context */ }
    store.dragTarget = name;
    store.dragActive = true;
  });

  window.addEventListener('dragover', (e) => {
    if (e.defaultPrevented || !eligible(e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  });

  window.addEventListener('dragleave', (e) => {
    if (!counted(e)) return;
    depth = Math.max(0, depth - 1);
    if (depth === 0) hideOverlay();
  });

  window.addEventListener('drop', async (e) => {
    // A drop onto a native file input is left to the browser, which also means
    // no dragleave follows, so the overlay is cleared here.
    if (counted(e)) hideOverlay();
    if (!eligible(e)) return;
    // Something on the page already took this drop (a widget in a custom
    // header, say); it is theirs, not ours to open a second workflow for.
    if (e.defaultPrevented) {
      hideOverlay();
      return;
    }
    e.preventDefault();

    // Two aria-modal dialogs at once is a defect (utils/modality.js); say why nothing opened.
    const blocker = otherModal();
    if (blocker) {
      refuseOverModal(blocker, 'Close this dialog first to upload dropped files.');
      return;
    }

    // The DataTransfer is only readable until the first await.
    const snapshot = snapshotDrop(e.dataTransfer);
    const store = getStore();
    if (!store) return;
    if (store.state === 'uploading') {
      store.showInfo('Wait for the current upload to finish, then drop again.');
      return;
    }

    // A close during the walk cancels this drop; an upload started during it
    // must not have its batch replaced.
    const session = store.sessionId;
    const [walked, resolved] = await Promise.all([walkDrop(snapshot), resolveUploadContext()]);
    if (store.sessionId !== session) return;
    const lateBlocker = otherModal();
    if (lateBlocker) {
      refuseOverModal(lateBlocker, 'Close this dialog first to upload dropped files.');
      return;
    }
    if (!resolved.context) {
      store.showInfo(resolved.message);
      return;
    }

    const dropSeqNow = ++dropSeq;
    let previewsLeft = DROP_PREVIEW_LIMIT;
    const items = walked.files.map(({ file, dirPath, root }) => {
      const asImage = file.type.startsWith('image/') && previewsLeft-- > 0;
      return {
        file,
        name: file.name,
        previewUrl: asImage ? URL.createObjectURL(file) : null,
        type: asImage ? 'image' : (friendlyType(file.type) === 'image' ? 'file' : friendlyType(file.type)),
        error: null,
        errorResourceId: null,
        _snippet: null,
        dirPath,
        dropId: `${dropSeqNow}.${root}`,
      };
    });

    if (items.length === 0) {
      store.showInfo(walked.unreadable > 0
        ? 'Could not read the dropped items.'
        : 'Nothing to upload: the drop held no files, only empty folders or hidden files.');
      return;
    }
    store.open(items, resolved.context);
    if (walked.unreadable > 0) {
      store.showInfo(`${walked.unreadable} dropped item${walked.unreadable !== 1 ? 's' : ''} could not be read.`);
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
    seriesId: null,
    groupCategoryId: null,   // Category for groups created from dropped folders
    keepStructure: true,     // dropped folders become groups instead of flattening
    dragActive: false,       // a file drag is over the page
    dragTarget: '',          // name shown on the drop overlay
    _folderGroups: {},       // folder key -> id of the group created for it
    sessionId: 0,            // bumped on close so a drop still being read can tell it was cancelled
    state: 'idle',       // 'idle' | 'preview' | 'uploading' | 'success' | 'error'
    uploadProgress: '',
    errorMessage: '',
    infoMessage: '',

    // ----- methods --------------------------------------------------------

    /**
     * Open the paste-upload modal with extracted items and page context.
     *
     * When the dialog is already open (and not mid-upload), new items are
     * **appended** so users can build up a batch across multiple pastes.
     *
     * @param {Array} items   output of `extractPasteContent`
     * @param {{ type: string, id: number|string, ownerId?: number|string, name?: string }|null} context
     */
    open(items, context) {
      if (!items || items.length === 0) return;

      // Opening now would replace the batch that is being uploaded.
      if (this.state === 'uploading') {
        for (const item of items) {
          if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
        }
        this.showInfo('Wait for the current upload to finish, then try again.');
        return;
      }

      // Append to existing list when the dialog is already visible
      if (this.isOpen && this.state !== 'uploading') {
        // Cancel any pending auto-close from a previous successful upload
        if (_autoCloseTimer) {
          clearTimeout(_autoCloseTimer);
          _autoCloseTimer = null;
        }
        // Remove successfully-uploaded items from a previous batch
        for (const existing of this.items) {
          if (existing.error === 'done' && existing.previewUrl) {
            URL.revokeObjectURL(existing.previewUrl);
          }
        }
        this.items = this.items.filter(i => i.error !== 'done');

        this.items.push(...items);
        // After the push: judged before it, a success-window drop of a new
        // folder would clear the category while the picker (never remounted
        // within one tick) still shows it.
        this._forgetCategoryWithoutFolders();
        this.state = 'preview';
        this.errorMessage = '';
        return;
      }

      // Fresh open – revoke any stale preview URLs
      for (const existing of this.items) {
        if (existing.previewUrl) URL.revokeObjectURL(existing.previewUrl);
      }
      this.items = items;
      this.context = context || null;
      this.tags = [];
      this.categoryId = null;
      this.seriesId = null;
      this.groupCategoryId = null;
      this.keepStructure = true;
      this._folderGroups = {};
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
      this.sessionId++;
      // Clear any pending auto-close timer
      if (_autoCloseTimer) {
        clearTimeout(_autoCloseTimer);
        _autoCloseTimer = null;
      }
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
      this.seriesId = null;
      this.groupCategoryId = null;
      this.keepStructure = true;
      this._folderGroups = {};
      this.dragActive = false;
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
      } else {
        this._forgetCategoryWithoutFolders();
      }
    },

    /** The picker is rebuilt once no folder is left, so the stored choice must go with it. */
    _forgetCategoryWithoutFolders() {
      if (!this.foldersApply()) this.groupCategoryId = null;
    },

    /** Folder identity of one item: the drop it came from plus its chain. */
    _folderKey(dropId, segments) {
      return JSON.stringify([dropId, ...segments]);
    },

    /** True when the batch holds dropped folders that could become groups. */
    foldersApply() {
      return this.context?.type === 'group' && this.items.some(i => i.dirPath?.length > 0);
    },

    /** True when the upload will create groups for the dropped folders. */
    structured() {
      return this.keepStructure && this.foldersApply();
    },

    /** How many groups the next upload would still create. */
    folderCount() {
      const needed = new Set();
      for (const item of this.items) {
        for (let n = 1; n <= (item.dirPath?.length || 0); n++) {
          const key = this._folderKey(item.dropId, item.dirPath.slice(0, n));
          if (!this._folderGroups[key]) needed.add(key);
        }
      }
      return needed.size;
    },

    folderSummary() {
      const n = this.folderCount();
      return n === 0 ? '' : `${n} group${n !== 1 ? 's' : ''} will be created`;
    },

    /**
     * Resolve (creating as needed) the group for a folder chain, parents first.
     * `failed` memoizes this run's failures so a broken folder is asked for once.
     * @returns {Promise<{ id: number }|{ error: string }>}
     */
    async _ensureFolderGroup(dropId, segments, failed) {
      let parentId = this.context.id;
      for (let n = 1; n <= segments.length; n++) {
        const key = this._folderKey(dropId, segments.slice(0, n));
        if (this._folderGroups[key]) {
          parentId = this._folderGroups[key];
          continue;
        }
        if (failed.has(key)) return { error: failed.get(key) };

        const name = segments[n - 1];
        const body = { Name: name, OwnerId: parentId };
        if (this.groupCategoryId) body.CategoryId = this.groupCategoryId;
        let message;
        try {
          const response = await fetch('/v1/group', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
            body: JSON.stringify(body),
          });
          if (response.ok) {
            const group = await response.json();
            this._folderGroups[key] = group.ID;
            parentId = group.ID;
            continue;
          }
          message = parseUploadError(await response.text(), response.status).message;
        } catch (err) {
          message = err.message || 'Network error';
        }
        message = `Could not create group "${name}": ${message}`;
        failed.set(key, message);
        return { error: message };
      }
      return { id: parentId };
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
      // Not while running, not during the success window (a second run would
      // overwrite the auto-close timer handle and the orphan would close a
      // later batch), and not with nothing left to send.
      if (this.state === 'uploading' || this.state === 'success') return;
      if (!this.items.some(i => i.error !== 'done')) return;

      this.state = 'uploading';
      this.errorMessage = '';

      const total = this.items.filter(i => i.error !== 'done').length;
      let successCount = 0;
      let current = 0;
      const structured = this.structured();
      const failedFolders = new Map();

      for (const item of this.items) {
        if (item.error === 'done') continue;

        current++;
        this.uploadProgress = `Uploading ${current} of ${total}...`;

        let ownerGroupId = this.context.id;
        if (structured && item.dirPath?.length) {
          const folder = await this._ensureFolderGroup(item.dropId, item.dirPath, failedFolders);
          if (folder.error) {
            item.error = folder.error;
            item.errorResourceId = null;
            continue;
          }
          ownerGroupId = folder.id;
        }

        const formData = new FormData();
        formData.append('resource', item.file, item.name);

        if (this.context.type === 'group') {
          formData.append('ownerId', ownerGroupId);
          formData.append('groups', ownerGroupId);
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

        if (this.seriesId) {
          formData.append('SeriesId', this.seriesId);
        }

        try {
          const response = await fetch('/v1/resource', {
            method: 'POST',
            body: formData,
          });
          if (!response.ok) {
            const text = await response.text();
            const parsed = parseUploadError(text, response.status);
            item.error = parsed.message;
            item.errorResourceId = parsed.resourceId;
          } else {
            item.error = 'done';
            item.errorResourceId = null;
            successCount++;
          }
        } catch (err) {
          item.error = err.message || 'Network error';
        }
      }

      if (successCount === total) {
        this.state = 'success';
        this.uploadProgress = `Uploaded ${successCount} file${successCount !== 1 ? 's' : ''} successfully.`;
        if (_autoCloseTimer) clearTimeout(_autoCloseTimer);
        _autoCloseTimer = setTimeout(() => {
          _autoCloseTimer = null;
          this.close();
          this._refreshPage();
        }, 1200);
      } else if (successCount > 0) {
        for (const item of this.items) {
          if (item.error === 'done' && item.previewUrl) {
            URL.revokeObjectURL(item.previewUrl);
          }
        }
        this.items = this.items.filter(i => i.error !== 'done');
        this._forgetCategoryWithoutFolders();
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
          window.Alpine.morph(main, newMain, morphOptionsWithShortcodeElements({
            updating(el, toEl, childrenOnly, skip) {
              if (el._x_dataStack) {
                toEl._x_dataStack = el._x_dataStack;
              }
            },
          }));
          window.Alpine?.store('lightbox')?.initFromDOM();
        }
      } catch (err) {
        console.error('Failed to refresh page after upload:', err);
        window.location.reload();
      }
    },
  });
}
