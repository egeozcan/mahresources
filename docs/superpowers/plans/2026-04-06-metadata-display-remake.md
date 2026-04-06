# Metadata Display Remake Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the metadata table on entity detail pages with type-aware rendering, informative collapsed states, and styling that matches the app's amber/stone design language.

**Architecture:** Modify the existing `tableMaker.js` rendering engine to detect value types (dates, booleans, URLs, IDs) and render them with distinct visual treatments. Restyle `jsonTable.css` to match the app's theme. Redesign `expandable-text` web component. Update `json.tpl` template to integrate the fullscreen toggle with the section header.

**Tech Stack:** JavaScript (DOM API), CSS, Pongo2 templates, Alpine.js, Playwright (E2E tests)

**Spec:** `docs/superpowers/specs/2026-04-06-metadata-display-remake.md`

---

### Task 1: Restyle jsonTable.css

Replace the standalone CSS with styles matching the app's amber/stone design language.

**Files:**
- Modify: `public/jsonTable.css` (entire file)

- [ ] **Step 1: Replace jsonTable.css with the new styles**

Replace the entire contents of `public/jsonTable.css` with:

```css
/* ===== Metadata table — amber/stone theme ===== */

.tableContainer {
    overflow: auto;
    max-width: 100%;
}

/* --- Fullscreen overlay --- */
.tableContainer.expanded {
    position: fixed;
    top: 0;
    left: 0;
    width: 100vw;
    height: 100vh;
    background: #fff;
    overflow: auto;
    z-index: 100;
    padding: 1.5rem;
}

.tableContainer.expanded > .metaTableInner {
    max-width: 1200px;
    margin: 0 auto;
}

/* --- Header bar (title + expand button) --- */
.metaHeader {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1rem 0.5rem 0;
}

.metaHeader .sidebar-group-title {
    margin: 0;
}

.metaExpandBtn {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    font-family: 'IBM Plex Mono', monospace;
    font-size: 0.75rem;
    color: #78716c;
    background: transparent;
    border: 1px solid #d6d3d1;
    border-radius: 6px;
    padding: 0.25rem 0.5rem;
    cursor: pointer;
    transition: border-color 120ms ease, color 120ms ease;
    flex-shrink: 0;
}

.metaExpandBtn:hover {
    border-color: #b45309;
    color: #b45309;
}

.metaExpandBtn:focus-visible {
    outline: 2px solid #b45309;
    outline-offset: 2px;
}

.metaExpandBtn:focus:not(:focus-visible) {
    outline: none;
}

.metaExpandBtn svg {
    flex-shrink: 0;
}

/* --- Base table --- */
.jsonTable {
    border-spacing: 0;
    width: 100%;
    border-collapse: collapse;
    overflow: auto;
    font-family: 'IBM Plex Mono', monospace;
}

.jsonTable .jsonTable {
    border: 1px solid #e7e5e4;
    border-radius: 4px;
    overflow: hidden;
    margin-top: 0.375rem;
}

/* --- Rows --- */
.jsonTable tr {
    border-left: 2px solid transparent;
    transition: border-color 120ms ease;
}

.jsonTable tr:hover {
    border-left-color: #b45309;
}

.jsonTable tr:hover > th,
.jsonTable tr:hover > td {
    background: #fafaf9;
}

/* --- Cells --- */
.jsonTable, .jsonTable tbody, .jsonTable tr, .jsonTable th, .jsonTable td {
    margin: 0;
    padding: 0;
    vertical-align: top;
}

.jsonTable th, .jsonTable td {
    text-align: left;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid #f5f5f4;
}

.jsonTable th {
    background: #f5f5f4;
    color: #57534e;
    font-size: 0.75rem;
    font-weight: 500;
    word-break: break-all;
}

.jsonTable td {
    background: #fff;
    color: #292524;
    font-size: 0.8125rem;
    position: relative;
    word-break: break-all;
}

.jsonTable td.odd {
    background: #fafaf9;
}

.jsonTable tr:last-child > th,
.jsonTable tr:last-child > td {
    border-bottom: none;
}

/* Nested subtable header cells slightly lighter */
.jsonTable .jsonTable > tbody > tr > th {
    background: #fafaf9;
}

.jsonTable tr.hasSubTable > th {
    padding-bottom: 0;
}

/* --- Collapsed state buttons --- */
.metaToggler {
    font-family: 'IBM Plex Mono', monospace;
    font-size: 0.6875rem;
    color: #78716c;
    background: #f5f5f4;
    border: 1px solid #e7e5e4;
    border-radius: 6px;
    padding: 0.1875rem 0.5rem;
    cursor: pointer;
    display: inline-block;
    user-select: none;
    transition: border-color 120ms ease, background 120ms ease, color 120ms ease;
}

.metaToggler:hover {
    border-color: #d6d3d1;
    background: #fafaf9;
    color: #57534e;
}

.metaToggler.expanded {
    background: #fff;
    border-color: #b45309;
    color: #b45309;
}

.metaToggler:focus-visible {
    outline: 2px solid #b45309;
    outline-offset: 2px;
}

.metaToggler:focus:not(:focus-visible) {
    outline: none;
}

/* --- Type-specific value styles --- */
.metaVal--id {
    color: #a8a29e;
    font-size: 0.75rem;
}

.metaVal--date {
    color: #78716c;
}

.metaVal--bool {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.75rem;
}

.metaVal--bool-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
}

.metaVal--bool-dot--on {
    background: #10b981;
}

.metaVal--bool-dot--off {
    background: #d6d3d1;
}

.metaVal--url {
    color: #b45309;
    text-decoration: none;
    font-size: 0.8125rem;
    word-break: break-all;
}

.metaVal--url:hover {
    text-decoration: underline;
}

/* --- Empty table --- */
.emptyTable::after {
    display: block;
    content: "No data";
    padding: 1rem 0;
    text-align: center;
    font-family: 'IBM Plex Mono', monospace;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.025em;
    color: #a8a29e;
}

/* --- Copy flash animation --- */
@keyframes copy-flash {
    0% { background: #fef3c7; }
    100% { background: transparent; }
}

.copy-flash {
    animation: copy-flash 300ms ease-out;
}

.copyTooltip {
    position: absolute;
    top: -0.25rem;
    right: 0.5rem;
    background: #44403c;
    color: #fff;
    font-family: 'IBM Plex Mono', monospace;
    font-size: 0.6875rem;
    padding: 0.1875rem 0.5rem;
    border-radius: 4px;
    pointer-events: none;
    opacity: 0;
    transition: opacity 150ms;
    z-index: 10;
    white-space: nowrap;
}

.copyTooltip.show {
    opacity: 1;
}
```

- [ ] **Step 2: Rebuild CSS**

Run: `npm run build-css`
Expected: Clean exit, no errors.

- [ ] **Step 3: Commit**

```bash
git add public/jsonTable.css
git commit -m "style: restyle metadata table CSS to match amber/stone theme"
```

---

### Task 2: Update json.tpl Template

Replace the fullscreen button with a header bar containing the section title and an icon toggle.

**Files:**
- Modify: `templates/partials/json.tpl` (entire file)
- Modify: `templates/displayGroup.tpl:67-70`
- Modify: `templates/displayResource.tpl:269-272`
- Modify: `templates/displayNote.tpl:62-65`

- [ ] **Step 1: Replace json.tpl with the new template**

Replace the entire contents of `templates/partials/json.tpl` with:

```html
<div
        class="tableContainer flex gap-3 flex-col"
        x-cloak
        :class="expanded && 'expanded'"
        x-data="
            () => ({
                jsonData: {{ jsonData|json }},
                keys: '{{ keys }}' ,
                expanded: false,
            })
        "
        x-effect="document.body.classList.toggle('overflow-hidden', expanded)"
        @click="(e) => {if(!e.shiftKey) return; expanded = !expanded; e.preventDefault();}"
>
    <div class="metaHeader">
        <h2 class="sidebar-group-title">{{ metaTitle|default:"Meta Data" }}</h2>
        <button
                x-show="jsonData && (Array.isArray(jsonData) ? jsonData.length : Object.keys(jsonData).length)"
                class="metaExpandBtn"
                @click.prevent="expanded = !expanded"
                :aria-label="expanded ? 'Minimize metadata view' : 'Expand metadata to fullscreen'"
                :aria-expanded="expanded.toString()"
        >
            <template x-if="!expanded">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M2 6V2h4M14 6V2h-4M2 10v4h4M14 10v4h-4"/></svg>
            </template>
            <template x-if="expanded">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M6 2H2v4M10 2h4v4M6 14H2v-4M10 14h4v-4"/></svg>
            </template>
            <span x-text="expanded ? 'Minimize' : 'Expand'"></span>
        </button>
    </div>
    <div class="metaTableInner" x-init="$el.appendChild(renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData))"></div>
</div>
```

- [ ] **Step 2: Remove sideTitle.tpl include from displayGroup.tpl**

In `templates/displayGroup.tpl`, change lines 67-70 from:

```html
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
        {% include "/partials/json.tpl" with jsonData=group.Meta %}
    </div>
```

to:

```html
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=group.Meta %}
    </div>
```

- [ ] **Step 3: Remove sideTitle.tpl include from displayResource.tpl**

In `templates/displayResource.tpl`, change lines 269-272 from:

```html
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
        {% include "/partials/json.tpl" with jsonData=resource.Meta %}
    </div>
```

to:

```html
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=resource.Meta %}
    </div>
```

- [ ] **Step 4: Remove sideTitle.tpl include from displayNote.tpl**

In `templates/displayNote.tpl`, change lines 62-65 from:

```html
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
        {% include "/partials/json.tpl" with jsonData=note.Meta %}
    </div>
```

to:

```html
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=note.Meta %}
    </div>
```

- [ ] **Step 5: Commit**

```bash
git add templates/partials/json.tpl templates/displayGroup.tpl templates/displayResource.tpl templates/displayNote.tpl
git commit -m "feat: integrate metadata header with expand toggle into json.tpl"
```

---

### Task 3: Add Type Detection Helpers to tableMaker.js

Add helper functions for detecting and rendering typed values. This task adds the helpers only — the next task wires them into the rendering pipeline.

**Files:**
- Modify: `src/tableMaker.js` (add functions after existing `escapeKey` function, before `getAllKeysFromObjArray`)

- [ ] **Step 1: Add type detection and rendering helpers**

Add the following code in `src/tableMaker.js` after the `escapeKey` function (after line 269) and before `getAllKeysFromObjArray` (line 277):

```javascript
// ===== Type detection helpers =====

const BOOLEAN_KEY_PATTERN = /^(active|enabled|disabled|visible|hidden|deleted|verified|published|is_|has_|can_|show_)/;

/**
 * Detect if a numeric value is likely a timestamp.
 * Values > 1e11 are milliseconds, values between 1e9 and 1e11 are seconds.
 */
function isTimestamp(val) {
    return typeof val === "number" && val > 1e9 && val < 1e13;
}

/**
 * Detect if a key + value pair represents a boolean-like field.
 * Only triggers for 0/1 values when the key matches known patterns.
 */
function isBooleanLike(key, val) {
    return (val === 0 || val === 1) && BOOLEAN_KEY_PATTERN.test(key);
}

/**
 * Detect if a key represents an ID field.
 */
function isIdKey(key) {
    return key === "id" || key === "parent" || key.endsWith("_id");
}

/**
 * Format a timestamp value as a human-readable date string.
 * Values > 1e11 treated as milliseconds, otherwise seconds.
 */
function formatTimestamp(val) {
    const ms = val > 1e11 ? val : val * 1000;
    const date = new Date(ms);
    const now = new Date();
    const diffMs = Math.abs(now - date);
    const isRecent = diffMs < 86400000; // 24 hours
    const hasTime = date.getHours() !== 0 || date.getMinutes() !== 0;

    if (isRecent || hasTime) {
        return date.toLocaleDateString(undefined, {
            year: 'numeric', month: 'short', day: 'numeric',
            hour: 'numeric', minute: 'numeric',
        });
    }

    return date.toLocaleDateString(undefined, {
        year: 'numeric', month: 'short', day: 'numeric',
    });
}

/**
 * Truncate a URL for display: strip protocol, show domain + path start.
 */
function truncateUrl(url) {
    try {
        const u = new URL(url);
        const host = u.hostname.replace(/^www\./, '');
        const path = u.pathname;
        const display = host + (path.length > 1 ? path : '');
        return display.length > 40 ? display.substring(0, 37) + '...' : display;
    } catch {
        return url;
    }
}

// ===== Type-aware element creators =====

function createDateElement(val) {
    const span = document.createElement("span");
    span.className = "metaVal--date";
    span.textContent = formatTimestamp(val);
    span.title = new Date(val > 1e11 ? val : val * 1000).toISOString();
    return span;
}

function createBoolElement(val) {
    const span = document.createElement("span");
    span.className = "metaVal--bool";

    const dot = document.createElement("span");
    dot.className = "metaVal--bool-dot " + (val ? "metaVal--bool-dot--on" : "metaVal--bool-dot--off");

    span.appendChild(dot);
    span.appendChild(document.createTextNode(val ? " yes" : " no"));
    return span;
}

function createIdElement(val) {
    const span = document.createElement("span");
    span.className = "metaVal--id";
    span.textContent = String(val);
    return span;
}

function createUrlElement(url) {
    const a = document.createElement("a");
    a.className = "metaVal--url";
    a.href = url;
    a.title = url;
    a.textContent = truncateUrl(url);
    a.target = "_blank";
    a.rel = "noopener noreferrer";
    a.addEventListener("click", (e) => e.stopPropagation()); // don't trigger copy
    return a;
}

```

- [ ] **Step 2: Build JS to verify no syntax errors**

Run: `npm run build-js`
Expected: Clean exit, no errors.

- [ ] **Step 3: Commit**

```bash
git add src/tableMaker.js
git commit -m "feat: add type detection and rendering helpers to tableMaker"
```

---

### Task 4: Wire Type-Aware Rendering into the Table Pipeline

Modify `generateObjectTable` to use type detection when rendering values. Replace emoji togglers with styled text buttons.

**Files:**
- Modify: `src/tableMaker.js` — `renderJsonTable` function (lines 6-32), `generateObjectTable` function (lines 34-131)

- [ ] **Step 1: Update renderJsonTable to accept a key parameter**

Replace the `renderJsonTable` function (lines 6-32) with:

```javascript
export function renderJsonTable(data, path = ["$"], key = "") {
    if (Array.isArray(data)) {
        return generateArrayTable(data, path);
    }

    if (data instanceof Date) {
        return createDateElement(data.getTime());
    }

    if (typeof data === "object" && data !== undefined && data !== null) {
        return generateObjectTable(data, path);
    }

    // --- Type-aware rendering for primitives ---

    // Boolean
    if (typeof data === "boolean") {
        return createBoolElement(data);
    }

    // Boolean-like (0/1 with matching key name)
    if (isBooleanLike(key, data)) {
        return createBoolElement(!!data);
    }

    // Timestamp
    if (isTimestamp(data)) {
        return createDateElement(data);
    }

    // ID field
    if (isIdKey(key) && (typeof data === "number" || typeof data === "string")) {
        return createIdElement(data);
    }

    if (typeof data === "string") {
        // data: URI image
        if (data.indexOf("data:image") === 0) {
            const img = document.createElement("img");
            img.src = data;
            return img;
        }

        // URL
        if (data.startsWith("http://") || data.startsWith("https://")) {
            return createUrlElement(data);
        }

        // Long string → expandable text
        const node = document.createElement("expandable-text");
        node.innerHTML = escapeHTML(data);
        return node;
    }

    return (
        data !== null
        && data !== undefined
        && typeof data.toString === "function"
            ? data.toString()
            : " "
    );
}
```

- [ ] **Step 2: Update generateObjectTable to pass key, use new togglers, and add copy feedback**

Replace the `generateObjectTable` function (lines 34-131) with:

```javascript
function generateObjectTable(obj, path = ["$"]) {
    const table = document.createElement("table");
    const tbody = document.createElement("tbody");
    const objKeys = Object.keys(obj || {});

    if (!obj || objKeys.length === 0) {
        table.classList.add("emptyTable");
        return table;
    }

    table.classList.add("objectTable", "jsonTable");
    table.appendChild(tbody);

    // Event delegation for copy-on-click with flash + tooltip feedback
    table.addEventListener("click", (e) => {
        if (e.target.closest("button") || e.target.closest("expandable-text") || e.target.closest("a")) {
            return;
        }
        const titled = findTitledAncestor(e.target, table);
        if (titled) {
            updateClipboard(titled.title);
            e.stopPropagation();

            // Flash animation
            const cell = e.target.closest("th, td") || titled;
            cell.classList.remove("copy-flash");
            // Force reflow so re-adding the class restarts the animation
            void cell.offsetWidth;
            cell.classList.add("copy-flash");
            cell.addEventListener("animationend", () => cell.classList.remove("copy-flash"), { once: true });

            // Tooltip
            const existing = cell.querySelector(".copyTooltip");
            if (existing) existing.remove();
            const tooltip = document.createElement("div");
            tooltip.className = "copyTooltip";
            tooltip.textContent = "Copied!";
            tooltip.setAttribute("role", "status");
            tooltip.setAttribute("aria-live", "polite");
            cell.appendChild(tooltip);
            requestAnimationFrame(() => tooltip.classList.add("show"));
            setTimeout(() => {
                tooltip.classList.remove("show");
                setTimeout(() => tooltip.remove(), 150);
            }, 1500);
        }
    });

    objKeys.forEach(key => {
        const row = tbody.insertRow(-1);
        const header = document.createElement("th");
        const subPath = [...path, escapeKey(key)];
        const pathText = subPath.join("");
        const content = renderJsonTable(obj[key], subPath, key);

        row.appendChild(header);
        header.innerHTML = escapeHTML(key);
        header.title = pathText;
        row.title = pathText;

        if (typeof content === "string") {
            const contentCell = row.insertCell();
            contentCell.innerHTML = escapeHTML(content);

        } else if (content?.matches?.("expandable-text") || content instanceof HTMLElement && !content.matches?.("table")) {
            const contentCell = row.insertCell();
            contentCell.appendChild(content);

        } else if (content instanceof HTMLElement && content.matches?.("table")) {
            row.classList.add("hasSubTable");
            content.classList.add("subTable");
            content.title = pathText;
            header.colSpan = 2;

            // Determine label for the toggler
            const val = obj[key];
            let toggleLabel;
            if (Array.isArray(val)) {
                toggleLabel = val.length === 0 ? "empty" : `${val.length} item${val.length !== 1 ? 's' : ''}`;
            } else {
                const keyCount = Object.keys(val || {}).length;
                toggleLabel = keyCount === 0 ? "empty" : `${keyCount} key${keyCount !== 1 ? 's' : ''}`;
            }

            const toggler = document.createElement("button");
            toggler.title = "Click to expand/collapse, shift-click to expand/collapse all subtables";
            toggler.classList.add("metaToggler");
            toggler.textContent = `${toggleLabel} \u2014 show`;
            toggler.tabIndex = 0;
            toggler.setAttribute("aria-expanded", "false");

            const listener = (e) => {
                e.preventDefault();
                e.stopPropagation();

                const isHidden = content.classList.toggle("hidden");

                if (e.shiftKey) {
                    const subTables = content.querySelectorAll(".subTable");
                    subTables.forEach(st => {
                        st.classList.toggle("hidden", isHidden);
                        const prevToggler = st.previousElementSibling;
                        if (prevToggler && prevToggler.matches(".metaToggler")) {
                            const prevLabel = prevToggler.textContent.split(" \u2014 ")[0];
                            prevToggler.textContent = `${prevLabel} \u2014 ${isHidden ? 'show' : 'hide'}`;
                            prevToggler.classList.toggle("expanded", !isHidden);
                            prevToggler.setAttribute("aria-expanded", String(!isHidden));
                        }
                    });
                }

                toggler.textContent = `${toggleLabel} \u2014 ${isHidden ? 'show' : 'hide'}`;
                toggler.classList.toggle("expanded", !isHidden);
                toggler.setAttribute("aria-expanded", String(!isHidden));
            };

            toggler.addEventListener("click", listener);
            toggler.addEventListener("keydown", (e) => {
                if (e.key === "Enter" || e.key === " ") {
                    listener(e);
                }
            });

            content.classList.add("hidden");
            header.appendChild(toggler);
            header.appendChild(content);

        } else {
            // Fallback for string returns from renderJsonTable
            const contentCell = row.insertCell();
            contentCell.textContent = typeof content === "string" ? content : "";
        }
    });

    return table;
}
```

- [ ] **Step 3: Update generateArrayTable copy handler to match**

In `generateArrayTable`, replace the existing event delegation block (the `table.addEventListener("click", ...)` at lines ~146-155) with the same copy-feedback handler:

```javascript
    // Event delegation for copy-on-click with flash + tooltip feedback
    table.addEventListener("click", (e) => {
        if (e.target.closest("button") || e.target.closest("expandable-text") || e.target.closest("a")) {
            return;
        }
        const titled = findTitledAncestor(e.target, table);
        if (titled) {
            updateClipboard(titled.title);
            e.stopPropagation();

            const cell = e.target.closest("th, td") || titled;
            cell.classList.remove("copy-flash");
            void cell.offsetWidth;
            cell.classList.add("copy-flash");
            cell.addEventListener("animationend", () => cell.classList.remove("copy-flash"), { once: true });

            const existing = cell.querySelector(".copyTooltip");
            if (existing) existing.remove();
            const tooltip = document.createElement("div");
            tooltip.className = "copyTooltip";
            tooltip.textContent = "Copied!";
            tooltip.setAttribute("role", "status");
            tooltip.setAttribute("aria-live", "polite");
            cell.appendChild(tooltip);
            requestAnimationFrame(() => tooltip.classList.add("show"));
            setTimeout(() => {
                tooltip.classList.remove("show");
                setTimeout(() => tooltip.remove(), 150);
            }, 1500);
        }
    });
```

- [ ] **Step 4: Remove the old plusEmoji/minusEmoji constants**

Delete lines 3-4 at the top of `src/tableMaker.js`:

```javascript
const plusEmoji = "➕";
const minusEmoji = "➖";
```

- [ ] **Step 5: Build JS to verify**

Run: `npm run build-js`
Expected: Clean exit, no errors.

- [ ] **Step 6: Commit**

```bash
git add src/tableMaker.js
git commit -m "feat: wire type-aware rendering and new togglers into metadata table"
```

---

### Task 5: Redesign expandable-text Web Component

Restyle the `expandable-text` web component to match the app's design language.

**Files:**
- Modify: `src/webcomponents/expandabletext.js` (entire file)

- [ ] **Step 1: Replace expandabletext.js with the redesigned component**

Replace the entire contents of `src/webcomponents/expandabletext.js` with:

```javascript
import { updateClipboard } from '../index.js';

class ExpandableText extends HTMLElement {
    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.uniqueId = `expandable-${Math.random().toString(36).substring(2, 11)}`;

        const style = document.createElement('style');
        style.textContent = `
          :host {
            display: inline;
          }
          .container {
            font-family: 'IBM Plex Mono', monospace;
            color: #292524;
          }
          .ellipsis {
            color: #a8a29e;
          }
          .toggle {
            font-family: 'IBM Plex Mono', monospace;
            font-size: 0.6875rem;
            color: #b45309;
            background: none;
            border: none;
            cursor: pointer;
            margin-left: 0.375rem;
            padding: 0;
            font-weight: 500;
          }
          .toggle:hover {
            text-decoration: underline;
          }
          .toggle:focus-visible {
            outline: 2px solid #b45309;
            outline-offset: 2px;
            border-radius: 2px;
          }
          .toggle:focus:not(:focus-visible) {
            outline: none;
          }
          .copy-btn {
            font-size: 0;
            color: #a8a29e;
            background: none;
            border: none;
            cursor: pointer;
            margin-left: 0.375rem;
            padding: 0.125rem;
            opacity: 0;
            transition: opacity 120ms;
            vertical-align: middle;
          }
          :host(:hover) .copy-btn,
          .copy-btn:focus-visible {
            opacity: 1;
          }
          .copy-btn:hover {
            color: #57534e;
          }
          .copy-btn:focus-visible {
            outline: 2px solid #b45309;
            outline-offset: 2px;
            border-radius: 2px;
            opacity: 1;
          }
          .copy-btn:focus:not(:focus-visible) {
            outline: none;
          }
        `;
        this.shadowRoot.appendChild(style);

        // Hidden slot suppresses light DOM from the accessibility tree
        const hiddenSlot = document.createElement('slot');
        hiddenSlot.style.display = 'none';
        hiddenSlot.setAttribute('aria-hidden', 'true');
        this.shadowRoot.appendChild(hiddenSlot);
    }

    disconnectedCallback() {
        if (this._container) {
            this._container.remove();
            this._container = null;
        }
    }

    connectedCallback() {
        if (this._container) return;

        const container = document.createElement('span');
        container.setAttribute('class', 'container');
        this._container = container;

        const fullText = this.textContent.trim();

        const previewSpan = document.createElement('span');
        const fullTextSpan = document.createElement('span');
        fullTextSpan.id = this.uniqueId;
        fullTextSpan.textContent = fullText;
        fullTextSpan.style.display = 'none';
        fullTextSpan.setAttribute('aria-hidden', 'true');

        if (fullText.length > 30) {
            previewSpan.textContent = fullText.substring(0, 30);

            const ellipsis = document.createElement('span');
            ellipsis.className = 'ellipsis';
            ellipsis.textContent = '...';
            previewSpan.appendChild(ellipsis);
        } else {
            previewSpan.textContent = fullText;
        }

        container.appendChild(previewSpan);
        container.appendChild(fullTextSpan);

        if (fullText.length > 30) {
            const toggleBtn = document.createElement('button');
            toggleBtn.type = 'button';
            toggleBtn.className = 'toggle';
            toggleBtn.textContent = 'show more';
            toggleBtn.setAttribute('aria-expanded', 'false');
            toggleBtn.setAttribute('aria-controls', this.uniqueId);

            toggleBtn.addEventListener('click', () => {
                const isExpanded = fullTextSpan.style.display !== 'none';
                if (!isExpanded) {
                    fullTextSpan.style.display = 'inline';
                    fullTextSpan.setAttribute('aria-hidden', 'false');
                    previewSpan.style.display = 'none';
                    toggleBtn.textContent = 'show less';
                    toggleBtn.setAttribute('aria-expanded', 'true');
                } else {
                    fullTextSpan.style.display = 'none';
                    fullTextSpan.setAttribute('aria-hidden', 'true');
                    previewSpan.style.display = 'inline';
                    toggleBtn.textContent = 'show more';
                    toggleBtn.setAttribute('aria-expanded', 'false');
                }
            });

            const copyBtn = document.createElement('button');
            copyBtn.type = 'button';
            copyBtn.className = 'copy-btn';
            copyBtn.setAttribute('aria-label', 'Copy text to clipboard');
            copyBtn.innerHTML = '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="5" y="5" width="9" height="9" rx="1.5"/><path d="M5 11H3.5A1.5 1.5 0 012 9.5V3.5A1.5 1.5 0 013.5 2h6A1.5 1.5 0 0111 3.5V5"/></svg>';
            copyBtn.addEventListener('click', () => {
                updateClipboard(fullText);
            });

            container.appendChild(toggleBtn);
            container.appendChild(copyBtn);
        }

        this.shadowRoot.appendChild(container);
    }
}

customElements.define('expandable-text', ExpandableText);
```

- [ ] **Step 2: Build JS to verify**

Run: `npm run build-js`
Expected: Clean exit, no errors.

- [ ] **Step 3: Commit**

```bash
git add src/webcomponents/expandabletext.js
git commit -m "feat: redesign expandable-text component with app-matched styling"
```

---

### Task 6: Full Build and Manual Smoke Test

Build everything and verify the metadata display renders correctly.

**Files:** None (verification only)

- [ ] **Step 1: Full build**

Run: `npm run build`
Expected: CSS, JS, and Go binary all build cleanly.

- [ ] **Step 2: Start ephemeral server**

Run (in a separate terminal):
```bash
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2
```

- [ ] **Step 3: Verify metadata renders on a group page**

Open `http://localhost:8181` in a browser. Create a group with metadata containing various types: dates (epoch numbers), booleans, IDs, URLs, nested objects, arrays. Verify:

1. The "Meta Data" header appears with the expand button to its right
2. Epoch numbers render as formatted dates (e.g., "Jun 14, 2021")
3. Boolean values show colored dot + "yes"/"no"
4. IDs show in muted style
5. URLs show as amber links
6. Empty arrays/objects show "empty — show" button
7. Non-empty arrays show "N items — show" button
8. Clicking "show" expands the nested content
9. Shift-clicking expands all nested tables
10. Click any cell → amber flash + "Copied!" tooltip
11. Long strings truncate with "show more" link
12. Expand button toggles fullscreen overlay
13. Keyboard navigation (Tab, Enter/Space) works on all buttons

- [ ] **Step 4: Commit (no changes expected — verification only)**

---

### Task 7: Update E2E Tests

Update the existing E2E test that relies on `.toggler` class and emoji content, and add a test for type-aware rendering.

**Files:**
- Modify: `e2e/tests/24-json-table-copy.spec.ts`

- [ ] **Step 1: Update existing test selectors**

The test at `e2e/tests/24-json-table-copy.spec.ts` uses `.jsonTable` selectors which are still valid (the class name hasn't changed). Review the test — it clicks on `td` with text `42` and verifies clipboard. The number `42` is a plain number (not a timestamp, not boolean-like), so it will render as-is. The test should still pass without changes.

Run: `cd e2e && npm run test:with-server -- --grep "JSON Table Copy"`
Expected: All 3 tests pass.

- [ ] **Step 2: Add type-aware rendering E2E test**

Add a new test file `e2e/tests/metadata-display-types.spec.ts`:

```typescript
/**
 * Tests for type-aware metadata rendering in the redesigned metadata table.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Metadata Display Type Rendering', () => {
  let categoryId: number;
  let groupId: number;
  const testRunId = Date.now();

  const testMeta = {
    id: 3360,
    data: 1623661253000,         // millisecond timestamp → date
    name: 'testuser',
    active: 1,                   // boolean-like key
    is_verified: 0,              // boolean-like key
    parent_id: 1948,             // ID field
    count: 42,                   // plain number (not timestamp, not boolean)
    website: 'https://example.com/user/profile',
    tags: [],                    // empty array
    settings: { theme: 'dark', lang: 'en' },  // non-empty object
    bio: 'This is a long biography text that should be truncated after thirty characters',
  };

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Meta Type Test Category ${testRunId}`,
      'Category for metadata type rendering test'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `Meta Type Test Group ${testRunId}`,
      description: 'Group for metadata type rendering test',
      categoryId: category.ID,
    });
    groupId = group.ID;

    const formData = new URLSearchParams();
    formData.append('ID', group.ID.toString());
    formData.append('Name', `Meta Type Test Group ${testRunId}`);
    formData.append('categoryId', category.ID.toString());
    formData.append('Meta', JSON.stringify(testMeta));

    const response = await apiClient['request'].post(
      `${apiClient['baseUrl']}/v1/group`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: formData.toString(),
      }
    );
    expect(response.ok()).toBeTruthy();
  });

  test('timestamps render as formatted dates', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();
    await expect(table).toBeVisible();

    // The epoch 1623661253000 is Jun 14, 2021 — should render as date text, not raw number
    const dataCell = table.locator('tr', { has: page.locator('th', { hasText: /^data$/ }) }).locator('td .metaVal--date');
    await expect(dataCell).toBeVisible();
    await expect(dataCell).toContainText('2021');
    // Raw number should NOT appear
    await expect(table.locator('td', { hasText: '1623661253000' })).toHaveCount(0);
  });

  test('boolean-like fields render with dot indicator', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    // "active: 1" should render as boolean (green dot + "yes")
    const activeRow = table.locator('tr', { has: page.locator('th', { hasText: /^active$/ }) });
    const activeBool = activeRow.locator('.metaVal--bool');
    await expect(activeBool).toBeVisible();
    await expect(activeBool).toContainText('yes');
    await expect(activeRow.locator('.metaVal--bool-dot--on')).toBeVisible();

    // "is_verified: 0" should render as boolean (gray dot + "no")
    const verifiedRow = table.locator('tr', { has: page.locator('th', { hasText: /^is_verified$/ }) });
    const verifiedBool = verifiedRow.locator('.metaVal--bool');
    await expect(verifiedBool).toBeVisible();
    await expect(verifiedBool).toContainText('no');
    await expect(verifiedRow.locator('.metaVal--bool-dot--off')).toBeVisible();
  });

  test('ID fields render in muted style', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const idCell = table.locator('tr', { has: page.locator('th', { hasText: /^id$/ }) }).locator('.metaVal--id');
    await expect(idCell).toBeVisible();
    await expect(idCell).toContainText('3360');

    const parentCell = table.locator('tr', { has: page.locator('th', { hasText: /^parent_id$/ }) }).locator('.metaVal--id');
    await expect(parentCell).toBeVisible();
    await expect(parentCell).toContainText('1948');
  });

  test('URLs render as clickable links', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const urlLink = table.locator('.metaVal--url');
    await expect(urlLink).toBeVisible();
    await expect(urlLink).toHaveAttribute('href', 'https://example.com/user/profile');
    await expect(urlLink).toContainText('example.com');
  });

  test('empty arrays show "empty — show" button', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const tagsRow = table.locator('tr', { has: page.locator('th', { hasText: /^tags$/ }) });
    const toggler = tagsRow.locator('.metaToggler');
    await expect(toggler).toBeVisible();
    await expect(toggler).toContainText('empty');
    await expect(toggler).toContainText('show');
  });

  test('non-empty objects show "N keys — show" and expand on click', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const settingsRow = table.locator('tr', { has: page.locator('th', { hasText: /^settings$/ }) });
    const toggler = settingsRow.locator('.metaToggler');
    await expect(toggler).toBeVisible();
    await expect(toggler).toContainText('2 keys');
    await expect(toggler).toContainText('show');

    // Click to expand
    await toggler.click();
    await expect(toggler).toContainText('hide');
    await expect(toggler).toHaveClass(/expanded/);

    // Nested table should be visible
    const nestedTable = settingsRow.locator('.jsonTable');
    await expect(nestedTable).toBeVisible();
    await expect(nestedTable.locator('th', { hasText: 'theme' })).toBeVisible();
  });

  test('expand button toggles fullscreen', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);

    const expandBtn = page.locator('.metaExpandBtn');
    await expect(expandBtn).toBeVisible();
    await expect(expandBtn).toContainText('Expand');

    await expandBtn.click();

    const container = page.locator('.tableContainer');
    await expect(container).toHaveClass(/expanded/);
    await expect(expandBtn).toContainText('Minimize');

    // Click minimize
    await expandBtn.click();
    await expect(container).not.toHaveClass(/expanded/);
  });

  test('plain numbers are not converted to dates or booleans', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    // count: 42 should render as plain text, not a date and not a boolean
    const countRow = table.locator('tr', { has: page.locator('th', { hasText: /^count$/ }) });
    await expect(countRow.locator('.metaVal--date')).toHaveCount(0);
    await expect(countRow.locator('.metaVal--bool')).toHaveCount(0);
    await expect(countRow.locator('td')).toContainText('42');
  });

  test('copy-on-click shows flash and tooltip', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto(`/group?id=${groupId}`);

    const table = page.locator('.jsonTable').first();
    // Click on count value cell (42) — plain number, renders as text in td
    const countCell = table.locator('tr', { has: page.locator('th', { hasText: /^count$/ }) }).locator('td');
    await countCell.click();

    // Tooltip should appear
    const tooltip = countCell.locator('.copyTooltip');
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toHaveText('Copied!');

    // Verify clipboard
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe('$.count');
  });

  test.afterAll(async ({ apiClient }) => {
    try {
      if (groupId) await apiClient.deleteGroup(groupId);
    } catch { /* ignore */ }
    try {
      if (categoryId) await apiClient.deleteCategory(categoryId);
    } catch { /* ignore */ }
  });
});
```

- [ ] **Step 3: Run the new E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Metadata Display Type"`
Expected: All tests pass.

- [ ] **Step 4: Run the existing JSON table copy tests to verify no regression**

Run: `cd e2e && npm run test:with-server -- --grep "JSON Table Copy"`
Expected: All 3 existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add e2e/tests/metadata-display-types.spec.ts
git commit -m "test: add E2E tests for type-aware metadata rendering"
```

---

### Task 8: Run Full Test Suite

Run all tests to verify nothing is broken.

**Files:** None (verification only)

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All tests pass.

- [ ] **Step 2: Run all E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All tests pass.

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: All tests pass.

- [ ] **Step 4: Fix any failures**

If any tests fail, investigate and fix. Common issues to watch for:
- Selectors that referenced `.toggler` class (now `.metaToggler`)
- Tests checking for emoji content (`➕`/`➖`)
- Tests that relied on the "Fullscreen" button text (now "Expand")
