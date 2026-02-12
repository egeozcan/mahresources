import { updateClipboard } from './index.js';

const plusEmoji = "➕";
const minusEmoji = "➖";

export function renderJsonTable(data, path = ["$"]) {
    if (Array.isArray(data)) {
        return generateArrayTable(data, path);
    }

    if (data instanceof Date) {
        return data.toLocaleDateString();
    }

    if (typeof data === "object" && data !== undefined) {
        return generateObjectTable(data, path);
    }

    if (typeof data === "string") {
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

    objKeys.forEach(key => {
        const row = tbody.insertRow(-1);
        const header = document.createElement("th");
        const subPath = [...path, escapeKey(key)];
        const pathText = subPath.join("");
        const content = renderJsonTable(obj[key], subPath);

        row.appendChild(header);
        header.innerHTML = escapeHTML(key);
        header.title = pathText;
        addCopyListener(header, pathText);
        addCopyListener(row, pathText);

        if (typeof content === "string") {
            const contentCell = row.insertCell();

            contentCell.innerHTML = escapeHTML(content);
        } else if (content?.matches?.("expandable-text")) {
            const contentCell = row.insertCell();

            contentCell.appendChild(content);

        } else {
            row.classList.add("hasSubTable");
            content.classList.add("subTable");
            addCopyListener(content, pathText);
            header.colSpan = 2;

            const toggler = document.createElement("button");

            toggler.title = "Click to expand/collapse, shift-click to expand/collapse all subtables";
            toggler.classList.add("toggler");
            toggler.innerHTML = plusEmoji;
            toggler.tabIndex = 0;

            header.appendChild(toggler);

            const listener = (e) => {
                e.preventDefault();
                e.stopPropagation();

                const isHidden = content.classList.toggle("hidden");

                // if the shift key is pressed, expand/contract all subtables
                if (e.shiftKey) {
                    const subTables = content.querySelectorAll(".subTable");

                    // expand all subtables, update the toggler emoji
                    subTables.forEach(table => {
                        table.classList.toggle("hidden", isHidden);

                        if (table.previousElementSibling && table.previousElementSibling.matches(".toggler")) {
                            table.previousElementSibling.innerHTML = isHidden ? plusEmoji : minusEmoji;
                        }
                    });
                }

                toggler.innerHTML = isHidden ? plusEmoji : minusEmoji;
            }

            toggler.addEventListener("click", listener);
            toggler.addEventListener("keydown", (e) => {
                if (e.key === "Enter" || e.key === " ") {
                    listener(e);
                }
            });

            content.classList.add("hidden");
            header.appendChild(content);
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

    const titles = getAllKeysFromObjArray(arr);

    if (arr.some(el => !isRenderableAsArray(el))) {
        arr.forEach((el, i) => {
            const row = tbody.insertRow();
            const contentCell = row.insertCell();
            const subPath = [...path, escapeKey(i)];
            const pathText = subPath.join("");
            const content = renderJsonTable(el, subPath);

            addCopyListener(row, pathText);

            if (typeof content === "string") {
                contentCell.innerHTML = escapeHTML(content);
            } else if (content?.matches?.("expandable-text")) {
                contentCell.appendChild(content);
            } else {
                addCopyListener(content, pathText);
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
        addCopyListener(row, [...path, escapeKey(idx)].join(""));

        titles.forEach(title => {
            const contentCell = row.insertCell();
            const subPath = [...path, escapeKey(idx), escapeKey(title)];
            const pathText = subPath.join("");
            addCopyListener(contentCell, pathText);
            const content = renderJsonTable(el[title], subPath);

            contentCell.classList.add(cellClass);
            contentCell.title = subPath.join("");

            if (typeof content === "string") {
                contentCell.innerHTML = escapeHTML(content);
            } else if (content?.matches?.("expandable-text")) {
                contentCell.appendChild(content);
            } else {
                contentCell.appendChild(content);
                addCopyListener(content, pathText);
            }
        });
    });

    return table;
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

/**
 *
 * @param {HTMLElement} el
 * @param {string} text
 */
function addCopyListener(el, text) {
    el.addEventListener("click", (e) => {
        // if the target is a button, ignore the click event
        if (e.target.matches("button") || e.target.matches("expandable-text")) {
            return;
        }

        updateClipboard(text);
        e.stopPropagation();
    });
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
