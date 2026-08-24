/**
 * Reads the per-session CSRF token published in the page's meta tag.
 *
 * Deliberately its own module: `src/csrf.js` wraps `window.fetch` and installs a
 * document-level submit listener at import time, so importing it from anywhere
 * that is not a browser (a vitest suite, say) executes those side effects. The
 * token reader itself is pure, and the bulk upload widget needs it — XHR is not
 * covered by the fetch wrapper.
 *
 * @returns {string} the token, or '' when auth is disabled (the tag renders empty).
 */
export function csrfToken() {
  const el = document.querySelector('meta[name="csrf-token"]');
  return (el && el.getAttribute('content')) || '';
}
