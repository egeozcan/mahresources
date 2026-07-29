/**
 * abortableFetch - fetch with abortable promise
 * @param request
 * @param opts
 * @returns {{abort: (function(): void), ready: Promise<Response>}}
 */
export function abortableFetch(request, opts) {
  const controller = new AbortController();
  const signal = controller.signal;

  return {
    abort: () => controller.abort(),
    ready: fetch(request, { ...opts, signal }),
  };
}

/**
 * isUndef - check if value is undefined
 * @param x
 * @returns {boolean}
 */
export function isUndef(x) {
  return typeof x === "undefined";
}

const numberCheck = /^\d+?(\.\d+?)?$/;

/**
 * isNumeric - check if value is numeric
 * @param x
 * @returns {boolean}
 */
export function isNumeric(x) {
  return numberCheck.test(x) && !isNaN(parseFloat(x));
}

/**
 * pick - pick values from object
 * @param obj
 * @param keys
 * @returns {{[p: string]: unknown}}
 */
export function pick(obj, ...keys) {
  return Object.fromEntries(
    Object.entries(obj).filter((x) => keys.includes(x[0]))
  );
}

/**
 * setCheckBox - set checkbox value
 * @param {HTMLInputElement} checkBox
 * @param checked
 */
export function setCheckBox(checkBox, checked) {
  if (checked) {
    checkBox.setAttribute("checked", "checked");
  } else {
    checkBox.removeAttribute("checked");
  }

  checkBox.checked = checked;
}

/**
 * updateClipboard - update clipboard with text
 * @param newClip
 */
export function updateClipboard(newClip) {
  navigator.clipboard.writeText(newClip).catch(function () {
    const copyText = document.createElement("input");
    setTimeout(() => copyText.remove(), 100);
    document.body.append(copyText);
    copyText.value = newClip;
    copyText.select();
    if (!document.execCommand("copy")) {
      throw new Error("execcommand failed");
    }
  }).catch(function() {
    prompt("", newClip)
  });
}

/**
 * parseQueryParams - parse query params
 * @param queryString
 * @returns {{}}
 */
export function parseQueryParams(queryString) {
  const res = {};
  // match a colon that is not preceded or followed by another colon
  const params = (queryString.match(/(?<!:):\w+(?!:)/g) || []).map(x => x.substring(1));

  for (const param of params) {
    res[param] = "";
  }

  return res;
}

/**
 * addMeta - add meta to a group
 * @param {number} id
 * @param {object} val
 * @returns {Promise<Response>}
 */
export function addMetaToGroup(id, val) {
  return fetch("/v1/groups/addMeta", {
    method: 'POST',
    body: JSON.stringify({ id: [id], Meta: JSON.stringify(val) }),
    headers: {
      "Accept": "application/json",
      "Content-Type": "application/json",
    }
  })
}

/**
 * addMetaToResource - add meta to resource
 * @param {number} id
 * @param {object} val
 * @returns {Promise<Response>}
 */
export function addMetaToResource(id, val) {
  return fetch("/v1/resources/addMeta", {
    method: 'POST',
    body: JSON.stringify({ id: [id], Meta: JSON.stringify(val) }),
    headers: {
      "Accept": "application/json",
      "Content-Type": "application/json",
    }
  });
}

/**
 * errorMessageFromResponse - read the human message out of a failed response.
 *
 * Every API rejection answers `{"error": "..."}` to a JSON-accepting caller
 * (server/http_utils/http_helpers.go). Finding 100 of the 2026-07-29 UI bug hunt
 * is what happens when a caller skips that and renders the body: the query page
 * showed the literal text `{"error":"no such table: nonexistent_table_xyz"}`
 * under the heading "Something went wrong."
 *
 * Falls back to the raw text when the body is not the expected shape, and to the
 * status line when there is no body at all, so a proxy's HTML error page never
 * ends up quoted at the reader either.
 *
 * @param {Response} response
 * @returns {Promise<string>}
 */
export async function errorMessageFromResponse(response) {
  let text = '';
  try {
    text = await response.text();
  } catch (_) {
    text = '';
  }

  if (text) {
    try {
      const parsed = JSON.parse(text);
      const message = parsed && (parsed.error || parsed.message);
      if (typeof message === 'string' && message.trim()) {
        return message.trim();
      }
    } catch (_) {
      // Not JSON. A trimmed plain-text body is still better than nothing, but an
      // HTML document is noise, so it falls through to the status line.
      const trimmed = text.trim();
      if (trimmed && !trimmed.startsWith('<')) {
        return trimmed;
      }
    }
  }

  return `Request failed with status ${response.status}${response.statusText ? ' ' + response.statusText : ''}`;
}
