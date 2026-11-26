// Custom element backing the [action] category-template shortcode.
//
// The server renders <action-shortcode> with the action, the entity and the
// params already resolved and validated, around a real <button> and an empty
// status span. A click (after the action's confirmation, if any) posts to the
// same endpoint the actions menu uses, POST /v1/jobs/action/run, so the run is
// checked there exactly as a menu run is. Progress and the outcome go to the
// status span for sighted readers and to the page's live region (mahAnnounce)
// for screen readers: the span is not a live region of its own, so nothing is
// announced twice, and an announcement survives the refresh below.
//
// A successful run is reported to [autorefresh] as a save of this entity, made
// from where the button sits, so the blocks for this entity and the one around
// the button re-render. A failed or abandoned run reports nothing.

import { askToConfirm } from '../components/confirmDialog.js';
import { presentState, terminalStates } from '../components/jobStates.js';

const TAG = 'action-shortcode';
export const POLL_MS = 2000;
// 150 polls at 2 s: five minutes.
export const MAX_POLLS = 150;

const RUN_URL = '/v1/jobs/action/run';
const FOLLOW_IN_PANEL = 'Still running. Follow it in the Jobs panel.';

function announce(message, assertive = false) {
  if (typeof window.mahAnnounce === 'function') window.mahAnnounce(message, { assertive });
}

// The two refusal shapes the run endpoint answers with, read as the action
// modal reads them: {"errors": [{field, message}]} for validation, and
// {"error": "..."} for everything else.
export function refusalMessage(data) {
  const detailed = Array.isArray(data?.errors)
    ? data.errors.map((e) => e && (e.message || e.field)).filter(Boolean)
    : [];
  if (detailed.length) return detailed.join('; ');
  return data?.error || 'Action failed';
}

function parseParams(raw) {
  try {
    const parsed = JSON.parse(raw || '{}');
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

export class ActionShortcode extends HTMLElement {
  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._running = false;
    // Advanced when polling must stop, so an answer still in flight is dropped.
    this._pollSeq = 0;
    this._polling = false;
    this._timer = null;
    // Delegated from the host, as [reload] does: a page morph may replace the
    // inner button, and a listener bound to it would go with it.
    this.addEventListener('click', (event) => {
      const button = event.target instanceof Element ? event.target.closest('button') : null;
      if (!button || button.closest(TAG) !== this) return;
      event.preventDefault();
      void this._activate(button);
    });
  }

  disconnectedCallback() {
    this._stopPolling();
  }

  get _label() {
    return this.dataset.label || this.querySelector(':scope > button')?.textContent.trim() || 'Action';
  }

  async _activate(button) {
    // aria-disabled rather than disabled, so focus stays on the button while it
    // runs; this guard is what makes it inert.
    if (this._running) return;
    this._running = true;
    let keepRunning = false;
    try {
      const confirmText = this.dataset.confirm;
      if (confirmText) {
        const label = this._label;
        const confirmed = await askToConfirm(confirmText, {
          title: label,
          confirmLabel: label,
          destructive: false,
          fallbackFocus: () => this.querySelector(':scope > button'),
        });
        if (!confirmed) return;
      }
      keepRunning = await this._run(button);
    } finally {
      if (!keepRunning) this._finish();
    }
  }

  // Answers true when the run continues as a polled Job.
  async _run(button) {
    const label = this._label;
    button.setAttribute('aria-disabled', 'true');
    this._setStatus('Running…');
    announce(`${label}: running`);

    let resp;
    let data = {};
    try {
      resp = await fetch(RUN_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({
          plugin: this.dataset.plugin,
          action: this.dataset.action,
          entity_ids: [Number(this.dataset.entityId)],
          params: parseParams(this.dataset.params),
        }),
      });
      data = await resp.json().catch(() => ({}));
    } catch {
      this._fail(label, 'Action failed');
      return false;
    }

    if (!resp.ok) {
      this._fail(label, refusalMessage(data));
      return false;
    }
    if (data?.redirect && data.success !== false) {
      window.location.href = data.redirect;
      return false;
    }
    if (resp.status === 202 || data?.job_id || data?.canonicalJobId) {
      this._setStatus('Started');
      announce(`${label}: started`);
      const jobId = data?.canonicalJobId;
      if (!jobId) {
        this._setStatus('Started. Follow it in the Jobs panel.');
        return false;
      }
      this._poll(jobId, label);
      return true;
    }
    if (data?.success === true) {
      this._succeed(label, data.message || 'Done');
    } else {
      this._fail(label, data?.message || 'Action failed');
    }
    return false;
  }

  _poll(jobId, label) {
    this._polling = true;
    const seq = ++this._pollSeq;
    let polls = 0;
    const tick = async () => {
      this._timer = null;
      if (seq !== this._pollSeq) return;
      polls++;
      let job;
      try {
        const resp = await fetch(`/v1/jobs/${encodeURIComponent(jobId)}`, { headers: { Accept: 'application/json' } });
        if (seq !== this._pollSeq) return;
        if (!resp.ok) throw new Error(String(resp.status));
        job = await resp.json();
        if (seq !== this._pollSeq) return;
      } catch {
        if (seq !== this._pollSeq) return;
        this._fail(label, "Could not read the job's progress");
        this._finish();
        return;
      }
      const state = String(job?.state || '').toLowerCase();
      if (terminalStates().includes(state)) {
        const outcome = presentState(job).label;
        if (state === 'succeeded') this._succeed(label, outcome);
        else this._fail(label, outcome);
        this._finish();
        return;
      }
      if (polls >= MAX_POLLS) {
        this._setStatus(FOLLOW_IN_PANEL);
        announce(`${label}: ${FOLLOW_IN_PANEL}`);
        this._finish();
        return;
      }
      this._timer = setTimeout(tick, POLL_MS);
    };
    this._timer = setTimeout(tick, POLL_MS);
  }

  // Polling stops when the button leaves the page: nobody is looking at its
  // status any more, and a refresh that replaced it has drawn a new one. A
  // synchronous run in flight is left to finish, because the action has run
  // and the refresh and the announcement still belong to it.
  _stopPolling() {
    if (!this._polling) return;
    this._pollSeq++;
    if (this._timer !== null) clearTimeout(this._timer);
    this._timer = null;
    this._finish();
  }

  _succeed(label, message) {
    this._setStatus(message);
    announce(`${label}: ${message}`);
    // Captured now, not at the start of the run: the button is still where it
    // was until the refresh this triggers. "saving" then "updated" in one tick,
    // so a run that fails never leaves autorefresh holding a capture. No meta:
    // the inline editors ignore an update that carries none.
    document.dispatchEvent(new CustomEvent('meta-shortcode-saving', { detail: { source: this } }));
    document.dispatchEvent(new CustomEvent('meta-shortcode-updated', {
      detail: { entityType: this.dataset.entityType, entityId: Number(this.dataset.entityId), source: this },
    }));
  }

  _fail(label, message) {
    this._setStatus(message);
    announce(`${label}: ${message}`, true);
  }

  _finish() {
    this._running = false;
    this._polling = false;
    this.querySelector(':scope > button')?.removeAttribute('aria-disabled');
  }

  _setStatus(text) {
    const status = this.querySelector(':scope > .action-shortcode-status');
    if (status) status.textContent = text;
  }
}

if (typeof customElements !== 'undefined' && !customElements.get(TAG)) {
  customElements.define(TAG, ActionShortcode);
}
