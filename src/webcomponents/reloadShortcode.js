// Custom element backing the [reload] category-template shortcode.
//
// The server renders an inert <reload-shortcode><button>: no token, no target.
// Which content a button refreshes is decided here, at click time, by walking up
// from the button:
//   1. the innermost <lazy-shortcode>/<details-shortcode> it sits inside, which
//      already holds the sealed token for its own body;
//   2. otherwise the [data-shortcode-region] wrapper the process_shortcodes tag
//      puts around a custom-content slot that contains a reload button, whose
//      token seals the whole slot;
//   3. otherwise the page, which is all that is left on surfaces that cannot
//      re-render a slot in place (share pages, live preview, list-header
//      carriers).
//
// Resolving at click time rather than at render time is what keeps "the most
// immediate area" meaningful: a button can arrive inside content that was itself
// fetched on demand, and it still finds whatever now encloses it.

import { reloadInto } from './deferredShortcodes.js';

const DEFERRED_HOSTS = 'lazy-shortcode, details-shortcode';
const REGION_ATTR = 'data-shortcode-region';
const BUTTON_SELECTOR = 'reload-shortcode > button';
// Mirrors defaultReloadLabel in shortcodes/reload_handler.go.
const DEFAULT_LABEL = 'Reload';

function announce(message, assertive = false) {
  if (typeof window.mahAnnounce === 'function') window.mahAnnounce(message, { assertive });
}

// Buttons inside a nested deferred block do not come back with this container's
// re-render (that block returns as an unrevealed placeholder), so counting them
// would shift the index used to find a replacement.
function reloadButtons(container) {
  return Array.from(container.querySelectorAll(BUTTON_SELECTOR)).filter((btn) => {
    const host = btn.closest(DEFERRED_HOSTS);
    return !host || host === container;
  });
}

// A reload replaces the content the button lives in, so the button that was
// activated is usually gone by the time the swap finishes. Put focus on its
// replacement (same position among the container's reload buttons) so keyboard
// and screen-reader users are not dropped back to the top of the document.
function restoreFocus(container, button, index) {
  if (button.isConnected) {
    button.focus();
    return;
  }
  const buttons = reloadButtons(container);
  const replacement = buttons[index] || buttons[0];
  if (replacement) {
    replacement.focus();
    return;
  }
  // No reload button survived the re-render (a conditional dropped it). Focus is
  // already gone with the old markup, so parking it on the refreshed content
  // beats leaving the reader on <body>.
  if (container.isConnected) parkFocus(container);
}

// Anything that can already take focus on its own must not be given a tabindex:
// stamping tabindex="-1" on a link or a control would pull it out of sequential
// Tab navigation for good. Focus parking succeeds on these, so an omission here
// is not a missed opportunity — it is a control silently dropped from the Tab
// order. Err toward listing too much: over-listing only costs a parking attempt.
const NATIVELY_FOCUSABLE = [
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

// A region wrapper is display:contents, so it generates no box and browsers do
// not reliably let it take focus. Rather than guess, try it and check.
function parkFocus(element) {
  if (focusOn(element)) return;
  // The wrapper could not hold focus, so hand it to the first child that can. A
  // hidden or boxless child is skipped rather than swallowing the focus.
  for (const child of element.children) {
    if (focusOn(child)) return;
  }
}

function focusOn(element) {
  const borrowed = !element.matches(NATIVELY_FOCUSABLE);
  if (borrowed) element.setAttribute('tabindex', '-1');
  element.focus({ preventScroll: true });
  if (document.activeElement === element) return true;
  // It could not take focus after all, so leave no trace on it.
  if (borrowed) element.removeAttribute('tabindex');
  return false;
}

// The server names the button whenever the face it rendered carries no text. It
// cannot see aria-hidden, though: an author's <span aria-hidden="true">↻</span>
// reads as text on the server and as nothing at all to a screen reader. Settle it
// here, against the real accessibility tree, so no reload button is ever nameless.
function ensureAccessibleName(button) {
  if (!button || button.hasAttribute('aria-label') || button.hasAttribute('aria-labelledby')) return;
  if (exposedText(button)) return;
  button.setAttribute('aria-label', DEFAULT_LABEL);
  if (!button.hasAttribute('title')) button.setAttribute('title', DEFAULT_LABEL);
}

// Every button waiting on a name is settled in one pass. Two reasons, and the
// second is why this is a queue rather than a direct call:
//   - exposedText reads computed style, which forces the browser to resolve
//     styles. A list page renders one reload button per card, and interleaving
//     those reads with the DOM work of upgrading elements would thrash layout;
//     batched, they cost one style resolution between them.
//   - The button is resolved at flush time, not when the check is requested. On
//     the morph path the node in place when the hook fires is the one about to be
//     replaced, so asking early would name a button that is on its way out.
const pendingNameChecks = new Set();
let nameCheckScheduled = false;

function scheduleNameCheck(host) {
  pendingNameChecks.add(host);
  if (nameCheckScheduled) return;
  nameCheckScheduled = true;
  queueMicrotask(() => {
    nameCheckScheduled = false;
    const hosts = Array.from(pendingNameChecks);
    pendingNameChecks.clear();
    for (const host of hosts) ensureAccessibleName(host.querySelector(':scope > button'));
  });
}

// Text an assistive technology would actually reach: everything under an
// aria-hidden subtree contributes nothing to the accessible name, and neither
// does anything CSS has taken off the page.
function exposedText(element) {
  let text = '';
  for (const node of element.childNodes) {
    if (node.nodeType === Node.TEXT_NODE) {
      text += node.textContent;
    } else if (
      node.nodeType === Node.ELEMENT_NODE &&
      node.getAttribute('aria-hidden') !== 'true' &&
      !node.hasAttribute('hidden') &&
      isDisplayed(node)
    ) {
      text += exposedText(node);
    }
  }
  return text.trim();
}

// display:none and visibility:hidden subtrees are not in the accessibility tree,
// so their text cannot name the button either. Only element children get here,
// and only for a button the server did not already label, so the common faces (a
// glyph, plain text) never reach a style lookup at all.
function isDisplayed(element) {
  const view = element.ownerDocument?.defaultView;
  if (!view || typeof view.getComputedStyle !== 'function') return true;
  const style = view.getComputedStyle(element);
  return style.display !== 'none' && style.visibility !== 'hidden';
}

class ReloadShortcode extends HTMLElement {
  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._busy = false;
    this._activationId = 0;
    this._repairName();

    // Delegated from the host element: a morph can replace the inner <button>
    // node, and a listener bound to the button itself would go with it.
    //
    // Only this host's own button counts. Nested [reload] is rejected at render
    // time, but a browser repairing invalid markup can still put a button
    // somewhere this element merely contains, and two hosts each acting on one
    // click would fire two reloads.
    this.addEventListener('click', (event) => {
      const button = event.target instanceof Element ? event.target.closest('button') : null;
      if (!button || button.closest('reload-shortcode') !== this) return;
      event.preventDefault();
      this._activate(button);
    });
  }

  // Called by the morph helper during Alpine's morph pass. The button is
  // server-rendered, so morph is free to replace its face, which can turn a named
  // button into one whose whole face is aria-hidden.
  //
  // A morph also installs a fresh region token, so any activation still in flight
  // is working on markup that is already gone: strand it, and let the refreshed
  // button be clicked again straight away rather than staying dead until an
  // obsolete request happens to settle.
  refreshFromMorph() {
    this._activationId = (this._activationId || 0) + 1;
    this._busy = false;
    this._repairName();
  }

  _repairName() {
    scheduleNameCheck(this);
  }

  _resolveTarget() {
    const deferred = this.closest(DEFERRED_HOSTS);
    if (deferred && typeof deferred.reload === 'function') {
      return { container: deferred, run: () => deferred.reload() };
    }
    const region = this.closest(`[${REGION_ATTR}]`);
    const token = region ? region.getAttribute(REGION_ATTR) : '';
    if (region && token) {
      // A region has no reveal path of its own to strand an in-flight reload, so
      // it detects a page morph landing underneath by watching its own token: the
      // seal is nonced, so a re-render always mints a different one.
      const isStale = () => region.getAttribute(REGION_ATTR) !== token;
      return { container: region, run: () => reloadInto(region, region, token, isStale) };
    }
    return null;
  }

  async _activate(button) {
    if (this._busy) return;

    const target = this._resolveTarget();
    if (!target) {
      window.location.reload();
      return;
    }

    this._busy = true;
    const activation = ++this._activationId;
    const hadFocus = document.activeElement === button;
    const index = reloadButtons(target.container).indexOf(button);
    this._clearFailure();
    button.setAttribute('aria-busy', 'true');
    button.setAttribute('data-reloading', '');
    // aria-busy tells assistive tech to withhold subtree updates; it does not
    // say that anything is happening. On a slow slot this is the only signal a
    // screen-reader user gets until the swap lands. A fast reload coalesces the
    // two announcements, because the live region debounces.
    announce('Reloading');

    let failed = false;
    try {
      // false means the result was abandoned (superseded, detached, or the
      // content was re-rendered underneath), so there is no success to report.
      const applied = await target.run();
      if (applied !== false && this._activationId === activation) announce('Content reloaded');
    } catch (e) {
      if (this._activationId === activation) {
        failed = true;
        announce('Could not reload the content', true);
      }
    } finally {
      // A morph superseded this activation: the markup it was announcing about,
      // the state it would clean up, and the focus it would restore all belong to
      // something else now.
      if (this._activationId === activation) {
        // `this` and `button` may both have been replaced by the swap; touching a
        // detached node is harmless, but only the live one is worth cleaning up.
        this._busy = false;
        if (button.isConnected) {
          button.removeAttribute('aria-busy');
          button.removeAttribute('data-reloading');
        }
        // A failed reload leaves the old content in place looking freshly loaded,
        // so sighted readers need a visible marker, not just the announcement.
        if (failed) this._showFailure();

        // Only take focus back if it is still ours to take: the reader may have
        // moved on during the round trip, and the swap itself drops focus to
        // <body>.
        const active = document.activeElement;
        if (hadFocus && (!active || active === button || active === document.body)) {
          restoreFocus(target.container, button, index);
        }
      }
    }
  }

  _showFailure() {
    this._clearFailure();
    // No role="alert" here: the assertive announcement above already covers
    // assistive tech, and this must not be spoken twice.
    const note = document.createElement('span');
    note.className = 'reload-shortcode-error';
    note.textContent = 'Reload failed.';
    this.appendChild(note);
  }

  _clearFailure() {
    const existing = this.querySelector('.reload-shortcode-error');
    if (existing) existing.remove();
  }
}

if (!customElements.get('reload-shortcode')) {
  customElements.define('reload-shortcode', ReloadShortcode);
}
