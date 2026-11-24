// Custom element backing the [calendar] category-template shortcode.
//
// The server renders the month as a table inside a <calendar-shortcode> host
// carrying two sealed tokens: the same shortcode with month set to the previous
// and to the next month. Prev/Next re-render through /v1/shortcodes/deferred,
// the endpoint [lazy] and [reload] use, and the response is itself a whole
// <calendar-shortcode>, which replaces this one.
//
// The swap goes through reloadInto, so the response first lands inside this
// host for a moment. That inner copy must do nothing when it connects: a
// calendar has no body, so one calendar is never legitimately inside another.

import { reloadInto } from './deferredShortcodes.js';
import { parkFocus } from '../utils/focus.js';

const TAG = 'calendar-shortcode';

function announce(message, assertive = false) {
  if (typeof window.mahAnnounce === 'function') window.mahAnnounce(message, { assertive });
}

export class CalendarShortcode extends HTMLElement {
  connectedCallback() {
    if (this._init) return;
    if (this.parentElement?.closest(TAG)) return;
    this._init = true;
    this._busy = false;
    this.addEventListener('click', (event) => this._onClick(event));
  }

  _onClick(event) {
    const button = event.target?.closest?.('button[data-calendar-nav]');
    if (!button || button.closest(TAG) !== this || this._busy) return;
    const direction = button.getAttribute('data-calendar-nav');
    const token = direction === 'prev' ? this.dataset.prevToken : this.dataset.nextToken;
    if (!token) return;
    this._busy = true;
    return this._navigate(token, direction, button);
  }

  async _navigate(token, direction, button) {
    let saved = null;
    let applied;
    try {
      applied = await reloadInto(this, this, token, undefined, () => {
        saved = Array.from(this.childNodes);
      });
    } catch (e) {
      // The old month is still on screen.
      this._busy = false;
      if (button.isConnected) button.focus();
      announce('Could not load the month', true);
      return;
    }
    if (applied === false) {
      // Something else took the content over (a page morph, an enclosing
      // [reload]); it owns what the reader sees now.
      this._busy = false;
      return;
    }

    const fresh = this.querySelector(`:scope > ${TAG}`);
    if (!fresh) {
      // An error marker or an "unavailable" comment: keep the month the reader
      // was looking at rather than replace it with that.
      if (saved) this.replaceChildren(...saved);
      this._busy = false;
      button.focus();
      announce('Could not load the month', true);
      return;
    }

    this.replaceWith(fresh);
    const same = fresh.querySelector(`button[data-calendar-nav="${direction}"]`);
    if (same) same.focus();
    else parkFocus(fresh);
    const caption = fresh.querySelector('caption');
    if (caption) announce(caption.textContent.replace(/\s+/g, ' ').trim());
  }
}

if (typeof customElements !== 'undefined' && !customElements.get(TAG)) {
  customElements.define(TAG, CalendarShortcode);
}
