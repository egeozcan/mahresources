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
