function renderJsonTable(data, path = ["$"]) {
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
        return data;
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

    table.classList.add("objectTable", "jsonTable");
    table.appendChild(tbody);

    if (!obj) {
        return table;
    }

    Object.keys(obj).forEach(key => {
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
        } else {
            row.classList.add("hasSubTable");
            content.classList.add("subTable");
            addCopyListener(content, pathText);
            header.colSpan = 2;
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
        return table;
    }

    const firstRow = arr[0];
    const titles = Object.keys(firstRow);

    if (
        !isRenderableAsArray(firstRow)
        || arr.some(el => !isRenderableAsArray(el, titles))
    ) {
        arr.forEach((el, i) => {
            const row = tbody.insertRow();
            const contentCell = row.insertCell();
            const subPath = [...path, escapeKey(i)];
            const pathText = subPath.join("");
            const content = renderJsonTable(el, subPath);
            addCopyListener(row, pathText);

            if (typeof content === "string") {
                contentCell.innerHTML = escapeHTML(content);
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
            } else {
                contentCell.appendChild(content);
                addCopyListener(content, pathText);
            }
        });
    });

    return table;
}

function isRenderableAsArray(obj, mustHaveExactlyTheseKeys = []) {
    if (Array.isArray(obj) || typeof obj !== "object" || obj instanceof Date) {
        return false;
    }

    if (mustHaveExactlyTheseKeys.length === 0) {
        return true;
    }

    const ownKeys = Object.keys(obj);

    return (
        ownKeys.length === mustHaveExactlyTheseKeys.length
        && ownKeys.every(key => mustHaveExactlyTheseKeys.indexOf(key) >= 0)
        && mustHaveExactlyTheseKeys.every(key => ownKeys.indexOf(key) >= 0)
    );
}

function escapeHTML(str) {
    if (str === " ") {
        return "&nbsp;";
    }

    if (str.indexOf("data:image") === 0) {
        return "<img src='" + str + "' />";
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
        updateClipboard(text);
        e.stopPropagation();
    });
}