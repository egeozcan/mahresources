function abortableFetch(request, opts) {
  const controller = new AbortController();
  const signal = controller.signal;

  return {
    abort: () => controller.abort(),
    ready: fetch(request, { ...opts, signal }),
  };
}

function isUndef(x) {
  return typeof x === "undefined";
}

const numberCheck = /^\d+?(\.\d+?)?$/;

function isNumeric(x) {
  return numberCheck.test(x) && !isNaN(parseFloat(x));
}

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

function setCheckBox(checkBox, checked) {
  if (checked) {
    checkBox.setAttribute("checked", "checked");
  } else {
    checkBox.removeAttribute("checked");
  }

  checkBox.checked = checked;
}

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


function parseQueryParams(query) {
  const res = {};
  const params = (query.match(/:[\w\d_]+/g) || []).map(x => x.substring(1));

  for (const param of params) {
    res[param] = "";
  }

  return res;
}