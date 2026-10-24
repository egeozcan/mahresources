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
 *
 * Because it replaces native Tab inside the container, only use it for dialogs
 * whose controls are plain form controls and buttons. Do not use it for a
 * dialog that hosts an editor (CodeMirror, contenteditable, where Tab can mean
 * indent), a shadow-root control, or an iframe: their own tab order is not
 * visible here and would be skipped.
 */
const CANDIDATES = 'button, [href], input, select, textarea, summary, [tabindex]';

// Open containments, innermost last. Only the last one handles events.
const stack = [];

// Disabled the way :disabled is: the element itself, or inside a disabled
// fieldset other than through that fieldset's first legend.
function isDisabled(el) {
  if (el.disabled) return true;
  for (let fieldset = el.closest('fieldset[disabled]'); fieldset; fieldset = fieldset.parentElement?.closest('fieldset[disabled]')) {
    const legend = fieldset.querySelector(':scope > legend');
    if (!legend || !legend.contains(el)) return true;
  }
  return false;
}

function isShown(el) {
  if (typeof el.checkVisibility === 'function' && el.getClientRects().length > 0) {
    return el.checkVisibility({ checkVisibilityCSS: true, visibilityProperty: true });
  }
  for (let node = el; node && node.nodeType === 1; node = node.parentElement) {
    if (getComputedStyle(node).display === 'none') return false;
  }
  return getComputedStyle(el).visibility !== 'hidden';
}

/**
 * The elements Tab visits inside root, in order, as the browser would choose
 * them: no negative tabindex, nothing disabled (a disabled fieldset included),
 * hidden, invisible or inert, and one stop per radio group (the checked radio,
 * else the first).
 */
export function tabStops(root) {
  const stops = [];
  const radioGroups = new Map();
  for (const el of root.querySelectorAll(CANDIDATES)) {
    const tabindex = el.getAttribute('tabindex');
    if (tabindex !== null && Number(tabindex) < 0) continue;
    if (el.matches('a:not([href]), area:not([href])') && tabindex === null) continue;
    if (isDisabled(el) || el.closest('[inert]') || !isShown(el)) continue;
    if (el.type === 'radio' && el.name) {
      const group = radioGroups.get(el.name);
      if (!group) {
        radioGroups.set(el.name, { index: stops.length });
        stops.push(el);
      } else if (el.checked && !stops[group.index].checked) {
        stops[group.index] = el;
      }
      continue;
    }
    stops.push(el);
  }
  return stops;
}

const stopsIn = tabStops;

// The innermost open containment still in the document; a dialog removed
// without stop() is dropped, so it cannot disable the one below it.
function topRoot() {
  while (stack.length && !stack[stack.length - 1].isConnected) stack.pop();
  return stack[stack.length - 1];
}

function onKeydown(event) {
  const root = topRoot();
  if (!root || event.key !== 'Tab') return;
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
  const root = topRoot();
  if (root && !root.contains(event.target)) stopsIn(root)[0]?.focus();
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
