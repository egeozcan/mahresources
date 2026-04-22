import { updateClipboard } from './index.js';

export function renderJsonTable(data, path = ["$"], key = "") {
    // BH-002: null/undefined must return a Node so appendChild() is safe.
    // An empty DocumentFragment appends no children and is the least-surprising
    // "nothing to render" signal for callers like json.tpl.
    if (data === null || data === undefined) {
        return document.createDocumentFragment();
    }

    if (Array.isArray(data)) {
        return generateArrayTable(data, path);
    }

    if (data instanceof Date) {
        return createDateElement(data.getTime());
    }

    if (typeof data === "object") {
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

        } else if (content instanceof HTMLElement && content.matches?.("table")) {
            // Nested table (array or object) — collapsible
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

        } else if (content instanceof HTMLElement) {
            // Non-table element (expandable-text, typed value span, img, etc.)
            const contentCell = row.insertCell();
            contentCell.appendChild(content);

        } else {
            // Plain string fallback
            const contentCell = row.insertCell();
            if (typeof content === "string") {
                contentCell.innerHTML = escapeHTML(content);
            }
        }
    });

    return table;
}

function generateArrayTable(arr, path = ["$"]) {
    const table = document.createElement("table");
    const tbody = document.createElement("tbody");

    table.classList.add("arrayTable", "jsonTable");
    table.appendChild(tbody);

    if (arr.length === 0) {
        table.classList.add("emptyTable");
        return table;
    }

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

    const titles = getAllKeysFromObjArray(arr);

    if (arr.some(el => !isRenderableAsArray(el))) {
        arr.forEach((el, i) => {
            const row = tbody.insertRow();
            const contentCell = row.insertCell();
            const subPath = [...path, escapeKey(i)];
            const pathText = subPath.join("");
            const content = renderJsonTable(el, subPath);

            row.title = pathText;

            if (typeof content === "string") {
                contentCell.innerHTML = escapeHTML(content);
            } else if (content instanceof HTMLElement) {
                content.title = pathText;
                contentCell.appendChild(content);
            }
        });

        return table;
    }

    const titleRow = tbody.insertRow(-1);

    titles.forEach(title => {
        const header = document.createElement("th");

        header.innerHTML = escapeHTML(title);
        titleRow.appendChild(header);
    });

    arr.forEach((el, idx) => {
        const row = tbody.insertRow();
        const cellClass = idx % 2 === 0 ? "even" : "odd";
        row.title = [...path, escapeKey(idx)].join("");

        titles.forEach(title => {
            const contentCell = row.insertCell();
            const subPath = [...path, escapeKey(idx), escapeKey(title)];
            const pathText = subPath.join("");
            const content = renderJsonTable(el[title], subPath, title);

            contentCell.classList.add(cellClass);
            contentCell.title = pathText;

            if (typeof content === "string") {
                contentCell.innerHTML = escapeHTML(content);
            } else if (content instanceof HTMLElement) {
                contentCell.appendChild(content);
                content.title = pathText;
            }
        });
    });

    return table;
}

/**
 * Walk from el up through th/td/tr ancestors until we find one with a title,
 * stopping at the table boundary. This handles cases where a td has no title
 * but its parent tr does (e.g. object table value cells).
 */
function findTitledAncestor(el, table) {
    let current = el.closest("th, td, tr");
    while (current && current !== table) {
        if (current.title) return current;
        current = current.parentElement?.closest("th, td, tr");
    }
    return null;
}

function isRenderableAsArray(obj) {
    return !(Array.isArray(obj) || typeof obj !== "object" || obj instanceof Date);
}

function escapeHTML(str) {
    if (str === " ") {
        return "&nbsp;";
    }

    if (str.indexOf("data:image") === 0) {
        const img = document.createElement("img");
        img.src = str;
        return img.outerHTML;
    }

    const text = document.createTextNode(str);
    const p = document.createElement("p");

    p.appendChild(text);
    return p.innerHTML;
}

/**
 * @param {string|number} key
 * @returns {string}
 */
function escapeKey(key) {
    if (typeof key === "number") {
        return `[${key}]`
    }

    if (key.match(/^[a-z_]([a-z0-9_]+)?$/i)) {
        return `.${key}`
    }

    return `["${key.replaceAll('"', '\\"')}"]`;
}

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

/**
 * Get all keys from an array of objects
 *
 * @param arr
 * @returns {any[]}
 */
function getAllKeysFromObjArray(arr) {
    const keys = new Set();

    for (const obj of arr) {
        if (!obj || typeof obj !== "object") {
            continue;
        }

        for (const key of Object.keys(obj)) {
            keys.add(key);
        }
    }

    return Array.from(keys);
}
