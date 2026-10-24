/**
 * Keep keyboard focus inside a modal dialog, without x-trap.
 *
 * Alpine's x-trap arms its focus trap 15ms after the dialog mounts and does not
 * cancel that timer when the dialog closes first, so a dialog answered sooner
 * left a trap armed on a removed element that swallowed every Tab on the page.
 *
 * This listens at document level, so it also holds after a mouse click on the
 * dialog's heading or backdrop has moved focus to <body>: the next Tab or
 * Shift+Tab lands on the dialog's first or last stop instead of leaving for the
 * browser's own UI, and a focus that lands outside the dialog is pulled back.
 * The rest of the page is expected to be inert as well; this covers the moves
 * inert cannot (focus on <body>, Tab past either end).
 */
const TABBABLE = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])';

function stopsIn(root) {
  return [...root.querySelectorAll(TABBABLE)].filter((el) => el.getClientRects().length > 0 && !el.closest('[inert]'));
}

export function containFocus(root) {
  if (!root) return { stop() {} };
  const onKeydown = (event) => {
    if (event.key !== 'Tab' || !root.isConnected) return;
    const stops = stopsIn(root);
    if (!stops.length) {
      event.preventDefault();
      return;
    }
    const first = stops[0];
    const last = stops[stops.length - 1];
    const active = document.activeElement;
    if (!root.contains(active)) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    } else if (event.shiftKey && active === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };
  const onFocusIn = (event) => {
    if (root.isConnected && !root.contains(event.target)) stopsIn(root)[0]?.focus();
  };
  document.addEventListener('keydown', onKeydown, true);
  document.addEventListener('focusin', onFocusIn, true);
  return {
    stop() {
      document.removeEventListener('keydown', onKeydown, true);
      document.removeEventListener('focusin', onFocusIn, true);
    },
  };
}

/**
 * Focus the first candidate that can take it: connected, enabled, painted.
 * Returns the element focused, or null.
 */
export function focusFirstAvailable(candidates) {
  for (const el of candidates) {
    if (!el?.isConnected || el.disabled || typeof el.focus !== 'function') continue;
    el.focus({ preventScroll: true });
    if (document.activeElement === el) return el;
  }
  return null;
}
