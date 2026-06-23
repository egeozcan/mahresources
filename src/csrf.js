/**
 * CSRF synchronizer-token wiring (defense-in-depth atop the SameSite=Lax session
 * cookie). Adds the per-session token, published in a <meta name="csrf-token">
 * tag, to same-origin state-changing requests so the server can verify them:
 *
 *   - fetch() requests get an `X-CSRF-Token` header.
 *   - native form submits get a hidden `csrf_token` field (urlencoded forms,
 *     where the token travels in the body) or a `csrf_token` query parameter
 *     (multipart upload forms, whose body the server cannot read without
 *     defeating the upload size limit).
 *
 * Entirely a no-op when auth is disabled: the meta tag renders empty, so nothing
 * is attached and behaviour matches the historical no-auth deployment.
 */

const UNSAFE = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

function csrfToken() {
  const el = document.querySelector('meta[name="csrf-token"]');
  return (el && el.getAttribute('content')) || '';
}

function isSameOrigin(url) {
  try {
    return new URL(url, window.location.origin).origin === window.location.origin;
  } catch {
    return true; // relative URLs resolve same-origin
  }
}

function methodOf(input, init) {
  if (init && init.method) return init.method.toUpperCase();
  if (input && typeof input !== 'string' && input.method) return input.method.toUpperCase();
  return 'GET';
}

function urlOf(input) {
  if (typeof input === 'string') return input;
  if (input instanceof Request) return input.url;
  if (input instanceof URL) return input.toString();
  return String(input);
}

// Wrap fetch so every same-origin unsafe request carries the token header.
// Spreading init preserves signal/body/etc.; we only ever add the header.
const nativeFetch = window.fetch.bind(window);
window.fetch = function (input, init) {
  const token = csrfToken();
  if (token) {
    const method = methodOf(input, init);
    if (UNSAFE.has(method) && isSameOrigin(urlOf(input))) {
      init = init ? { ...init } : {};
      const headers = new Headers(
        init.headers !== undefined
          ? init.headers
          : input instanceof Request
          ? input.headers
          : undefined
      );
      if (!headers.has('X-CSRF-Token')) headers.set('X-CSRF-Token', token);
      init.headers = headers;
    }
  }
  return nativeFetch(input, init);
};

// Native form submits: inject the token. Capture phase so it runs before any
// framework submit handlers, but it is harmless if the form is later submitted
// via fetch (the wrapper above adds the header regardless).
document.addEventListener(
  'submit',
  (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    const method = (form.getAttribute('method') || 'get').toUpperCase();
    if (!UNSAFE.has(method)) return;
    const token = csrfToken();
    if (!token) return;
    const action = form.getAttribute('action') || window.location.href;
    if (!isSameOrigin(action)) return;

    const enctype = (form.enctype || form.getAttribute('enctype') || '').toLowerCase();
    if (enctype === 'multipart/form-data') {
      // The server does not read multipart bodies for the token (it would defeat
      // the streaming upload size limit), so pass it as a query parameter.
      const url = new URL(action, window.location.origin);
      url.searchParams.set('csrf_token', token);
      form.setAttribute('action', url.pathname + url.search + url.hash);
    } else {
      let input = form.querySelector('input[name="csrf_token"]');
      if (!input) {
        input = document.createElement('input');
        input.type = 'hidden';
        input.name = 'csrf_token';
        form.appendChild(input);
      }
      input.value = token;
    }
  },
  true
);
