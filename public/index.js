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

window.addEventListener("load", () => {
  const filter = /#image\//;
  const fullScreen = false;
  const animation = false;

  const options = { filter, fullScreen, animation }

  baguetteBox.run(".note-container", options);
  baguetteBox.run(".gallery", options);
});
