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
