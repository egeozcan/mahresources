// Custom element backing the [autorefresh] category-template shortcode.
//
// The server renders the block's body in place and wraps it in an
// <autorefresh-shortcode> carrying a sealed token of the raw body and the entity
// it was rendered for. Inline editors announce each save with a document-level
// `meta-shortcode-updated` event; a block re-renders through the same endpoint
// [lazy] and [reload] use when either
//   - the save was for the block's own entity, wherever the editor sits, or
//   - the editor that saved sits inside the block, whatever entity it edits.
//
// Saves close together are coalesced: each one restarts a short timer, and only
// when it runs out does every block that matched any of them refresh, once.

import { reloadInto } from './deferredShortcodes.js';
import { FOCUSABLE_IN_CONTAINER, parkFocus } from '../utils/focus.js';

const TAG = 'autorefresh-shortcode';
export const COALESCE_MS = 250;

// hostsToRefresh picks, from every block on the page, the ones a save described
// by detail should refresh. A block whose ancestor block is also refreshing is
// dropped: the outer re-render rebuilds it anyway, and refreshing both would
// send a request whose answer is thrown away.
//
// captured holds the blocks the editor sat in when its save began. A refresh
// landing while the save is in flight replaces the editor, so by the time the
// save reports, its source is detached and contains-matching finds nothing; the
// block elements themselves survive a refresh, so the captured ones still do.
export function hostsToRefresh(hosts, detail, captured = []) {
  if (!detail) return [];
  const source = detail.source || null;
  const fromCapture = captured.filter((host) => host && host.isConnected);
  const matched = hosts.filter((host) => {
    if (fromCapture.includes(host)) return true;
    if (!host || !host.isConnected) return false;
    if (source && typeof host.contains === 'function' && host.contains(source)) return true;
    return (
      host.getAttribute('data-entity-type') === detail.entityType &&
      Number(host.getAttribute('data-entity-id')) === Number(detail.entityId)
    );
  });
  for (const host of fromCapture) if (!matched.includes(host)) matched.push(host);
  return outermost(matched);
}

export function outermost(hosts) {
  return hosts.filter((host) => !hosts.some((other) => other !== host && other.contains(host)));
}

// createRefreshScheduler collects matched blocks and refreshes them once the
// saves stop arriving. run receives the blocks to refresh, already reduced to the
// outermost ones still on the page.
export function createRefreshScheduler(run, delay = COALESCE_MS) {
  const pending = new Set();
  let timer = null;

  function flush() {
    timer = null;
    const hosts = outermost(Array.from(pending).filter((host) => host.isConnected));
    pending.clear();
    if (hosts.length) run(hosts);
  }

  return {
    add(hosts) {
      if (!hosts.length) return;
      for (const host of hosts) pending.add(host);
      if (timer !== null) clearTimeout(timer);
      timer = setTimeout(flush, delay);
    },
  };
}

const scheduler = createRefreshScheduler((hosts) => {
  for (const host of hosts) void host.refresh();
});

// Editor element -> the blocks it sat in when its current save began.
const savingIn = new WeakMap();

function enclosingHosts(element) {
  const hosts = [];
  for (let host = element?.closest?.(TAG); host; host = host.parentElement?.closest(TAG)) hosts.push(host);
  return hosts;
}

function onMetaSaving(event) {
  const source = event.detail?.source;
  if (source) savingIn.set(source, enclosingHosts(source));
}

function onMetaUpdated(event) {
  const source = event.detail?.source;
  const captured = (source && savingIn.get(source)) || [];
  if (source) savingIn.delete(source);
  const hosts = Array.from(document.querySelectorAll(TAG));
  scheduler.add(hostsToRefresh(hosts, event.detail, captured));
}

class AutorefreshShortcode extends HTMLElement {
  // Called by a [reload] button inside the block, and by refresh() below.
  reload(beforeSwap) {
    const token = this.getAttribute('data-token');
    if (!token) return Promise.resolve(false);
    // A page morph installs a freshly sealed token, so a changed token means
    // this content was re-rendered underneath the request.
    const isStale = () => this.getAttribute('data-token') !== token;
    return reloadInto(this, this, token, isStale, beforeSwap);
  }

  // A refresh replaces everything in the block, including whatever control holds
  // focus inside it: the editor that just saved, or one the reader moved to while
  // the request was in flight. Focus is read at the moment of the swap, not when
  // the refresh starts, and put back on the control in the same position among
  // the block's focusable elements so a keyboard user stays where they were.
  async refresh() {
    let index = -1;
    const beforeSwap = () => {
      const active = document.activeElement;
      if (!active || active === this || !this.contains(active)) return;
      index = Array.from(this.querySelectorAll(FOCUSABLE_IN_CONTAINER)).indexOf(active);
      if (index < 0) index = 0;
    };

    let applied = false;
    try {
      applied = (await this.reload(beforeSwap)) !== false;
    } catch (e) {
      // The old content stays on screen. A refresh nobody asked for directly
      // has no button to show a failure next to, and the edit itself saved.
      return;
    }
    if (!applied || index < 0 || !this.isConnected) return;

    // Inline editors are Lit elements: they render their controls in a
    // microtask after being connected, so wait one task before looking.
    await new Promise((resolve) => setTimeout(resolve, 0));
    const now = document.activeElement;
    if (now && now !== document.body && now.isConnected) return;
    const replacement = Array.from(this.querySelectorAll(FOCUSABLE_IN_CONTAINER))[index];
    if (replacement) replacement.focus();
    else parkFocus(this);
  }
}

if (typeof customElements !== 'undefined' && !customElements.get(TAG)) {
  customElements.define(TAG, AutorefreshShortcode);
  document.addEventListener('meta-shortcode-saving', onMetaSaving);
  document.addEventListener('meta-shortcode-updated', onMetaUpdated);
}
