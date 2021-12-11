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

const numberCheck = /^\d+?(\.\d+?)?$/

function isNumeric(x) {
    return numberCheck.test(x) && !isNaN(parseFloat(x))
}