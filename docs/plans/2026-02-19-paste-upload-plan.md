# Paste-to-Upload Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to paste files, images, and text onto group/note detail pages (and owner-filtered list pages) to create resources, with a confirmation modal showing previews and metadata options.

**Architecture:** New Alpine store (`$store.pasteUpload`) handles paste detection, content extraction, and upload state. A globally-included modal template (`pasteUpload.tpl`) renders the UI. Templates annotate pages with `data-paste-context` attributes so the JS knows which entity to associate uploads with.

**Tech Stack:** Alpine.js (store + focus plugin), Tailwind CSS, Pongo2 templates, existing `POST /v1/resource` API

---

### Task 1: Alpine Store — Core State and Content Extraction

**Files:**
- Create: `src/components/pasteUpload.js`

**Step 1: Create the store module with state, open/close, and content extraction**

```js
/**
 * Register the pasteUpload Alpine store.
 * @param {import('alpinejs').Alpine} Alpine
 */
export function registerPasteUploadStore(Alpine) {
  Alpine.store('pasteUpload', {
    // State
    isOpen: false,
    items: [],        // Array of { file: File, name: string, previewUrl: string|null, type: 'file'|'image'|'text', error: string|null }
    context: null,    // { type: 'group'|'note', id: number, ownerId?: number, name: string }
    tags: [],         // Selected tag IDs
    categoryId: null, // Selected resource category ID
    state: 'idle',    // 'idle' | 'preview' | 'uploading' | 'success' | 'error'
    uploadProgress: '', // e.g. "Uploaded 2 of 5..."
    errorMessage: '',
    infoMessage: '',  // For "filter by owner first" toast

    open(items, context) {
      this.items = items;
      this.context = context;
      this.tags = [];
      this.categoryId = null;
      this.state = 'preview';
      this.uploadProgress = '';
      this.errorMessage = '';
      this.isOpen = true;
    },

    close() {
      // Revoke object URLs to prevent memory leaks
      for (const item of this.items) {
        if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
      }
      this.isOpen = false;
      this.items = [];
      this.context = null;
      this.state = 'idle';
      this.uploadProgress = '';
      this.errorMessage = '';
    },

    removeItem(index) {
      const item = this.items[index];
      if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
      this.items.splice(index, 1);
      if (this.items.length === 0) this.close();
    },

    showInfo(message) {
      this.infoMessage = message;
      setTimeout(() => { this.infoMessage = ''; }, 4000);
    },
  });
}

/**
 * Generate a timestamped filename.
 */
function timestampedName(prefix, ext) {
  const now = new Date();
  const ts = now.toISOString().replace(/[:.]/g, '-').slice(0, 19);
  return `${prefix}-${ts}.${ext}`;
}

/**
 * Extract pasteable content from a ClipboardEvent.
 * Returns an array of { file, name, previewUrl, type } or empty array.
 */
export function extractPasteContent(clipboardData) {
  const items = [];

  // Priority 1: Actual files
  if (clipboardData.files && clipboardData.files.length > 0) {
    for (const file of clipboardData.files) {
      const isImage = file.type.startsWith('image/');
      items.push({
        file,
        name: file.name || timestampedName('pasted-image', file.type.split('/')[1] || 'png'),
        previewUrl: isImage ? URL.createObjectURL(file) : null,
        type: isImage ? 'image' : 'file',
        error: null,
      });
    }
    return items;
  }

  // Priority 2: Image items (screenshots)
  for (const item of clipboardData.items) {
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile();
      if (file) {
        items.push({
          file,
          name: timestampedName('pasted-image', item.type.split('/')[1] || 'png'),
          previewUrl: URL.createObjectURL(file),
          type: 'image',
          error: null,
        });
      }
    }
  }
  if (items.length > 0) return items;

  // Priority 3: HTML content
  const html = clipboardData.getData('text/html');
  if (html && html.trim()) {
    const blob = new Blob([html], { type: 'text/html' });
    const file = new File([blob], timestampedName('pasted-content', 'html'), { type: 'text/html' });
    items.push({
      file,
      name: file.name,
      previewUrl: null,
      type: 'text',
      _snippet: html.replace(/<[^>]*>/g, '').slice(0, 200),
      error: null,
    });
    return items;
  }

  // Priority 4: Plain text
  const text = clipboardData.getData('text/plain');
  if (text && text.trim()) {
    const blob = new Blob([text], { type: 'text/plain' });
    const file = new File([blob], timestampedName('pasted-text', 'txt'), { type: 'text/plain' });
    items.push({
      file,
      name: file.name,
      previewUrl: null,
      type: 'text',
      _snippet: text.slice(0, 200),
      error: null,
    });
    return items;
  }

  return [];
}
```

**Step 2: Run the JS build to verify no syntax errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds (the module isn't imported yet, but Vite can validate it exists)

**Step 3: Commit**

```bash
git add src/components/pasteUpload.js
git commit -m "feat(paste-upload): add Alpine store with state management and content extraction"
```

---

### Task 2: Upload Logic in the Store

**Files:**
- Modify: `src/components/pasteUpload.js`

**Step 1: Add the upload method to the store object, after `showInfo`**

```js
    async upload() {
      if (this.items.length === 0 || !this.context) return;

      this.state = 'uploading';
      this.errorMessage = '';
      let successCount = 0;
      const total = this.items.length;

      for (let i = 0; i < this.items.length; i++) {
        const item = this.items[i];
        if (item.error === 'done') continue; // Already uploaded in a retry

        this.uploadProgress = `Uploading ${i + 1} of ${total}...`;

        const formData = new FormData();
        formData.append('resource', item.file, item.name);

        // Set owner: for groups, the group IS the owner; for notes, use note's ownerId
        const ownerId = this.context.type === 'group'
          ? this.context.id
          : (this.context.ownerId || null);
        if (ownerId) formData.append('ownerId', ownerId);

        // For notes, also link via many-to-many
        if (this.context.type === 'note') {
          formData.append('notes', this.context.id);
        }
        // For groups, also link via many-to-many (groups field)
        if (this.context.type === 'group') {
          formData.append('groups', this.context.id);
        }

        // Shared metadata
        for (const tagId of this.tags) {
          formData.append('tags', tagId);
        }
        if (this.categoryId) {
          formData.append('resourceCategoryId', this.categoryId);
        }

        try {
          const resp = await fetch('/v1/resource', { method: 'POST', body: formData });
          if (!resp.ok) {
            const text = await resp.text();
            item.error = text || `HTTP ${resp.status}`;
            continue;
          }
          item.error = 'done';
          successCount++;
        } catch (err) {
          item.error = err.message || 'Network error';
        }
      }

      if (successCount === total) {
        this.state = 'success';
        this.uploadProgress = `Uploaded ${total} file${total > 1 ? 's' : ''} successfully`;
        setTimeout(() => {
          this.close();
          this._refreshPage();
        }, 800);
      } else if (successCount > 0) {
        // Partial failure — remove successful items, keep failed ones
        this.items = this.items.filter(item => item.error !== 'done');
        this.state = 'error';
        this.errorMessage = `${successCount} of ${total} uploaded. ${this.items.length} failed — retry or cancel.`;
      } else {
        this.state = 'error';
        this.errorMessage = 'All uploads failed. Check your connection and retry.';
      }
    },

    async _refreshPage() {
      try {
        const response = await fetch(window.location.href, { headers: { 'Accept': 'text/html' } });
        const html = await response.text();
        const parser = new DOMParser();
        const doc = parser.parseFromString(html, 'text/html');

        // Morph main content area
        const main = document.querySelector('.main');
        const newMain = doc.querySelector('.main');
        if (main && newMain && window.Alpine) {
          window.Alpine.morph(main, newMain, {
            updating(el, toEl) {
              if (el._x_dataStack) toEl._x_dataStack = el._x_dataStack;
            }
          });
        }

        // Re-init lightbox for new images
        if (window.Alpine?.store('lightbox')?.initFromDOM) {
          window.Alpine.store('lightbox').initFromDOM();
        }
      } catch (err) {
        // Fall back to full page reload
        window.location.reload();
      }
    },
```

**Step 2: Run JS build**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add src/components/pasteUpload.js
git commit -m "feat(paste-upload): add sequential upload logic with progress and error handling"
```

---

### Task 3: Global Paste Event Listener

**Files:**
- Modify: `src/components/pasteUpload.js` (add `setupPasteListener` export)
- Modify: `src/main.js:72` (import + register store), `src/main.js:113-131` (replace old paste handler)

**Step 1: Add `setupPasteListener` at the bottom of `pasteUpload.js`**

```js
/**
 * Setup the global paste event listener.
 * Replaces the old file-input-only paste handler from main.js.
 */
export function setupPasteListener() {
  window.addEventListener('paste', (e) => {
    // Guard 1: If a file input exists, use the old behavior (resource create form)
    const fileInput = document.querySelector("input[type='file']");
    if (fileInput && e.clipboardData.files && e.clipboardData.files.length) {
      e.preventDefault();
      const dt = new DataTransfer();
      for (const file of fileInput.files) dt.items.add(file);
      for (const file of e.clipboardData.files) dt.items.add(file);
      fileInput.files = dt.files;
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
      fileInput.closest('.flex')?.classList.add('ring-2', 'ring-indigo-500', 'rounded-md');
      setTimeout(() => fileInput.closest('.flex')?.classList.remove('ring-2', 'ring-indigo-500', 'rounded-md'), 1500);
      return;
    }

    // Guard 2: If focus is in a text input, textarea, or contenteditable, ignore
    const active = document.activeElement;
    if (active) {
      const tag = active.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || active.isContentEditable) return;
    }

    // Extract content from clipboard
    const items = extractPasteContent(e.clipboardData);
    if (items.length === 0) return;

    e.preventDefault();

    const store = window.Alpine?.store('pasteUpload');
    if (!store) return;

    // Check for data-paste-context on the page
    const contextEl = document.querySelector('[data-paste-context]');
    if (contextEl) {
      try {
        const context = JSON.parse(contextEl.getAttribute('data-paste-context'));
        store.open(items, context);
        return;
      } catch { /* fall through */ }
    }

    // Check for owner filter in URL (list pages)
    const params = new URLSearchParams(window.location.search);
    const ownerId = params.get('ownerId');
    if (ownerId) {
      // Fetch group name for context
      fetch(`/v1/group.json?id=${encodeURIComponent(ownerId)}`)
        .then(r => r.ok ? r.json() : null)
        .then(group => {
          if (group) {
            store.open(items, { type: 'group', id: group.ID, name: group.Name });
          } else {
            store.showInfo('Could not find owner group. Try filtering by owner first.');
            for (const item of items) { if (item.previewUrl) URL.revokeObjectURL(item.previewUrl); }
          }
        })
        .catch(() => {
          store.showInfo('Could not fetch owner info.');
          for (const item of items) { if (item.previewUrl) URL.revokeObjectURL(item.previewUrl); }
        });
      return;
    }

    // No context and no owner filter — show info message
    store.showInfo('To paste and upload, navigate to a group or note detail page, or filter a list by owner.');
    for (const item of items) { if (item.previewUrl) URL.revokeObjectURL(item.previewUrl); }
  });
}
```

**Step 2: Update `src/main.js` — add import and register store**

Add this import after line 32 (after the `registerEntityPickerStore` import):

```js
import { registerPasteUploadStore, setupPasteListener } from './components/pasteUpload.js';
```

Add store registration after line 72 (after `registerEntityPickerStore(Alpine);`):

```js
registerPasteUploadStore(Alpine);
```

**Step 3: Update `src/main.js` — replace old paste handler and call new setup**

Delete lines 113-131 (the old `window.addEventListener('paste', ...)` block).

Add after `setupBulkSelectionListeners();` (was line 134):

```js
setupPasteListener();
```

**Step 4: Run JS build**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add src/components/pasteUpload.js src/main.js
git commit -m "feat(paste-upload): add global paste listener with guard conditions and context detection"
```

---

### Task 4: Modal Template

**Files:**
- Create: `templates/partials/pasteUpload.tpl`
- Modify: `templates/layouts/base.tpl:66` (add include)

**Step 1: Create the modal template**

Create `templates/partials/pasteUpload.tpl` with this content:

```html
{# Paste Upload Modal #}
<div x-show="$store.pasteUpload.isOpen"
     x-cloak
     class="fixed inset-0 z-50 overflow-y-auto"
     role="dialog"
     aria-modal="true"
     aria-labelledby="paste-upload-title"
     @keydown.escape.window="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()">
    {# Backdrop #}
    <div class="fixed inset-0 bg-black bg-opacity-50 transition-opacity"
         tabindex="-1"
         @click="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()"></div>

    {# Modal content #}
    <div class="flex min-h-full items-center justify-center p-4">
        <div class="relative bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[80vh] flex flex-col"
             @click.stop
             x-trap.noscroll="$store.pasteUpload.isOpen">
            {# Header #}
            <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200">
                <h2 id="paste-upload-title" class="text-lg font-semibold text-gray-900">
                    Upload to <span x-text="$store.pasteUpload.context?.name || 'Unknown'"></span>
                </h2>
                <button @click="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()"
                        class="text-gray-400 hover:text-gray-600"
                        :disabled="$store.pasteUpload.state === 'uploading'"
                        aria-label="Close">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                </button>
            </div>

            {# Item list #}
            <div class="flex-1 overflow-y-auto p-4 space-y-3">
                {# ARIA live region #}
                <span class="sr-only" aria-live="polite" aria-atomic="true"
                      x-text="$store.pasteUpload.uploadProgress || ($store.pasteUpload.items.length + ' items ready to upload')"></span>

                {# Error message #}
                <div x-show="$store.pasteUpload.errorMessage"
                     class="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700"
                     x-text="$store.pasteUpload.errorMessage"></div>

                {# Items #}
                <template x-for="(item, index) in $store.pasteUpload.items" :key="index">
                    <div class="flex items-center gap-3 p-2 border border-gray-200 rounded-lg"
                         :class="{ 'border-red-300 bg-red-50': item.error && item.error !== 'done' }">
                        {# Preview #}
                        <div class="w-16 h-16 flex-shrink-0 bg-gray-100 rounded overflow-hidden flex items-center justify-center">
                            <template x-if="item.type === 'image' && item.previewUrl">
                                <img :src="item.previewUrl" class="w-full h-full object-cover" alt="Preview">
                            </template>
                            <template x-if="item.type === 'file'">
                                <svg class="w-8 h-8 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
                                </svg>
                            </template>
                            <template x-if="item.type === 'text'">
                                <div class="p-1 text-xs text-gray-500 leading-tight overflow-hidden max-h-full"
                                     x-text="item._snippet || 'Text content'"></div>
                            </template>
                        </div>

                        {# Name input #}
                        <div class="flex-1 min-w-0">
                            <input type="text"
                                   x-model="item.name"
                                   class="w-full text-sm border border-gray-300 rounded px-2 py-1 focus:ring-indigo-500 focus:border-indigo-500"
                                   :disabled="$store.pasteUpload.state === 'uploading'"
                                   :aria-label="'Filename for item ' + (index + 1)">
                            <p x-show="item.error && item.error !== 'done'"
                               class="text-xs text-red-600 mt-1"
                               x-text="item.error"></p>
                        </div>

                        {# Remove button #}
                        <button @click="$store.pasteUpload.removeItem(index)"
                                :disabled="$store.pasteUpload.state === 'uploading'"
                                class="text-gray-400 hover:text-red-500 flex-shrink-0"
                                :aria-label="'Remove item ' + (index + 1)">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                            </svg>
                        </button>
                    </div>
                </template>

                {# Shared metadata #}
                <div x-show="$store.pasteUpload.state !== 'uploading'" class="border-t border-gray-200 pt-3 mt-3 space-y-3">
                    <div x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/tags',
                             elName: 'paste-tags',
                             addUrl: '/v1/tag',
                         })"
                         x-effect="$store.pasteUpload.tags = selectedResults.map(r => r.ID)">
                        <label class="block text-sm font-medium text-gray-700">Tags</label>
                        <input x-ref="autocompleter"
                               type="text"
                               class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
                               x-bind="inputEvents"
                               autocomplete="off"
                               placeholder="Add tags...">
                        {% include "/partials/form/formParts/dropDownResults.tpl" with action="pushVal" %}
                        {% include "/partials/form/formParts/dropDownSelectedResults.tpl" %}
                    </div>

                    <div x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/resourceCategories',
                             elName: 'paste-category',
                             max: 1,
                         })"
                         x-effect="$store.pasteUpload.categoryId = selectedResults[0]?.ID || null">
                        <label class="block text-sm font-medium text-gray-700">Resource Category</label>
                        <input x-ref="autocompleter"
                               type="text"
                               class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
                               x-bind="inputEvents"
                               autocomplete="off"
                               placeholder="Select category...">
                        {% include "/partials/form/formParts/dropDownResults.tpl" with action="pushVal" %}
                        {% include "/partials/form/formParts/dropDownSelectedResults.tpl" %}
                    </div>
                </div>
            </div>

            {# Footer #}
            <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
                <span class="text-sm text-gray-600">
                    <template x-if="$store.pasteUpload.state === 'uploading'">
                        <span x-text="$store.pasteUpload.uploadProgress"></span>
                    </template>
                    <template x-if="$store.pasteUpload.state === 'success'">
                        <span class="text-green-600" x-text="$store.pasteUpload.uploadProgress"></span>
                    </template>
                    <template x-if="$store.pasteUpload.state === 'preview'">
                        <span x-text="$store.pasteUpload.items.length + ' item' + ($store.pasteUpload.items.length > 1 ? 's' : '')"></span>
                    </template>
                </span>
                <div class="flex gap-2">
                    <button @click="$store.pasteUpload.close()"
                            type="button"
                            :disabled="$store.pasteUpload.state === 'uploading'"
                            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50">
                        Cancel
                    </button>
                    <button @click="$store.pasteUpload.upload()"
                            type="button"
                            :disabled="$store.pasteUpload.state === 'uploading' || $store.pasteUpload.items.length === 0"
                            class="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
                        <span x-show="$store.pasteUpload.state !== 'uploading'">Upload</span>
                        <span x-show="$store.pasteUpload.state === 'uploading'" class="flex items-center gap-2">
                            <svg class="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                            Uploading...
                        </span>
                    </button>
                </div>
            </div>
        </div>
    </div>
</div>

{# Info toast (for "filter by owner first" message) #}
<div x-show="$store.pasteUpload.infoMessage"
     x-cloak
     x-transition:enter="transition ease-out duration-200"
     x-transition:enter-start="opacity-0 translate-y-2"
     x-transition:enter-end="opacity-100 translate-y-0"
     x-transition:leave="transition ease-in duration-150"
     x-transition:leave-start="opacity-100 translate-y-0"
     x-transition:leave-end="opacity-0 translate-y-2"
     class="fixed bottom-20 left-1/2 -translate-x-1/2 z-50 px-4 py-3 bg-gray-800 text-white text-sm rounded-lg shadow-lg max-w-md text-center"
     role="status"
     aria-live="polite"
     x-text="$store.pasteUpload.infoMessage">
</div>
```

**Step 2: Add the include in `templates/layouts/base.tpl`**

After line 66 (`{% include "/partials/lightbox.tpl" %}`), add:

```html
    {% include "/partials/pasteUpload.tpl" %}
```

**Step 3: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Full build succeeds (Go binary + CSS + JS)

**Step 4: Commit**

```bash
git add templates/partials/pasteUpload.tpl templates/layouts/base.tpl
git commit -m "feat(paste-upload): add modal template with item preview, metadata, and info toast"
```

---

### Task 5: Add `data-paste-context` to Detail Templates

**Files:**
- Modify: `templates/displayGroup.tpl:3-4`
- Modify: `templates/displayNote.tpl:3-4`

**Step 1: Add context attribute to `displayGroup.tpl`**

Change line 3-4 from:

```html
{% block body %}
    <div x-data="{ entity: {{ group|json }} }">
```

To:

```html
{% block body %}
    <div x-data="{ entity: {{ group|json }} }" data-paste-context='{{ pasteContext|json }}'>
```

Wait — the `pasteContext` variable doesn't exist yet on the server side. Instead, construct it inline in the template. Change to:

```html
{% block body %}
    <div x-data="{ entity: {{ group|json }} }" data-paste-context='{"type":"group","id":{{ group.ID }},"name":"{{ group.Name|escapejs }}"}'>
```

**Step 2: Add context attribute to `displayNote.tpl`**

Change line 3-4 from:

```html
{% block body %}
    <div x-data="{ entity: {{ note|json }} }">
```

To:

```html
{% block body %}
    <div x-data="{ entity: {{ note|json }} }" data-paste-context='{"type":"note","id":{{ note.ID }},"ownerId":{{ note.OwnerId }},"name":"{{ note.Name|escapejs }}"}'>
```

Note: `note.OwnerId` is a `uint` in Go and will render as `0` if there's no owner, which is fine (the JS treats `0` as falsy).

**Step 3: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds

**Step 4: Manual smoke test**

Run: `cd /Users/egecan/Code/mahresources && ./mahresources -ephemeral -bind-address=:8181`

1. Navigate to a group detail page
2. Verify the `data-paste-context` attribute is rendered in the DOM
3. Copy an image, paste on the page — modal should appear
4. Verify items show with preview, name input, remove button
5. Cancel — modal closes

**Step 5: Commit**

```bash
git add templates/displayGroup.tpl templates/displayNote.tpl
git commit -m "feat(paste-upload): add data-paste-context to group and note detail templates"
```

---

### Task 6: Add `data-paste-context` to List Templates

**Files:**
- Modify: `templates/listGroups.tpl:7-9` (the `{% block body %}` area)
- Modify: `templates/listNotes.tpl:3` (the `{% block gallery %}` area)
- Modify: `templates/listResources.tpl:8` (the `{% block body %}` area)

The list templates use `owners` (array of Group models) from the server. When an owner filter is active, `owners` contains the selected owner group. We use this to render the attribute server-side.

**Step 1: Modify `listGroups.tpl`**

The body block starts at line 7. Wrap the items container:

Change:

```html
{% block body %}
    <div class="flex flex-col gap-4 items-container">
```

To:

```html
{% block body %}
    <div class="flex flex-col gap-4 items-container"{% if owners && owners|length == 1 %} data-paste-context='{"type":"group","id":{{ owners.0.ID }},"name":"{{ owners.0.Name|escapejs }}"}'{% endif %}>
```

**Step 2: Modify `listNotes.tpl`**

The `listNotes.tpl` extends `layouts/gallery.tpl` instead of `base.tpl`. We need to check that layout. The gallery block wraps notes. Add context to the gallery block wrapper.

Change:

```html
{% block gallery %}
    {% for entity in notes %}
```

To:

```html
{% block gallery %}
    <div{% if owners && owners|length == 1 %} data-paste-context='{"type":"group","id":{{ owners.0.ID }},"name":"{{ owners.0.Name|escapejs }}"}'{% endif %}>
    {% for entity in notes %}
```

And close the div after the `{% endfor %}`:

```html
    {% endfor %}
    </div>
{% endblock %}
```

**Step 3: Modify `listResources.tpl`**

Change:

```html
{% block body %}
    <section class="list-container">
```

To:

```html
{% block body %}
    <section class="list-container"{% if owner && owner|length == 1 %} data-paste-context='{"type":"group","id":{{ owner.0.ID }},"name":"{{ owner.0.Name|escapejs }}"}'{% endif %}>
```

Note: `listResources.tpl` uses `owner` (singular) via `searchFormResource.tpl`, not `owners` (plural). Check the template context provider to confirm. The resource list page passes `owner` as `selectedItems=owner` for the autocompleter.

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add templates/listGroups.tpl templates/listNotes.tpl templates/listResources.tpl
git commit -m "feat(paste-upload): add data-paste-context to owner-filtered list pages"
```

---

### Task 7: E2E Test — Paste Upload on Group Detail

**Files:**
- Create: `e2e/tests/paste-upload.spec.ts`

**Step 1: Write E2E test**

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Paste Upload', () => {
  test('should open modal when pasting an image on group detail page', async ({ groupPage, page }) => {
    // Create a group to work with
    const groupName = `paste-test-group-${Date.now()}`;
    await groupPage.create({ name: groupName });

    // Verify we're on the detail page and context attribute exists
    const contextEl = page.locator('[data-paste-context]');
    await expect(contextEl).toBeAttached();

    // Simulate paste with a file
    // Create a small PNG in memory
    const buffer = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
      'base64'
    );

    // Dispatch a paste event with a file via JS
    await page.evaluate(async (data) => {
      const arr = new Uint8Array(data);
      const blob = new Blob([arr], { type: 'image/png' });
      const file = new File([blob], 'test-image.png', { type: 'image/png' });
      const dt = new DataTransfer();
      dt.items.add(file);
      const event = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true });
      document.dispatchEvent(event);
    }, Array.from(buffer));

    // Modal should appear
    const modal = page.locator('[role="dialog"][aria-labelledby="paste-upload-title"]');
    await expect(modal).toBeVisible({ timeout: 3000 });

    // Header should show group name
    await expect(modal.locator('#paste-upload-title')).toContainText(groupName);

    // Should have 1 item row
    const items = modal.locator('input[aria-label^="Filename"]');
    await expect(items).toHaveCount(1);

    // Cancel should close modal
    await modal.getByRole('button', { name: 'Cancel' }).click();
    await expect(modal).not.toBeVisible();
  });

  test('should upload pasted file to group', async ({ groupPage, page, request }) => {
    const groupName = `paste-upload-${Date.now()}`;
    await groupPage.create({ name: groupName });

    // Get the group ID from the context
    const contextStr = await page.locator('[data-paste-context]').getAttribute('data-paste-context');
    const context = JSON.parse(contextStr!);

    // Simulate paste
    const buffer = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
      'base64'
    );
    await page.evaluate(async (data) => {
      const arr = new Uint8Array(data);
      const blob = new Blob([arr], { type: 'image/png' });
      const file = new File([blob], 'test-image.png', { type: 'image/png' });
      const dt = new DataTransfer();
      dt.items.add(file);
      document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true }));
    }, Array.from(buffer));

    // Modal should appear
    const modal = page.locator('[role="dialog"][aria-labelledby="paste-upload-title"]');
    await expect(modal).toBeVisible({ timeout: 3000 });

    // Click Upload
    await modal.getByRole('button', { name: 'Upload' }).click();

    // Modal should close after successful upload
    await expect(modal).not.toBeVisible({ timeout: 10000 });

    // Verify resource was created and owned by this group
    const resp = await request.get(`/v1/resources.json?ownerId=${context.id}`);
    expect(resp.ok()).toBeTruthy();
    const resources = await resp.json();
    expect(resources.length).toBeGreaterThanOrEqual(1);
  });

  test('should show info toast when pasting on list page without owner filter', async ({ page }) => {
    await page.goto('/groups');

    const buffer = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
      'base64'
    );
    await page.evaluate(async (data) => {
      const arr = new Uint8Array(data);
      const blob = new Blob([arr], { type: 'image/png' });
      const file = new File([blob], 'test.png', { type: 'image/png' });
      const dt = new DataTransfer();
      dt.items.add(file);
      document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true }));
    }, Array.from(buffer));

    // Should show info toast, NOT the modal
    const toast = page.locator('[role="status"]');
    await expect(toast).toBeVisible({ timeout: 3000 });
    await expect(toast).toContainText('filter');
  });

  test('should not intercept paste in text inputs', async ({ groupPage, page }) => {
    await groupPage.create({ name: `input-test-${Date.now()}` });

    // Focus a text input (e.g. the autocompleter in sidebar)
    const input = page.locator('input[type="text"]').first();
    await input.focus();

    // Paste — should NOT trigger the modal
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.items.add(new File([new Blob(['test'])], 'test.png', { type: 'image/png' }));
      document.activeElement?.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true }));
    });

    const modal = page.locator('[role="dialog"][aria-labelledby="paste-upload-title"]');
    // Brief wait to make sure modal does NOT appear
    await page.waitForTimeout(500);
    await expect(modal).not.toBeVisible();
  });
});
```

**Step 2: Run E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server -- --grep "Paste Upload"`
Expected: Tests pass (adjust as needed based on actual fixtures/page objects)

**Step 3: Commit**

```bash
git add e2e/tests/paste-upload.spec.ts
git commit -m "test(paste-upload): add E2E tests for paste upload on group detail and list pages"
```

---

### Task 8: Final Integration Test and Cleanup

**Files:**
- Verify all modified files

**Step 1: Run full JS build**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds

**Step 2: Run Go tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./...`
Expected: All tests pass (no backend changes were made)

**Step 3: Run full E2E suite**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server`
Expected: All tests pass including new paste-upload tests

**Step 4: Manual smoke test checklist**

Start ephemeral server: `./mahresources -ephemeral -bind-address=:8181`

- [ ] Create a group, navigate to its detail page
- [ ] Paste an image — modal appears with preview thumbnail
- [ ] Edit the filename in the modal
- [ ] Add a tag via the autocompleter
- [ ] Click Upload — resource created, page refreshes, resource visible in "Own Entities > Resources"
- [ ] Navigate to a note detail page, paste text — modal shows with text snippet preview
- [ ] Upload — resource created and linked to note
- [ ] Navigate to `/groups` list without owner filter, paste — info toast appears
- [ ] Filter groups by owner, paste — modal appears with owner's name
- [ ] On resource creation page (`/resource/new`), paste — old behavior (files merge into input, ring feedback)
- [ ] Focus a text input on any page, paste — no modal interference

**Step 5: Commit any fixes from smoke testing**

```bash
git add -A
git commit -m "fix(paste-upload): address issues found during smoke testing"
```

(Only if fixes were needed.)
