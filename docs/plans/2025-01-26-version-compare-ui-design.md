# Version Compare UI Design

## Overview

A dedicated comparison page for comparing resource versions - both within the same resource and across different resources. Supports visual image comparison, text diffs, PDF viewing, and metadata comparison.

## URL Structure

**Route:** `GET /resource/compare`

**Query parameters:**
- `r1` (required) - First resource ID
- `v1` (optional) - Version number for r1, defaults to current
- `r2` (optional) - Second resource ID, defaults to r1 (same-resource comparison)
- `v2` (required) - Version number for r2

**Examples:**
```
/resource/compare?r1=123&v1=1&v2=3           # Same resource, v1 vs v3
/resource/compare?r1=123&v1=2&r2=456&v2=1    # Cross-resource comparison
```

## Page Layout

```
┌─────────────────────────────────────────────────────────────┐
│ Compare Resources                                           │
├─────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────┐  ┌─────────────────────────────┐│
│ │ Resource: [autocomplete]│  │ Resource: [autocomplete]    ││
│ │ Version:  [select v1 ▼] │  │ Version:  [select v3 ▼]     ││
│ └─────────────────────────┘  └─────────────────────────────┘│
├─────────────────────────────────────────────────────────────┤
│ METADATA COMPARISON                                         │
│ ┌───────────────┬─────────────────┬─────────────────┬──────┐│
│ │ Property      │ Left            │ Right           │Status││
│ ├───────────────┼─────────────────┼─────────────────┼──────┤│
│ │ Content Type  │ image/png       │ image/png       │  =   ││
│ │ File Size     │ 2.4 MB          │ 1.8 MB          │ -25% ││
│ │ Dimensions    │ 1920×1080       │ 1920×1080       │  =   ││
│ │ Hash Match    │                 │                 │  ✗   ││
│ │ Created       │ Jan 15, 2025    │ Jan 20, 2025    │      ││
│ │ Comment       │ "Original"      │ "Compressed"    │      ││
│ │ Resource      │ [Link: photo..]→│ [Link: photo..]→│      ││
│ │ Owner         │ Vacation 2024   │ Vacation 2024   │  =   ││
│ └───────────────┴─────────────────┴─────────────────┴──────┘│
├─────────────────────────────────────────────────────────────┤
│ CONTENT COMPARISON (varies by type)                         │
└─────────────────────────────────────────────────────────────┘
```

**Metadata fields:**
- Content type, file size (with delta %), dimensions (if applicable)
- Hash match indicator (identical content = green checkmark)
- Perceptual hash similarity for images (percentage if available)
- Created date, version comment
- Links to parent resource detail pages
- Owner/group (highlighted if different between resources)

## Image Comparison Modes

**Mode selector toolbar:**
```
┌─────────────────────────────────────────────────────────────┐
│ [Side-by-side] [Slider] [Onion skin] [Toggle]              │
├─────────────────────────────────────────────────────────────┤
│ (comparison view based on selected mode)                    │
└─────────────────────────────────────────────────────────────┘
```

### Side-by-side
- Two images displayed next to each other
- Synchronized zoom/pan (drag one, both move)
- Zoom controls: fit, 100%, zoom in/out buttons
- Images scale down to fit viewport, maintain aspect ratio

### Slider
- Images overlaid, draggable vertical divider
- Left of divider shows image 1, right shows image 2
- Divider has visible handle for dragging

### Onion skin
- Images overlaid with opacity slider (0-100%)
- 0% = only left image, 100% = only right image
- Slider defaults to 50%

### Toggle
- Single image displayed, click/tap to switch
- Visual indicator showing which version is displayed (v1/v2 badge)
- Keyboard shortcut: spacebar to toggle

### Shared controls
- All modes support zoom/pan where applicable
- "Swap sides" button to flip left/right assignment

## Text Diff Modes

For files with `text/*` MIME types.

**Mode selector:**
```
┌─────────────────────────────────────────────────────────────┐
│ [Unified] [Side-by-side]                    Lines: 42 changed│
├─────────────────────────────────────────────────────────────┤
```

### Unified diff view
```
  10 │ function processData(input) {
  11 │   const result = [];
- 12 │   for (let i = 0; i < input.length; i++) {
+ 12 │   for (const item of input) {
- 13 │     result.push(transform(input[i]));
+ 13 │     result.push(transform(item));
  14 │   }
  15 │   return result;
```
- Line numbers from original file
- Red background for removed lines (prefixed with `-`)
- Green background for added lines (prefixed with `+`)
- Unchanged context lines in neutral color

### Side-by-side view
```
│ Left (v1)                    │ Right (v3)                   │
├──────────────────────────────┼──────────────────────────────┤
│ 12 │ for (let i = 0; ...     │ 12 │ for (const item of ...  │
│ 13 │ result.push(trans...    │ 13 │ result.push(trans...    │
```
- Synchronized scrolling between panes
- Matching line highlighting on hover
- Word-level diff highlighting within changed lines

### Stats
- Lines added/removed/changed count displayed in header

## PDF Comparison

```
┌─────────────────────────────────────────────────────────────┐
│ PDF COMPARISON                          [Load in viewer]    │
├─────────────────────────────────────────────────────────────┤
│ (before clicking "Load in viewer")                          │
│                                                             │
│ ┌─────────────────────────┐  ┌─────────────────────────────┐│
│ │ ┌─────┐                 │  │ ┌─────┐                     ││
│ │ │ PDF │  document.pdf   │  │ │ PDF │  document.pdf       ││
│ │ └─────┘  v1 • 2.4 MB    │  │ └─────┘  v3 • 2.1 MB        ││
│ │ [Download]              │  │ [Download]                  ││
│ └─────────────────────────┘  └─────────────────────────────┘│
├─────────────────────────────────────────────────────────────┤
│ (after clicking "Load in viewer")                           │
│                                                             │
│ ┌─────────────────────────┐  ┌─────────────────────────────┐│
│ │ ┌─────────────────────┐ │  │ ┌─────────────────────────┐ ││
│ │ │                     │ │  │ │                         │ ││
│ │ │   PDF in iframe     │ │  │ │   PDF in iframe         │ ││
│ │ │   (browser viewer)  │ │  │ │   (browser viewer)      │ ││
│ │ │                     │ │  │ │                         │ ││
│ │ └─────────────────────┘ │  │ └─────────────────────────┘ ││
│ │ [Download]              │  │ [Download]                  ││
│ └─────────────────────────┘  └─────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

- PDFs load on-demand (click "Load in viewer") to avoid slow page loads
- Side-by-side iframes using browser's native PDF renderer
- Download buttons remain available below each iframe
- Each iframe independently scrollable/zoomable

## Binary File Fallback

For non-text, non-image, non-PDF files:

```
┌─────────────────────────────────────────────────────────────┐
│ CONTENT PREVIEW NOT AVAILABLE                               │
│                                                             │
│ These files cannot be compared visually. Use the download   │
│ links to compare them locally.                              │
│                                                             │
│ ┌─────────────────────────┐  ┌─────────────────────────────┐│
│ │ ┌─────┐                 │  │ ┌─────┐                     ││
│ │ │icon │  filename.ext   │  │ │icon │  filename.ext       ││
│ │ └─────┘  v1 • 2.4 MB    │  │ └─────┘  v3 • 2.1 MB        ││
│ │                         │  │                             ││
│ │ [Download]              │  │ [Download]                  ││
│ └─────────────────────────┘  └─────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

- Thumbnail preview if available (from existing thumbnail system)
- Filename and version number
- File size
- Prominent download button for each version

## Entry Points

### 1. Version panel (same-resource comparison)

Update `templates/partials/versionPanel.tpl` - when user selects two versions and clicks compare:
```
Navigate to: /resource/compare?r1={resourceId}&v1={selected1}&v2={selected2}
```

### 2. Bulk action (cross-resource comparison)

In the resource list view, when exactly 2 resources are selected:
- Add "Compare" button to bulk actions bar
- Clicking navigates to: `/resource/compare?r1={id1}&r2={id2}`
- Both default to their current versions (v1/v2 params omitted)

```
┌─────────────────────────────────────────────────────────────┐
│ 2 selected    [Add Tags] [Remove Tags] [Compare] [Delete]  │
└─────────────────────────────────────────────────────────────┘
```

### 3. Compare page resource picker

- Resource autocomplete uses resource search endpoint (`/v1/resources`)
- Version select populates via AJAX call to `/v1/resource/versions?id={resourceId}`
- Changing either resource or version updates URL and reloads comparison

## Implementation

### Backend Files

| File | Action | Description |
|------|--------|-------------|
| `server/template_handlers/compare_handler.go` | Create | Handler for `GET /resource/compare` |
| `application_context/resource_version_context.go` | Modify | Extend `CompareVersions` for cross-resource support |
| `server/routes.go` | Modify | Add compare page route |
| `templates/compare.tpl` | Create | Main comparison page template |
| `templates/partials/versionPanel.tpl` | Modify | Update compare button to navigate to compare page |
| `templates/resources.tpl` | Modify | Add "Compare" to bulk actions when 2 selected |

### Frontend Files

| File | Action | Description |
|------|--------|-------------|
| `src/components/compareView.js` | Create | Alpine.js component for resource/version selection, URL sync, mode selection |
| `src/components/imageCompare.js` | Create | Image comparison modes (side-by-side, slider, onion skin, toggle) |
| `src/components/textDiff.js` | Create | Text diff rendering (unified, side-by-side) |
| `package.json` | Modify | Add `jsdiff` dependency for text diffing |

### E2E Test Files

| File | Action | Description |
|------|--------|-------------|
| `e2e/tests/version-compare.spec.ts` | Create | E2E tests for comparison feature |

## E2E Test Cases

1. **Same-resource comparison via version panel**
   - Create resource, upload two versions
   - Open version panel, select two versions, click compare
   - Verify navigation to compare page with correct params
   - Verify metadata table shows both versions

2. **Cross-resource comparison via bulk action**
   - Create two resources
   - Select both in resource list
   - Verify "Compare" button appears in bulk actions
   - Click compare, verify navigation with both resource IDs

3. **Resource/version picker on compare page**
   - Navigate to compare page
   - Change resource via autocomplete
   - Verify version dropdown updates with new resource's versions
   - Change version, verify URL updates

4. **Image comparison modes**
   - Compare two image resources
   - Verify all four mode buttons present
   - Click each mode, verify view changes appropriately

5. **Text diff view**
   - Create two versions of a text file with different content
   - Compare them, verify diff is displayed
   - Toggle between unified and side-by-side modes

6. **PDF iframe loading**
   - Compare two PDF resources
   - Verify "Load in viewer" button present
   - Click it, verify iframes appear

7. **Metadata comparison accuracy**
   - Compare resources with different sizes/types
   - Verify delta percentages and match indicators are correct
