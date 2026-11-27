// Custom element backing the [tabs] category-template shortcode.
//
// The server renders a layout that works without JavaScript: a hidden tab list,
// then every panel as a named role="group" with its label shown above it. This
// element upgrades that to the WAI-ARIA tabs pattern: it shows the tab list,
// gives the tabs and panels IDs and their tab/tabpanel wiring, and hides every
// panel but the selected one. The server emits no IDs because one template
// renders many times on a page (cards, hover cards); IDs come from a counter.
//
// Activation is manual (APG): arrow keys, Home and End move focus between tabs,
// and Enter, Space or a click selects. A lazy panel fetches when it is shown, so
// automatic activation would fire one request for every tab an arrow-key reader
// passes over.
//
// Everything acts on this host's own tab list and panels only (direct children),
// so a [tabs] nested in a panel is left to its own host.

import { tabbable } from 'tabbable';

const TAG = 'tabs-shortcode';

let nextUid = 0;

// Hosts a morph has patched, settled together once the morph call returns.
const pendingMorphs = new Set();
let morphFlushQueued = false;

function flushMorphs() {
  morphFlushQueued = false;
  const hosts = Array.from(pendingMorphs).filter((host) => host.isConnected);
  pendingMorphs.clear();
  for (const host of hosts) host._apply();
  // The suspended flags read closest('.tabs-panel[hidden]'), which an outer host
  // flushed later in the list may still have changed: sync them once every host
  // has applied, so nested sets agree whatever order they were queued in.
  for (const host of hosts) host._syncSuspended();
  for (const host of hosts) host._restoreFocusAfterMorph();
}

export class TabsShortcode extends HTMLElement {
  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._uid = ++nextUid;
    this._selected = 0;
    this._focusBeforeMorph = null;
    this.addEventListener('keydown', (event) => this._onKeydown(event));
    this.addEventListener('click', (event) => this._onClick(event));
    this.addEventListener('focusin', (event) => this._onFocusin(event));
    this.addEventListener('focusout', (event) => this._onFocusout(event));
    // A deferred block clears aria-busy when its content lands (a first load, a
    // retry or a [reload]). That content may bring the panel's first link, after
    // which the panel's own tab stop is one too many.
    if (typeof MutationObserver === 'function') {
      new MutationObserver(() => this._syncPanelTabStop()).observe(this, {
        attributes: true,
        attributeFilter: ['aria-busy'],
        subtree: true,
      });
    }
    this._apply();
  }

  // Called by the morph helper after it patched this host's attributes and
  // before it patches the children, which still carry the server's markup. That
  // pass would overwrite anything done here, so the work waits for a microtask:
  // it runs after the whole morph returns and before the next rendering step.
  refreshFromMorph() {
    if (!this._init) return;
    const active = document.activeElement;
    if (this._tabs().includes(active)) this._focusBeforeMorph = active;
    pendingMorphs.add(this);
    if (morphFlushQueued) return;
    morphFlushQueued = true;
    queueMicrotask(flushMorphs);
  }

  _restoreSelection(state) {
    const morphing = pendingMorphs.has(this);
    if (!this._init || (!state && !morphing)) return;
    if (state?.uid !== this._uid) {
      // A positional patch can have reused a different set's focused button.
      const previousFocus = this._focusBeforeMorph;
      this._focusBeforeMorph = null;
      if (previousFocus === document.activeElement) previousFocus?.blur();
    }
    this._uid = state?.uid ?? ++nextUid;
    this._selected = state?.selected ?? 0;
    if (!morphing) this._apply();
  }

  _list() {
    return this.querySelector(':scope > .tabs-list');
  }

  _tabs() {
    const list = this._list();
    return list ? Array.from(list.querySelectorAll(':scope > [role="tab"]')) : [];
  }

  _panels() {
    return Array.from(this.querySelectorAll(':scope > .tabs-panel'));
  }

  // The tab button an event belongs to, when it is one of this host's own.
  _ownTab(target) {
    const tab = target instanceof Element ? target.closest('[role="tab"]') : null;
    return tab && tab.parentElement === this._list() ? tab : null;
  }

  // Idempotent: every call derives the whole state from _uid and _selected.
  // Returns false, leaving the stacked layout, when the markup is not a tab set.
  _apply() {
    const list = this._list();
    const tabs = this._tabs();
    const panels = this._panels();
    if (!list || tabs.length === 0 || tabs.length !== panels.length) return false;

    this._selected = Math.min(Math.max(this._selected, 0), tabs.length - 1);
    const base = `tabs-sc-${this._uid}`;
    tabs.forEach((tab, i) => {
      tab.id = `${base}-tab-${i}`;
      tab.setAttribute('aria-controls', `${base}-panel-${i}`);
      tab.setAttribute('aria-selected', String(i === this._selected));
    });
    panels.forEach((panel, i) => {
      panel.id = `${base}-panel-${i}`;
      panel.setAttribute('role', 'tabpanel');
      panel.setAttribute('aria-labelledby', `${base}-tab-${i}`);
      panel.removeAttribute('aria-label');
      const fallback = panel.querySelector(':scope > .tabs-fallback-label');
      if (fallback) fallback.hidden = true;
      panel.hidden = i !== this._selected;
      if (panel.hidden) panel.removeAttribute('tabindex');
    });
    list.hidden = false;

    this._syncRovingTabindex();
    this._syncPanelTabStop();
    // A nested list becomes tabbable only after its own upgrade.
    this.parentElement?.closest(TAG)?._syncPanelTabStop();
    this._syncSuspended();
    return true;
  }

  // One tab stop in the list: the focused tab while focus is in it, otherwise
  // the selected one. stop forces the latter, for a focusout that is leaving.
  _syncRovingTabindex(stop) {
    const tabs = this._tabs();
    if (stop === undefined) {
      const focused = tabs.indexOf(document.activeElement);
      stop = focused >= 0 ? focused : this._selected;
    }
    tabs.forEach((tab, i) => tab.setAttribute('tabindex', i === stop ? '0' : '-1'));
  }

  _syncPanelTabStop() {
    const panel = this._panels()[this._selected];
    if (!panel || panel.hidden || panel.getAttribute('role') !== 'tabpanel') return;
    if (tabbable(panel, { getShadowRoot: true }).length === 0) {
      panel.setAttribute('tabindex', '0');
    } else if (document.activeElement !== panel) {
      // Removing it from the focused panel would drop the reader on <body>; it
      // goes the next time the panel is shown instead.
      panel.removeAttribute('tabindex');
    }
  }

  // A [lazy] block in a hidden panel must not fetch when a morph refreshes it:
  // it re-arms and loads when it is next shown (LazyShortcode.refreshFromMorph).
  _syncSuspended() {
    for (const lazy of this.querySelectorAll('lazy-shortcode')) {
      lazy.suspended = !!lazy.closest('.tabs-panel[hidden]');
    }
  }

  // A morph re-hides the tab list until the microtask restores it. A browser
  // that recalculates style in that gap may move focus off the hidden tab, so
  // the tab that had it gets it back, unless focus has gone somewhere real.
  _restoreFocusAfterMorph() {
    const tab = this._focusBeforeMorph;
    this._focusBeforeMorph = null;
    if (!tab || !tab.isConnected || !this._tabs().includes(tab)) return;
    const active = document.activeElement;
    if (active === tab || (active && active !== document.body)) return;
    tab.focus({ preventScroll: true });
    this._syncRovingTabindex();
  }

  _onKeydown(event) {
    const tab = this._ownTab(event.target);
    if (!tab || event.altKey || event.ctrlKey || event.metaKey) return;
    const tabs = this._tabs();
    const i = tabs.indexOf(tab);
    const n = tabs.length;
    let next;
    switch (event.key) {
      case 'ArrowRight':
        next = (i + 1) % n;
        break;
      case 'ArrowLeft':
        next = (i - 1 + n) % n;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = n - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    tabs[next].focus();
  }

  // Enter and Space reach here too: they activate the native <button>.
  _onClick(event) {
    const tab = this._ownTab(event.target);
    if (!tab) return;
    const i = this._tabs().indexOf(tab);
    if (i < 0) return;
    this._selected = i;
    this._apply();
  }

  _onFocusin(event) {
    if (!this._ownTab(event.target)) return;
    this._syncRovingTabindex();
    this._syncPanelTabStop();
  }

  _onFocusout(event) {
    if (!this._ownTab(event.target)) return;
    const list = this._list();
    if (event.relatedTarget instanceof Node && list && list.contains(event.relatedTarget)) return;
    this._syncRovingTabindex(this._selected);
  }
}

if (typeof customElements !== 'undefined' && !customElements.get(TAG)) {
  customElements.define(TAG, TabsShortcode);
}
