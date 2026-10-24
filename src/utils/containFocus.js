/**
 * Keep keyboard focus inside a modal dialog, without x-trap.
 *
 * Alpine's x-trap arms its focus trap 15ms after the dialog mounts and does not
 * cancel that timer when the dialog closes first, so a dialog answered sooner
 * left a trap armed on a removed element that swallowed every Tab on the page.
 *
 * Tab and Shift+Tab are handled here, at window level in the capture phase, so
 * this runs before any other trap's document listener: a confirm raised from
 * inside a modal that has its own x-trap (mass edit, plugin actions) would
 * otherwise have that trap answer every Tab first and keep focus where it was.
 * Alpine bundles its own copy of focus-trap, so its trap stack cannot be shared
 * to pause it; taking the event before it is how the parent is paused. While a
 * containment is active the topmost one owns Tab outright: it moves focus to
 * the next or previous stop itself, wrapping at either end, and stops the event.
 * Focus that lands outside the dialog, as after a mouse click on its heading or
 * backdrop, is pulled back to it. The rest of the page is expected to be inert.
 */
const TABBABLE = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])';

// Open containments, innermost last. Only the last one handles events.
const stack = [];

function stopsIn(root) {
  return [...root.querySelectorAll(TABBABLE)].filter((el) => el.getClientRects().length > 0 && !el.closest('[inert]'));
}

function onKeydown(event) {
  const root = stack[stack.length - 1];
  if (!root || event.key !== 'Tab' || !root.isConnected) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  const stops = stopsIn(root);
  if (!stops.length) return;
  const at = stops.indexOf(document.activeElement);
  let next;
  if (at < 0) next = event.shiftKey ? stops.length - 1 : 0;
  else next = (at + (event.shiftKey ? -1 : 1) + stops.length) % stops.length;
  stops[next].focus();
}

function onFocusIn(event) {
  const root = stack[stack.length - 1];
  if (root?.isConnected && !root.contains(event.target)) stopsIn(root)[0]?.focus();
}

export function containFocus(root) {
  if (!root) return { stop() {} };
  if (!stack.length) {
    window.addEventListener('keydown', onKeydown, true);
    window.addEventListener('focusin', onFocusIn, true);
  }
  stack.push(root);
  return {
    stop() {
      const i = stack.lastIndexOf(root);
      if (i >= 0) stack.splice(i, 1);
      if (!stack.length) {
        window.removeEventListener('keydown', onKeydown, true);
        window.removeEventListener('focusin', onFocusIn, true);
      }
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
