# Frontend Component Inventory

---

## globalSearch

**File:** src/components/globalSearch.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('globalSearch', globalSearch)`

### What It Does
Provides a modal search dialog for finding resources, notes, groups, tags, categories, queries, relation types, and note types across the application. Results are fetched from `/v1/search` with adaptive debouncing and a client-side LRU cache (30s TTL, max 50 entries). Screen reader announcements via ARIA live region.

### Public API
- Properties: `isOpen` (boolean), `query` (string), `results` (array), `selectedIndex` (number), `loading` (boolean), `typeIcons` (object), `typeLabels` (object)
- Methods: `toggle()`, `close()`, `search()`, `navigateUp()`, `navigateDown()`, `selectResult()`, `navigateTo(url)`, `getIcon(type)`, `getLabel(type)`, `highlightMatch(text, query)`
- Events: none dispatched; listens for global `keydown`

### Keyboard Shortcuts
- `Cmd/Ctrl+K`: toggle search dialog open/closed
- `ArrowUp/ArrowDown`: navigate results
- `Enter`: navigate to selected result
- `Escape`: close dialog (via Alpine focus trap)

### Template Integration
- Used in global layout templates (search overlay)

---

## bulkSelection (store + components)

**File:** src/components/bulkSelection.js
**Type:** Alpine.js store + two Alpine.js data components + global event listener setup
**Registration:** `Alpine.store('bulkSelection', ...)` via `registerBulkSelectionStore(Alpine)`, `Alpine.data('bulkSelectionForms', bulkSelectionForms)`, `Alpine.data('selectableItem', selectableItem)`, `setupBulkSelectionListeners()` called at init

### What It Does
Manages multi-select behavior across entity list pages. The store tracks selected item IDs and manages bulk action editor forms. `selectableItem` wraps individual list checkboxes with click, shift-click (range select), right-click, and keyboard toggling. `bulkSelectionForms` registers bulk action forms (add/remove tags, delete, merge) that submit via AJAX and morph the list container on success. `setupBulkSelectionListeners` adds a global spacebar handler for text-selection-based toggling and inline tag editing.

### Public API
**Store (`$store.bulkSelection`):**
- Properties: `selectedIds` (Set), `elements` (array), `editors` (array), `options` (object), `activeEditor` (HTMLElement|null), `lastSelected` (any)
- Methods: `isSelected(id)`, `isAnySelected()`, `select(id)`, `deselect(id)`, `toggle(id)`, `selectUntil(id)`, `deselectAll()`, `selectAll()`, `toggleEditor(form)`, `isActiveEditor(el)`, `setActiveEditor(el)`, `closeEditor(el)`, `registerOption(option)`, `registerForm(form)`

**Data component `selectableItem({ itemNo, itemId })`:**
- Properties: none exposed
- Methods: `selected()` (returns boolean)
- Events via `events` object: `@click`, `@contextmenu`, `@keydown.space.prevent`, `@keydown.enter.prevent`

**Data component `bulkSelectionForms`:**
- Methods: `init()` (auto-registers child forms)

### Keyboard Shortcuts
- `Space`: toggle selection on items within a text selection range
- `Shift+Click`: range select/deselect
- `Space/Enter` on selectable item: toggle selection

### Template Integration
- List pages for resources, notes, groups (list-container elements)

---

## blockEditor

**File:** src/components/blockEditor.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockEditor', blockEditor)`

### What It Does
Manages a block-based content editor for notes. Blocks are ordered entities (text, heading, divider, gallery, references, todos, table, calendar) fetched from `/v1/note/blocks`. Supports CRUD operations, drag reordering, debounced content auto-save, and lexicographic fractional positioning (port of Go's position algorithm). Block types are loaded from the server API at init.

### Public API
- Properties: `noteId` (number), `blocks` (array), `editMode` (boolean), `addBlockPickerOpen` (boolean), `loading` (boolean), `error` (string|null), `blockTypes` (array)
- Methods: `init()`, `loadBlocks()`, `toggleEditMode()`, `addBlock(type, afterPosition)`, `updateBlockContentDebounced(blockId, content)`, `updateBlockContent(blockId, content)`, `updateBlockState(blockId, state)`, `deleteBlock(blockId)`, `moveBlock(blockId, direction)`, `renderMarkdown(text)`, `getDefaultContent(type)`, `calculatePosition(afterPosition)`, `positionBetween(before, after)`
- Events: none

### Template Integration
- Note detail page (block editor section)

---

## blockText

**File:** src/components/blocks/blockText.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockText', blockText)`

### What It Does
Renders and edits a text block within the block editor. Supports debounced auto-save on input and immediate save on blur. Receives save functions from the parent blockEditor.

### Public API
- Properties: `block` (object), `text` (string)
- Methods: `onInput()`, `save()`

---

## blockHeading

**File:** src/components/blocks/blockHeading.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockHeading', blockHeading)`

### What It Does
Renders and edits a heading block with configurable level (1-6). Supports debounced auto-save and immediate save on blur or level change.

### Public API
- Properties: `block` (object), `text` (string), `level` (number)
- Methods: `onInput()`, `save()`

---

## blockDivider

**File:** src/components/blocks/blockDivider.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockDivider', blockDivider)`

### What It Does
Renders a horizontal divider block. Contains no editable state or content.

### Public API
- Properties: none
- Methods: none

---

## blockTodos

**File:** src/components/blocks/blockTodos.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockTodos', blockTodos)`

### What It Does
Renders a checklist/todo block. Items can be checked/unchecked (persisted to block state), and in edit mode items can be added, removed, and relabeled. Check state is separate from content to allow non-edit interactions.

### Public API
- Properties: `block` (object), `items` (array of {id, label}), `checked` (array of ids), `editMode` (getter, boolean)
- Methods: `isChecked(itemId)`, `toggleCheck(itemId)`, `saveContent()`, `addItem()`, `removeItem(idx)`

---

## blockGallery

**File:** src/components/blocks/blockGallery.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockGallery', blockGallery)`

### What It Does
Renders a gallery of resources (images/videos) within a note block. Fetches resource metadata for lightbox integration. Uses the entityPicker store to browse and add resources. Supports removing individual resources and opening the lightbox at a specific index.

### Public API
- Properties: `block` (object), `resourceIds` (array of numbers), `resourceMeta` (object), `editMode` (getter, boolean), `noteId` (number)
- Methods: `init()`, `fetchResourceMeta()`, `openPicker()`, `openGalleryLightbox(index)`, `updateResourceIds(value)`, `addResources(ids)`, `removeResource(id)`

---

## blockReferences

**File:** src/components/blocks/blockReferences.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockReferences', blockReferences)`

### What It Does
Renders a list of referenced groups within a note block. Fetches group metadata (name, breadcrumb) via the picker module. Uses the entityPicker store to browse and add groups.

### Public API
- Properties: `block` (object), `groupIds` (array of numbers), `groupMeta` (object), `loadingMeta` (boolean), `editMode` (getter, boolean)
- Methods: `init()`, `fetchGroupMeta()`, `openPicker()`, `getGroupDisplay(id)`, `addGroups(ids)`, `removeGroup(id)`

---

## blockTable

**File:** src/components/blocks/blockTable.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockTable', blockTable)`

### What It Does
Renders a data table block that operates in two modes: manual (user-defined columns and rows) or query mode (fetches data from a saved query via `/v1/note/block/table/query`). Includes client-side sorting, stale-while-revalidate caching (30s TTL, 10s stale threshold), and static/dynamic refresh modes.

### Public API
- Properties: `block` (object), `columns` (array), `rows` (array), `queryId` (number|null), `queryParams` (object), `isStatic` (boolean), `queryColumns` (array), `queryRows` (array), `queryLoading` (boolean), `queryError` (string|null), `isRefreshing` (boolean), `lastFetchTime` (Date|null), `sortColumn` (string), `sortDirection` (string), `editMode` (getter, boolean), `isQueryMode` (getter, boolean), `displayColumns` (getter), `displayRows` (getter), `sortedRows` (getter), `lastFetchTimeFormatted` (getter)
- Methods: `init()`, `toggleSort(colId)`, `saveContent()`, `fetchQueryData(forceRefresh)`, `manualRefresh()`, `selectQuery(query)`, `clearQuery()`, `toggleStatic()`, `updateQueryParam(key, value)`, `removeQueryParam(key)`, `addQueryParam()`, `addColumn()`, `removeColumn(idx)`, `addRow()`, `removeRow(idx)`

---

## blockCalendar

**File:** src/components/blocks/blockCalendar.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockCalendar', blockCalendar)`

### What It Does
Renders a calendar block with month and agenda views. Supports multiple calendar sources (ICS URLs, resource-based ICS files) and custom events stored in block state. Uses stale-while-revalidate caching (5 min threshold). Calendar sources are managed in edit mode. Event data is fetched from `/v1/note/block/calendar/events`.

### Public API
- Properties: `block` (object), `calendars` (array), `view` (string: 'month'|'agenda'), `currentDate` (Date), `customEvents` (array), `events` (array), `calendarMeta` (object), `loading` (boolean), `error` (string|null), `isRefreshing` (boolean), `lastFetchTime` (Date|null), `newUrl` (string), `showColorPicker` (string|null), `showEventModal` (boolean), `editingEvent` (object|null), `eventForm` (object), `expandedDay` (string|null), `editMode` (getter), `currentMonth` (getter), `currentYear` (getter), `dateRange` (getter), `monthDays` (getter), `agendaEvents` (getter)
- Methods: `init()`, `fetchEvents(forceRefresh)`, `prevMonth()`, `nextMonth()`, `setView(v)`, `saveState()`, `saveContent()`, `addCalendarFromUrl()`, `addCalendarFromResource(resourceId, resourceName)`, `removeCalendar(calId)`, `updateCalendarName(calId, name)`, `updateCalendarColor(calId, color)`, `openResourcePicker()`, `getEventsForDay(date)`, `isToday(date)`, `isExpanded(date)`, `toggleExpandedDay(date)`, `closeExpandedDay()`, `goToEventMonth(event)`, `formatEventTime(event)`, `formatAgendaDate(date)`, `getCalendarColor(calId)`, `getCalendarName(calId)`, `isCustomEvent(event)`, `openEventModalForDay(date)`, `openEventModalForEdit(event)`, `closeEventModal()`, `saveEvent()`, `deleteEvent()`

---

## eventModal

**File:** src/components/blocks/eventModal.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('eventModal', eventModal)`

### What It Does
Reusable modal component for creating and editing calendar events. Used by blockCalendar. Provides form fields for title, dates, times, all-day toggle, location, and description. Invokes callback functions on save and delete.

### Public API
- Properties: `isOpen` (boolean), `mode` (string: 'create'|'edit'), `event` (object|null), `title` (string), `startDate` (string), `startTime` (string), `endDate` (string), `endTime` (string), `allDay` (boolean), `location` (string), `description` (string), `onSave` (function|null), `onDelete` (function|null)
- Methods: `open(options)`, `close()`, `save()`, `deleteEvent()`, `onAllDayChange()`

---

## downloadCockpit

**File:** src/components/downloadCockpit.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('downloadCockpit', downloadCockpit)`

### What It Does
Provides a slide-out panel displaying background download and plugin action job progress via Server-Sent Events (SSE) from `/v1/jobs/events`. Tracks download speed, progress percentages, and job status. Supports pause/resume/cancel/retry operations. Retains completed/failed jobs briefly after backend removal. Uses exponential backoff for SSE reconnection.

### Public API
- Properties: `isOpen` (boolean), `jobs` (array), `retainedCompletedJobs` (array), `eventSource` (EventSource|null), `connectionStatus` (string: 'connected'|'disconnected'|'connecting'), `speedTracking` (object), `statusIcons` (object), `statusLabels` (object), `activeCount` (getter, number), `hasActiveJobs` (getter, boolean), `displayJobs` (getter, array)
- Methods: `init()`, `toggle()`, `close()`, `connect()`, `disconnect()`, `cancelJob(jobId)`, `pauseJob(jobId)`, `resumeJob(jobId)`, `retryJob(jobId)`, `formatProgress(job)`, `formatBytes(bytes)`, `getSpeed(job)`, `formatSpeed(job)`, `getProgressPercent(job)`, `isActive(job)`, `canPause(job)`, `canResume(job)`, `canRetry(job)`, `truncateUrl(url, maxLength)`, `getJobTitle(job)`, `getJobSubtitle(job)`, `getFilename(url)`
- Events: listens for `jobs-panel-open` (window), dispatches `download-completed` (window), dispatches `plugin-action-completed` (window)

### Keyboard Shortcuts
- `Cmd/Ctrl+Shift+D`: toggle jobs panel

### Template Integration
- Global layout (jobs/download panel overlay)

---

## groupTree

**File:** src/components/groupTree.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('groupTree', groupTree)`

### What It Does
Renders an interactive hierarchical tree of groups. Supports lazy-loading of child groups from `/v1/group/tree/children`, path highlighting, and expand/collapse toggling. Tree nodes are rendered imperatively as DOM elements (not Alpine templates).

### Public API
- Properties: `tree` (object map of parent->children), `expandedNodes` (Set), `loadingNodes` (Set), `highlightedSet` (Set), `containingId` (number), `rootId` (number), `requestAborters` (Map)
- Methods: `init()`, `buildTree(rows)`, `render()`, `renderNode(node, isRoot)`, `handleClick(e)`, `expandNode(nodeId)`

### Template Integration
- Group detail page (hierarchy tree section)

---

## imageCompare

**File:** src/components/imageCompare.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('imageCompare', imageCompare)`

### What It Does
Provides image comparison with multiple modes: side-by-side, slider (swipe), overlay (opacity blend), and toggle. Slider supports mouse and touch drag. Images can be swapped between left and right sides.

### Public API
- Properties: `mode` (string: 'side-by-side'|'slider'|'overlay'|'toggle'), `leftUrl` (string), `rightUrl` (string), `sliderPos` (number 0-100), `opacity` (number 0-100), `showLeft` (boolean), `isDragging` (boolean)
- Methods: `swapSides()`, `toggleSide()`, `startSliderDrag(e)`

### Template Integration
- Resource compare page (image comparison view)

---

## textDiff

**File:** src/components/textDiff.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('textDiff', textDiff)`

### What It Does
Fetches two text files by URL and computes a line-level diff using the `diff` library. Supports unified and split (side-by-side) display modes with added/removed/context line annotations and statistics.

### Public API
- Properties: `mode` (string: 'unified'|'split'), `loading` (boolean), `error` (string|null), `leftText` (string), `rightText` (string), `unifiedDiff` (array), `splitLeft` (array), `splitRight` (array), `stats` ({added, removed})
- Methods: `init()`, `computeDiff()`

### Template Integration
- Resource compare page (text diff view)

---

## compareView

**File:** src/components/compareView.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('compareView', compareView)`

### What It Does
Manages URL state for the resource version comparison page. Handles resource and version selection via dropdowns, fetching available versions per resource, and updating URL parameters to trigger page navigation.

### Public API
- Properties: `r1` (number|string), `v1` (number|string), `r2` (number|string), `v2` (number|string)
- Methods: `updateUrl()`, `fetchVersions(resourceId)`, `onResource1Change(resourceId)`, `onResource2Change(resourceId)`, `onVersion1Change(versionNumber)`, `onVersion2Change(versionNumber)`

### Template Integration
- Resource compare page (selector controls)

---

## schemaForm

**File:** src/components/schemaForm.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('schemaForm', schemaForm)`

### What It Does
Dynamically renders form UI from a JSON Schema definition. Supports all standard JSON Schema features: object/array/primitive types, `$ref` resolution, `oneOf`/`anyOf`/`allOf` composition, `if/then/else` conditionals, `enum`/`const`, validation constraints (min/max, minLength/maxLength, exclusiveMinimum/Maximum, pattern), required fields, and additional properties (free-form key/value editing). Produces a hidden JSON text field for form submission.

### Public API
- Properties: `schema` (object), `value` (object), `name` (string), `jsonText` (string)
- Methods: `init()`, `updateJson()`, `renderForm()`

### Template Integration
- Plugin settings pages, meta editing forms, anywhere JSON Schema-driven forms are needed

---

## pasteUpload (store + listener)

**File:** src/components/pasteUpload.js
**Type:** Alpine.js store + global paste event listener
**Registration:** `Alpine.store('pasteUpload', ...)` via `registerPasteUploadStore(Alpine)`, `setupPasteListener()` called at init

### What It Does
Intercepts global paste events and provides a modal workflow for uploading pasted content (images, files, HTML, plain text) as resources. Detects upload context from `data-paste-context` attributes or `ownerId` query parameters. Supports batch uploads with per-item error handling, duplicate detection (showing existing resource IDs), tag/category/series assignment, auto-close on success, and page morphing after upload.

### Public API
**Store (`$store.pasteUpload`):**
- Properties: `isOpen` (boolean), `items` (array of {file, name, previewUrl, type, error, errorResourceId, _snippet}), `context` (object|null), `tags` (array), `categoryId` (number|null), `seriesId` (number|null), `state` (string: 'idle'|'preview'|'uploading'|'success'|'error'), `uploadProgress` (string), `errorMessage` (string), `infoMessage` (string)
- Methods: `open(items, context)`, `close()`, `removeItem(index)`, `showInfo(message)`, `upload()`
- Events: none dispatched

**Exported utility:** `extractPasteContent(clipboardData)` -- extracts uploadable items from ClipboardEvent data

### Template Integration
- Global (paste listener active on all pages; modal overlay)

---

## cardActionMenu

**File:** src/components/cardActionMenu.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('cardActionMenu', cardActionMenu)`

### What It Does
Provides a dropdown menu on entity cards for triggering plugin actions. Dispatches a `plugin-action-open` custom event with action details (plugin name, action ID, entity IDs, parameters, confirmation requirements) for the pluginActionModal to handle.

### Public API
- Properties: `open` (boolean)
- Methods: `toggle()`, `close()`, `runAction(action, entityId, entityType)`
- Events: dispatches `plugin-action-open` (window)

### Template Integration
- Entity card components (resource, note, group cards in list views)

---

## pluginActionModal

**File:** src/components/pluginActionModal.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('pluginActionModal', pluginActionModal)`

### What It Does
Renders a modal dialog for executing plugin actions. Listens for `plugin-action-open` events, displays parameter forms based on action definition, validates required fields, submits to `/v1/jobs/action/run`, and handles async job creation (opening jobs panel), redirects, or inline results.

### Public API
- Properties: `isOpen` (boolean), `action` (object|null), `formValues` (object), `errors` (object), `submitting` (boolean), `result` (object|null)
- Methods: `init()`, `open(detail)`, `close()`, `submit()`
- Events: listens for `plugin-action-open` (window), dispatches `jobs-panel-open` (window)

### Template Integration
- Global layout (plugin action modal overlay)

---

## pluginSettings

**File:** src/components/pluginSettings.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('pluginSettings', pluginSettings)`

### What It Does
Manages plugin settings forms. Collects form values including special handling for checkboxes (unchecked state) and number fields (type coercion), submits as JSON to `/v1/plugin/settings`, and displays save confirmation or validation errors.

### Public API
- Properties: `pluginName` (string), `saved` (boolean), `error` (string)
- Methods: `saveSettings(event)`

### Template Integration
- Plugin settings pages

---

## sharedCalendar

**File:** src/components/sharedCalendar.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('sharedCalendar', sharedCalendar)`

### What It Does
Read-only calendar component for shared note views (public share links). Displays events from a share server endpoint `/s/{token}/block/{blockId}/calendar/events`. Supports month and agenda views, custom event creation/editing, and state persistence to the share server.

### Public API
- Properties: `blockId` (string), `shareToken` (string), `calendars` (array), `view` (string), `currentDate` (Date), `customEvents` (array), `events` (array), `calendarMeta` (object), `loading` (boolean), `error` (string|null), `isRefreshing` (boolean), `showEventModal` (boolean), `editingEvent` (object|null), `eventForm` (object), `expandedDay` (string|null), `currentMonth` (getter), `currentYear` (getter), `dateRange` (getter), `monthDays` (getter), `agendaEvents` (getter)
- Methods: `init()`, `fetchEvents(forceRefresh)`, `saveState()`, `prevMonth()`, `nextMonth()`, `setView(v)`, `goToEventMonth(event)`, `getEventsForDay(date)`, `isToday(date)`, `isExpanded(date)`, `toggleExpandedDay(date)`, `closeExpandedDay()`, `formatEventTime(event)`, `formatAgendaDate(date)`, `getCalendarColor(calId)`, `getCalendarName(calId)`, `isCustomEvent(event)`, `openEventModalForDay(date)`, `openEventModalForEdit(event)`, `closeEventModal()`, `saveEvent()`, `deleteEvent()`

### Template Integration
- Shared note view pages

---

## sharedTodos

**File:** src/components/sharedTodos.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('sharedTodos', sharedTodos)`

### What It Does
Simplified todos component for shared note views. Allows checking/unchecking items (no add/remove/edit). Performs optimistic updates with rollback on server error. Syncs state to `/s/{token}/block/{blockId}/state`.

### Public API
- Properties: `blockId` (string), `shareToken` (string), `checked` (array of ids), `saving` (boolean), `error` (string|null)
- Methods: `isChecked(itemId)`, `toggleItem(itemId)`

### Template Integration
- Shared note view pages

---

## multiSort

**File:** src/components/multiSort.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('multiSort', multiSort)`

### What It Does
Provides a multi-column sort builder UI for entity list queries. Users can add/remove/reorder sort criteria, choose columns and directions (asc/desc), and sort by metadata keys (JSON path expressions). Initializes from URL query parameters and produces hidden form inputs for submission.

### Public API
- Properties: `sortColumns` (array of {column, direction, metaKey}), `availableColumns` (array of {Name, Value}), `name` (string)
- Methods: `init()`, `parseSort(sortStr)`, `formatSort(sort)`, `addSort()`, `removeSort(index)`, `isValidMetaKey(key)`, `moveUp(index)`, `moveDown(index)`, `getColumnName(value)`, `getAvailableColumnsForRow(currentIndex)`

### Template Integration
- Entity list pages (sort controls)

---

## freeFields

**File:** src/components/freeFields.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('freeFields', freeFields)`

### What It Does
Renders dynamic key-value metadata fields for entities. Supports loading initial values from JSON, fetching remote field suggestions from a URL, and outputting combined values as a JSON string in a hidden input. Handles type coercion for numeric, boolean, null, and date values.

### Public API
- Properties: `fields` (array of {name, value}), `name` (string), `url` (string), `jsonOutput` (boolean), `id` (string), `title` (string), `fromJSON` (object), `remoteFields` (array), `jsonText` (string)
- Methods: `init()`

**Exported utilities (global):**
- `generateParamNameForMeta({name, value, operation})`: builds meta query filter strings
- `getJSONValue(x)`: coerces string to typed JSON value
- `getJSONOrObjValue(x)`: like getJSONValue but also parses JSON objects/arrays

### Template Integration
- Entity create/edit forms (metadata sections)

---

## codeEditor

**File:** src/components/codeEditor.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('codeEditor', codeEditor)`

### What It Does
Wraps CodeMirror 6 as an Alpine component for editing SQL and HTML code. Loads language extensions asynchronously (SQL with dialect-specific autocompletion from `/v1/query/schema`, or HTML). Syncs editor content back to a hidden input for form submission. Includes line numbers, bracket matching, auto-closing brackets, syntax highlighting, and undo history.

### Public API
- Properties: `view` (EditorView|null), `langCompartment` (Compartment)
- Methods: `init()`, `loadSQL(dbType)`, `loadHTML()`, `destroy()`

### Template Integration
- Query create/edit pages, HTML editor fields

---

## lightbox (store)

**File:** src/components/lightbox.js (+ lightbox/navigation.js, lightbox/zoom.js, lightbox/gestures.js, lightbox/editPanel.js, lightbox/quickTagPanel.js)
**Type:** Alpine.js store
**Registration:** `Alpine.store('lightbox', ...)` via `registerLightboxStore(Alpine)`

### What It Does
Full-featured image/video viewer with pagination across list pages. Composed of five modules:

**Navigation** (lightbox/navigation.js): Opens/closes the lightbox, navigates between items, loads next/previous pages via JSON API, extracts lightbox items from DOM `[data-lightbox-item]` elements, handles multi-section source containers.

**Zoom** (lightbox/zoom.js): Zoom (1x-5x) and pan with constraint bounds. Fullscreen toggle. Native zoom percentage display and preset zoom levels (Fit, Stretch, 25%-500%). Zoom preset popover.

**Gestures** (lightbox/gestures.js): Touch swipe navigation, pinch-to-zoom with zoom-toward-center tracking, mouse drag pan when zoomed, wheel navigation (horizontal scroll = prev/next, ctrl+wheel = zoom toward cursor), double-click to zoom to native resolution.

**Edit Panel** (lightbox/editPanel.js): Side panel for editing resource name, description, and tags directly within the lightbox. Caches resource details (LRU, max 100). Uses API calls for name/description updates and tag add/remove. Morphs list container on close if changes were made.

**Quick Tag Panel** (lightbox/quickTagPanel.js): Side panel with 9 configurable tag slots (persisted to localStorage). One-click/keyboard toggle tags on the current resource. Number keys 1-9 toggle the corresponding tag slot.

### Public API
**Store (`$store.lightbox`):**
- Properties: `isOpen` (boolean), `currentIndex` (number), `items` (array), `loading` (boolean), `pageLoading` (boolean), `currentPage` (number), `hasNextPage` (boolean), `hasPrevPage` (boolean), `isFullscreen` (boolean), `zoomLevel` (number), `panX` (number), `panY` (number), `editPanelOpen` (boolean), `resourceDetails` (object|null), `detailsLoading` (boolean), `quickTagPanelOpen` (boolean), `quickTagSlots` (array of 9), `isDragging` (boolean), `animationsDisabled` (boolean), `needsRefreshOnClose` (boolean)
- Methods: `init()`, `initFromDOM()`, `open(index)`, `openFromClick(event, resourceId, contentType)`, `close()`, `next()`, `prev()`, `toggleFullscreen()`, `isZoomed()`, `setZoomLevel(level)`, `resetZoom()`, `nativeZoomPercent()`, `zoomPresets()`, `setNativeZoom(nativePct)`, `showZoomPresets(btn)`, `handleTouchStart(e)`, `handleTouchMove(e)`, `handleTouchEnd(e)`, `handleWheel(e)`, `handleDoubleClick(e)`, `handleMouseDown(e)`, `handleMouseMove(e)`, `handleMouseUp(e)`, `openEditPanel()`, `closeEditPanel()`, `updateName(newName)`, `updateDescription(newDescription)`, `saveTagAddition(tag)`, `saveTagRemoval(tag)`, `getCurrentTags()`, `openQuickTagPanel()`, `closeQuickTagPanel()`, `setQuickTagSlot(index, tag)`, `clearQuickTagSlot(index)`, `toggleQuickTag(index)`, `isTagOnResource(tagId)`, `focusTagEditor()`, `quickTagKeyLabel(index)`

### Keyboard Shortcuts
- `ArrowLeft/ArrowRight`: prev/next image (handled in template)
- `Escape`: close lightbox
- `1-9`: toggle quick tag slots (handled in template)
- `Double-click`: zoom to native resolution / reset zoom
- `Ctrl+Scroll`: zoom toward cursor

### Template Integration
- All pages with resource listings (gallery, list, dashboard grid)

---

## entityPicker (store)

**File:** src/components/picker/entityPicker.js (+ picker/entityConfigs.js, picker/entityMeta.js)
**Type:** Alpine.js store
**Registration:** `Alpine.store('entityPicker', ...)` via `registerEntityPickerStore(Alpine)`

### What It Does
A generic modal picker for selecting entities (resources, groups) from the application. Supports search with debounce, tab-based views (e.g., "Note Resources" vs "All Resources"), filter parameters, and multi-select with existing-item exclusion. Used by block components (gallery, references, calendar) to browse and add entities.

### Public API
**Store (`$store.entityPicker`):**
- Properties: `config` (object|null), `isOpen` (boolean), `activeTab` (string|null), `loading` (boolean), `error` (string|null), `noteId` (number|null), `searchQuery` (string), `filterValues` (object), `results` (array), `tabResults` (object), `selectedIds` (Set), `existingIds` (Set), `onConfirm` (function|null), `displayResults` (getter), `hasTabResults` (getter), `selectionCount` (getter)
- Methods: `open({entityType, noteId, existingIds, onConfirm})`, `close()`, `confirm()`, `loadResults()`, `onSearchInput()`, `setFilter(key, value)`, `addToFilter(key, value)`, `removeFromFilter(key, value)`, `toggleSelection(itemId)`, `isSelected(itemId)`, `isAlreadyAdded(itemId)`, `setActiveTab(tabId)`
- Events: dispatches `entity-picker-closed` (window)

### Template Integration
- Used programmatically by blockGallery, blockReferences, blockCalendar

---

## autocompleter (dropdown)

**File:** src/components/dropdown.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('autocompleter', autocompleter)`

### What It Does
A multi-select autocomplete dropdown used throughout the app for selecting tags, groups, notes, categories, and other entities. Fetches suggestions from a configurable URL with debounced search. Supports creating new items via `addUrl`, popover-based dropdown positioning, standalone mode for lightbox integration, and custom event dispatch on selection. Uses ARIA live region for screen reader announcements.

### Public API
- Properties: `max` (number), `min` (number), `ownerId` (number), `results` (array), `selectedIndex` (number), `errorMessage` (boolean|string), `dropdownActive` (boolean), `selectedResults` (array), `selectedIds` (Set), `url` (string), `addUrl` (string), `extraInfo` (string), `filterEls` (array), `sortBy` (string), `addModeForTag` (string|boolean), `loading` (boolean)
- Methods: `init()`, `destroy()`, `addVal()`, `exitAdd()`, `pushVal($event)`, `ensureMaxItems()`, `removeItem(item)`, `getItemDisplayName(item)`, `announceSelectedItem()`, `showSelected()`
- Events: dispatches `multiple-input` (on element, with name and value), dispatches custom event via `dispatchOnSelect` parameter (window)
- Input events object: `@keydown.escape`, `@keydown.arrow-up.prevent`, `@keydown.arrow-down.prevent`, `@keydown.enter.prevent`, `@keydown.tab`, `@blur`, `@focus`, `@input`

### Keyboard Shortcuts
- `ArrowUp/ArrowDown`: navigate dropdown results
- `Enter`: select highlighted item or trigger add mode
- `Escape`: close dropdown
- `Tab`: close dropdown

### Template Integration
- Entity create/edit forms (tag, group, note, category pickers), bulk selection tag editors, lightbox tag editor

---

## confirmAction

**File:** src/components/confirmAction.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('confirmAction', confirmAction)`

### What It Does
Wraps a form to show a confirmation dialog before submission. When the form is submitted, shows a `confirm()` dialog with a configurable message. Holding Shift bypasses the confirmation.

### Public API
- Properties: `message` (string)
- Methods: none (behavior is via events object)
- Events object: `@submit` (prevents default unless confirmed or shift held)

### Keyboard Shortcuts
- `Shift+Submit`: bypass confirmation dialog

### Template Integration
- Delete forms throughout the application

---

## savedSetting (store)

**File:** src/components/storeConfig.js
**Type:** Alpine.js store
**Registration:** `Alpine.store('savedSetting', ...)` via `registerSavedSettingStore(Alpine)`

### What It Does
Persists UI settings (checkbox states, input values) to localStorage or sessionStorage. Registers elements whose values are restored on page load and auto-saved on change.

### Public API
**Store (`$store.savedSetting`):**
- Properties: `sessionSettings` (object), `localSettings` (object)
- Methods: `registerEl(el, isLocal, defVal)` -- registers an element for persistence

### Template Integration
- Settings toggles in list pages and layout

---

## expandable-text

**File:** src/webcomponents/expandabletext.js
**Type:** Web Component (Custom Element)
**Registration:** `customElements.define('expandable-text', ExpandableText)`

### What It Does
A custom HTML element that truncates long text to 30 characters with a "Read more"/"Read less" toggle button. Includes a "Copy" button to copy the full text to clipboard. Uses Shadow DOM with scoped styles and ARIA attributes for accessibility.

### Public API
- Attributes: none
- Content: text content inside the element tag
- Shadow DOM: displays preview, full text (hidden by default), toggle button, copy button

### Template Integration
- JSON table rendering (tableMaker.js), entity detail pages for long text values

---

## inline-edit

**File:** src/webcomponents/inlineedit.js
**Type:** Web Component (Custom Element)
**Registration:** `customElements.define('inline-edit', InlineEdit)`

### What It Does
An inline editable text element. Displays text with a pencil icon edit button. Clicking the button switches to an input/textarea. On blur, submits the new value via POST to a configurable URL. Shows green flash on success, red flash and rollback on error. Escape cancels editing.

### Public API
- Observed Attributes: `multiline` (boolean, switches to textarea), `post` (string, URL to POST changes), `name` (string, form field name), `label` (string, ARIA label)
- Content: text content inside the element tag
- Methods (internal): `enterEditMode()`, `exitEditMode()`

### Keyboard Shortcuts
- `Escape`: cancel editing and revert
- `Enter` (single-line mode): save and exit edit mode

### Template Integration
- Entity detail pages for inline name/description editing

---

## renderJsonTable (tableMaker)

**File:** src/tableMaker.js
**Type:** Utility (global function)
**Registration:** `window.renderJsonTable = renderJsonTable`

### What It Does
Recursively renders arbitrary JSON data (objects, arrays, primitives) as nested HTML tables. Object keys become table headers, arrays become columnar tables (when all elements are objects) or row lists. Subtables are collapsible with toggle buttons. Clicking any cell copies its JSONPath to clipboard. Uses `<expandable-text>` for long strings. Supports Shift+click to expand/collapse all subtables.

### Public API
- `renderJsonTable(data, path)`: returns an HTMLElement (table or text node)

### Template Integration
- Entity detail pages (metadata display), query result rendering

---

## Utility Functions (index.js)

**File:** src/index.js
**Type:** Utility (global functions)
**Registration:** All exported functions attached to `window.*`

### What It Does
Provides shared utility functions used across components and templates.

### Public API
- `abortableFetch(request, opts)`: returns `{abort, ready}` for cancellable fetch requests
- `isUndef(x)`: returns boolean
- `isNumeric(x)`: returns boolean
- `pick(obj, ...keys)`: returns filtered object
- `setCheckBox(checkBox, checked)`: sets checkbox state
- `updateClipboard(newClip)`: copies text to clipboard with fallback
- `parseQueryParams(queryString)`: extracts `:paramName` placeholders from query strings
- `addMetaToGroup(id, val)`: POST JSON metadata to group
- `addMetaToResource(id, val)`: POST JSON metadata to resource

---

## main.js (Entry Point)

**File:** src/main.js
**Type:** Entry point / initialization
**Registration:** N/A

### What It Does
Bootstraps the entire frontend: imports and registers Alpine.js plugins (morph, collapse, focus), registers all Alpine stores (bulkSelection, savedSetting, lightbox, entityPicker, pasteUpload), registers all Alpine data components (27 total), exposes utility functions globally, starts Alpine, initializes lightbox from DOM, sets up bulk selection listeners, sets up global paste listener, and handles `download-completed` events to morph-refresh resource lists.

### Alpine Plugins Used
- `@alpinejs/morph`: DOM morphing for seamless updates
- `@alpinejs/collapse`: animated collapse/expand transitions
- `@alpinejs/focus`: focus trapping for modals/dialogs

### Global Event Listeners
- `DOMContentLoaded`: initialize lightbox store and scan DOM
- `download-completed`: morph-refresh `.list-container` when background downloads complete
