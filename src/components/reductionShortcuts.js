/**
 * Keyboard review: j/k a s x ?
 *
 * Spread into the review store, so `this` is the store.
 *
 * The handler is bound to the Clusters region and nowhere else, so a shortcut
 * works only while focus is inside it (WCAG 2.1.4: a single-key shortcut is
 * active only when its component has focus). It also stands down for a modifier
 * key, a form field, an open dialog or an open media viewer. The shortcuts dialog
 * has an off switch, remembered in localStorage.
 */

const STORAGE_KEY = 'reduction-shortcuts';
const CARD = 'article[data-cluster-id]';
const EDITABLE = 'input, select, textarea, [contenteditable=""], [contenteditable="true"]';

export function readShortcutPreference() {
  try {
    return window.localStorage.getItem(STORAGE_KEY) !== 'off';
  } catch {
    return true;
  }
}

function cards(region) {
  return [...region.querySelectorAll(CARD)].filter((card) => card.getClientRects().length > 0);
}

function press(card, testid) {
  const button = card?.querySelector(`[data-testid="${testid}"]`);
  if (!button || button.disabled || !button.getClientRects().length) return false;
  button.click();
  return true;
}

export const shortcutMethods = {
  setShortcuts(on) {
    this.shortcutsOn = on;
    try {
      window.localStorage.setItem(STORAGE_KEY, on ? 'on' : 'off');
    } catch {
      // Without storage the choice lasts until the page is left.
    }
    window.mahAnnounce?.(on ? 'Keyboard shortcuts on.' : 'Keyboard shortcuts off.');
  },

  onKey(event) {
    if (!this.shortcutsOn || this.dialog || this.applying) return;
    if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.target?.closest?.(EDITABLE)) return;
    if (globalThis.Alpine?.store('lightbox')?.isOpen) return;

    const region = event.currentTarget;
    const key = event.key;
    if (key === '?') {
      event.preventDefault();
      // Focus goes back to the control the reviewer was on, not to the region this handler is bound to.
      this.openDialog('shortcuts', null, event.target);
      return;
    }
    if (!['j', 'k', 'a', 's', 'x'].includes(key)) return;

    const list = cards(region);
    const current = event.target.closest?.(CARD);
    const index = current ? list.indexOf(current) : -1;

    if (key === 'j' || key === 'k') {
      event.preventDefault();
      const next = key === 'j' ? list[index + 1] : list[index - 1];
      const target = next || (index === -1 ? list[0] : null);
      if (!target) return;
      target.focus({ preventScroll: true });
      const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
      target.scrollIntoView({ block: 'nearest', behavior: reduce ? 'auto' : 'smooth' });
      return;
    }

    if (!current || this.busy) return;
    event.preventDefault();
    if (key === 'a') press(current, 'cluster-approve');
    else if (key === 's') press(current, 'cluster-skip') || press(current, 'cluster-reopen');
    else if (key === 'x') press(current, 'cluster-expand');
  },
};
