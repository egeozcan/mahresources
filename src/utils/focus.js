/**
 * Focus management shared across the app.
 *
 * The two correct implementations of "put focus somewhere sensible" already
 * existed before this module — one in `components/downloadCockpit.js` (capture
 * the trigger, restore on close) and one in `webcomponents/reloadShortcode.js`
 * (park focus on refreshed content when the element that had it is gone) — and
 * both were private to their file, so every other dialog, overlay and
 * re-rendering region in the app did nothing at all. That is findings 4, 5, 30,
 * 35, 66, 74, 90, 97, 123 and 124 of the 2026-07-29 UI bug hunt.
 *
 * This is the extraction, not a redesign: the behaviour and the comments come
 * from those two files, which now import from here.
 */

// Anything that can already take focus on its own must not be given a tabindex:
// stamping tabindex="-1" on a link or a control would pull it out of sequential
// Tab navigation for good. Focus parking succeeds on these, so an omission here
// is not a missed opportunity — it is a control silently dropped from the Tab
// order. Err toward listing too much: over-listing only costs a parking attempt.
export const NATIVELY_FOCUSABLE = [
  'a[href]',
  'area[href]',
  'button',
  'input',
  'select',
  'textarea',
  'summary',
  'iframe',
  'object',
  'embed',
  'audio[controls]',
  'video[controls]',
  '[contenteditable]',
  '[tabindex]',
].join(', ');

// What counts as "the first thing a reader should land on" inside a panel.
// Narrower than NATIVELY_FOCUSABLE on purpose: it excludes tabindex="-1", which
// is exactly what parkFocus stamps, so a previously parked container is never
// mistaken for a control.
export const FOCUSABLE_IN_CONTAINER = [
  'button:not([disabled])',
  '[href]',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ');

/**
 * Focus `element`, borrowing a tabindex only if it needs one, and report
 * whether it actually took focus. Leaves no trace on elements that refuse.
 */
export function focusOn(element) {
  if (!element || typeof element.focus !== 'function') return false;
  const borrowed = !element.matches(NATIVELY_FOCUSABLE);
  if (borrowed) element.setAttribute('tabindex', '-1');
  element.focus({ preventScroll: true });
  if (document.activeElement === element) return true;
  // It could not take focus after all, so leave no trace on it.
  if (borrowed) element.removeAttribute('tabindex');
  return false;
}

/**
 * Put focus on or beside `element`, whatever it takes.
 *
 * A region wrapper may be display:contents, generating no box, and browsers do
 * not reliably let those take focus. Rather than guess, try it and check, then
 * work outwards. Each rung is further from the content than the last, and the
 * ladder only ends where focus already was, so it never leaves the reader worse
 * off than doing nothing.
 */
export function parkFocus(element) {
  if (!element) return;
  if (focusOn(element)) return;
  // The wrapper could not hold focus, so hand it to the first child that can. A
  // hidden or boxless child is skipped rather than swallowing the focus.
  for (const child of element.children) {
    if (focusOn(child)) return;
  }
  // Nothing inside can hold it either: a boxless wrapper whose refreshed content
  // is only text has no element child at all. Climb until something takes it,
  // which lands the reader beside the content instead of on <body>.
  for (let ancestor = element.parentElement; ancestor; ancestor = ancestor.parentElement) {
    if (focusOn(ancestor)) return;
  }
}

/**
 * The element that opened something, taken from the event that opened it.
 *
 * `currentTarget` and not `target`: a click on the icon inside a button reports
 * the icon as `target`, and focusing an SVG span is not the same as returning
 * the reader to the control they pressed. `currentTarget` is only valid during
 * dispatch, which is why this is read at open time and stored.
 */
export function captureTrigger(event) {
  return event?.currentTarget ?? null;
}

/**
 * The element that currently has focus, or null when nothing meaningful does.
 *
 * `document.activeElement` is `<body>` whenever focus is nowhere, and `<body>` is
 * not a focus *target*: focusOn would borrow a tabindex, report success, and leave
 * the reader on `<body>` — the exact state these helpers exist to avoid, plus a
 * stray `tabindex="-1"` on the body element. Callers use this to tell "the reader
 * was somewhere" from "the reader was nowhere", and fall back to something real in
 * the second case.
 *
 * Needed because a dialog opened by a keyboard shortcut has no trigger event to
 * capture: `captureTrigger` reads `currentTarget`, and a document-level keydown
 * handler calls the component's own toggle() with no event at all.
 */
export function focusedElement() {
  const active = document.activeElement;
  if (!active || active === document.body || active === document.documentElement) return null;
  return active;
}

/**
 * Send focus back to `trigger` after a dialog or overlay closes.
 *
 * Returns true if focus landed. A trigger that has been re-rendered away since
 * it was captured is not focusable — `isConnected` is the check, because
 * `.focus()` on a detached node silently does nothing and leaves the reader on
 * `<body>`. `fallback` is where to park in that case.
 */
export function restoreFocus(trigger, fallback = null) {
  if (trigger?.isConnected && focusOn(trigger)) return true;
  if (fallback?.isConnected) {
    parkFocus(fallback);
    return true;
  }
  return false;
}

/**
 * Move focus into a container that has just been revealed: its first focusable
 * descendant, or the container itself if it has none.
 */
export function focusFirstIn(container) {
  if (!container) return;
  const first = container.querySelector(FOCUSABLE_IN_CONTAINER);
  if (first && focusOn(first)) return;
  parkFocus(container);
}
