# Paste-to-Upload on Group and Note Detail Pages

## Overview

Add the ability to paste files, images, and text content directly onto group and note detail pages (and owner-filtered list pages) to create resources. A confirmation modal shows previews, allows per-item name editing, and supports shared metadata (tags, category) before upload.

## Architecture

### New Files

- **`src/components/pasteUpload.js`** — Alpine store (`$store.pasteUpload`) + global paste event listener
- **`templates/partials/pasteUpload.tpl`** — Modal template (included globally in base layout)

### Modified Files

- **`src/main.js`** — Import/register the store; replace existing paste handler
- **`templates/layouts/base.tpl`** — Include `pasteUpload.tpl` partial
- **`templates/displayGroup.tpl`** — Add `data-paste-context` attribute
- **`templates/displayNote.tpl`** — Add `data-paste-context` attribute
- **List templates** (`listGroups.tpl`, `listNotes.tpl`, `listResources.tpl`) — Add context when filtered by owner

### Flow

1. Global `paste` event listener intercepts paste events
2. If `<input type="file">` exists → existing behavior (merge files into input, visual ring)
3. If focus is in a text input/textarea/contenteditable → ignore (user is typing)
4. Check for `data-paste-context` attribute on a parent element
5. If context found → open paste upload modal with previews
6. If on a list page without owner filter → show info message
7. Upload via `fetch` to `POST /v1/resource` with multipart form data

No backend changes required — the existing `POST /v1/resource` endpoint accepts all needed fields.

## Paste Event Handling

### Content Detection Priority

1. `clipboardData.files` — actual files (images, PDFs, etc.)
2. `clipboardData.items` with `type: 'image/*'` — screenshots/copied images (via `.getAsFile()`)
3. `clipboardData.getData('text/html')` — rich text → saved as `.html` file
4. `clipboardData.getData('text/plain')` — plain text → saved as `.txt` file

### Guard Conditions (paste ignored when)

- `<input type="file">` exists on page (existing handler takes over)
- Focus is inside a text input, textarea, or `contenteditable` element
- No `data-paste-context` found and not on an owner-filtered list page

### File Naming

- Actual files: keep original filename
- Screenshot/image paste: `pasted-image-YYYY-MM-DDTHH-mm-ss.png`
- HTML paste: `pasted-content-YYYY-MM-DDTHH-mm-ss.html`
- Text paste: `pasted-text-YYYY-MM-DDTHH-mm-ss.txt`

### List Page Context

- Check URL query params for `owner`/`ownerId` parameter
- If present: fetch group/note name via API, populate modal context
- If absent: show info message — "Paste to upload requires filtering by an owner first"

## Modal UI

### Layout (follows entityPicker pattern)

- Fixed overlay with backdrop (`bg-black bg-opacity-50`)
- Centered card (`max-w-2xl`), focus-trapped, ESC to close
- Header: "Upload to [Entity Name]" with close button
- Body: item list + shared metadata
- Footer: Cancel / Upload buttons

### Item List (batch with per-item names)

Each pasted item shown as a row:

- **Preview column**: image thumbnail (~64px) for images, file icon for other files, text snippet for text/HTML
- **Name input**: editable text field, pre-filled with generated filename
- **Remove button**: X to remove individual items from batch
- Scrollable container (`max-h-60 overflow-y-auto`)

### Shared Metadata Section

- **Tags**: autocompleter dropdown (reuses existing component, `/v1/tags.json`)
- **Category**: autocompleter dropdown (`/v1/resource_categories.json`)
- Applied to all items in the batch

### States

- **Idle**: modal closed
- **Preview**: showing items, awaiting confirmation
- **Uploading**: progress indicator, buttons disabled
- **Success**: brief flash, modal closes, page refreshes
- **Error**: error message with retry for failed items

### Accessibility

- `role="dialog"`, `aria-modal="true"`, `aria-labelledby`
- Focus trap via `x-trap.noscroll` (Alpine focus plugin)
- ARIA live region for upload status
- ESC closes, Tab navigates

## Upload Flow

### Submission

- Build `FormData` per item, send sequentially to `POST /v1/resource`
- Each request includes: `resource` (file), `ownerId`, `tags[]`, `resourceCategoryId`
- For notes: also include `notes[]` with the note ID
- Progress updates after each upload ("Uploaded 2 of 5...")

### After Upload

- **Success**: close modal, refresh page via `fetch` + `Alpine.morph` (same pattern as `download-completed`)
- **Partial failure**: keep modal open, show which items failed, allow retry for failed items only
- **Total failure**: show error, keep all items for retry

### Note-Specific Behavior

- Resource created with note's `OwnerId` as owner (if present) AND note ID in `notes[]`
- If note has no owner, resource created without owner but still linked to the note

## Integration Points

### Paste Handler Replacement

The current global paste handler in `main.js` (lines 114-131) is replaced by the store's listener. The new listener checks for `<input type="file">` first and reproduces existing behavior — no regression.

### `data-paste-context` Attribute

**`displayGroup.tpl`**:
```html
<div data-paste-context='{"type":"group","id":{{group.ID}},"name":"{{group.Name}}"}'>
```

**`displayNote.tpl`**:
```html
<div data-paste-context='{"type":"note","id":{{note.ID}},"ownerId":{{note.OwnerId}},"name":"{{note.Name}}"}'>
```

**List templates**: rendered server-side only when `query.OwnerId` is present.

### Template Inclusion

`pasteUpload.tpl` included in `base.tpl` alongside lightbox partial.
