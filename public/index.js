/***
 * abortableFetch - fetch with abortable promise
 * @param request
 * @param opts
 * @returns {{abort: (function(): void), ready: Promise<Response>}}
 */
function abortableFetch(request, opts) {
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
function isUndef(x) {
  return typeof x === "undefined";
}

const numberCheck = /^\d+?(\.\d+?)?$/;

/**
 * isNumeric - check if value is numeric
 * @param x
 * @returns {boolean}
 */
function isNumeric(x) {
  return numberCheck.test(x) && !isNaN(parseFloat(x));
}

/**
 * pick - pick values from object
 * @param obj
 * @param keys
 * @returns {{[p: string]: unknown}}
 */
function pick(obj, ...keys) {
  return Object.fromEntries(
    Object.entries(obj).filter((x) => keys.includes(x[0]))
  );
}

document.addEventListener("DOMContentLoaded", () => {
  const filter = /#image\//;
  const fullScreen = false;
  const animation = false;

  const options = { filter, fullScreen, animation }

  baguetteBox.run(".list-container", options);
  baguetteBox.run(".gallery", options);
});

window.addEventListener('paste', e => {
  const fileInput = document.querySelector("input[type='file']");
  if (!fileInput || !e.clipboardData.files || !e.clipboardData.files.length) {
    return;
  }
  fileInput.files = e.clipboardData.files;
});

/**
 * setCheckBox - set checkbox value
 * @param {HTMLInputElement} checkBox
 * @param checked
 */
function setCheckBox(checkBox, checked) {
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
function updateClipboard(newClip) {
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
function parseQueryParams(queryString) {
  const res = {};
  const params = (queryString.match(/:[\w\d_]+/g) || []).map(x => x.substring(1));

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
function addMetaToGroup(id, val) {
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
function addMetaToResource(id, val) {
  return fetch("/v1/resources/addMeta", {
    method: 'POST',
    body: JSON.stringify({ id: [id], Meta: JSON.stringify(val) }),
    headers: {
      "Accept": "application/json",
      "Content-Type": "application/json",
    }
  });
}