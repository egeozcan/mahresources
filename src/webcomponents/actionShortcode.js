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
const POLL_WINDOW_MS = 5 * 60 * 1000;

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
  static observedAttributes = ['data-plugin', 'data-action', 'data-entity-type', 'data-entity-id', 'data-params', 'data-label', 'data-confirm', 'data-async'];

  _payloadVersion = 0;
  _attachmentVersion = 0;
  _running = false;
  _jobPoll = null;

  connectedCallback() {
    if (this._init) return;
    this._init = true;
    // Delegated from the host, as [reload] does: a page morph may replace the
    // inner button, and a listener bound to it would go with it.
    this.addEventListener('click', (event) => {
      const button = event.target instanceof Element ? event.target.closest('button') : null;
      if (!button || button.closest(TAG) !== this) return;
      event.preventDefault();
      void this._activate();
    });
  }

  attributeChangedCallback(_name, before, after) {
    if (before === after) return;
    this._payloadVersion++;
    this._statusElement()?.replaceChildren();
  }

  disconnectedCallback() {
    this._attachmentVersion++;
    if (this._running) this._statusElement()?.replaceChildren();
    // A submitted synchronous action can still report its result; only the
    // progress watcher is tied to this attachment of the button.
    if (this._jobPoll) this._finish();
  }

  get _label() {
    return this.dataset.label || this.querySelector(':scope > button')?.textContent.trim() || 'Action';
  }

  _isAttached(run) {
    return this.isConnected && this._attachmentVersion === run.attachmentVersion;
  }

  _isCurrent(run) {
    return this._isAttached(run) && this._payloadVersion === run.payloadVersion && this._label === run.label;
  }

  async _activate() {
    // aria-disabled rather than disabled, so focus stays on the button while it
    // runs; this guard is what makes it inert.
    if (this._running) return;
    this._running = true;
    // List morphs reuse hosts by position. Freeze the request before asking,
    // and treat a changed or disconnected host as a cancelled confirmation.
    const { plugin, action, entityType, params, confirm } = this.dataset;
    const entityId = Number(this.dataset.entityId);
    const run = {
      payloadVersion: this._payloadVersion,
      attachmentVersion: this._attachmentVersion,
      entityType,
      entityId,
      label: this._label,
      body: JSON.stringify({ plugin, action, entity_ids: [entityId], params: parseParams(params) }),
    };
    let keepRunning = false;
    try {
      if (confirm) {
        const confirmed = await askToConfirm(confirm, {
          title: run.label,
          confirmLabel: run.label,
          destructive: false,
          fallbackFocus: () => this._isCurrent(run) ? this.querySelector(':scope > button') : null,
        });
        if (!confirmed) return;
      }
      if (!this._isCurrent(run)) return;
      keepRunning = await this._run(run);
    } finally {
      if (!keepRunning) this._finish();
    }
  }

  // Answers true when the run continues as a polled Job.
  async _run(run) {
    this.querySelector(':scope > button')?.setAttribute('aria-disabled', 'true');
    this._setStatus(run, 'Running…');
    announce(`${run.label}: running`);

    let resp;
    let data = {};
    try {
      resp = await fetch(RUN_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: run.body,
      });
      data = await resp.json().catch(() => ({}));
    } catch {
      this._fail(run, 'Action failed');
      return false;
    }

    if (!resp.ok) {
      this._fail(run, refusalMessage(data));
      return false;
    }
    if (data?.success === false) {
      this._fail(run, data.message || 'Action failed');
      return false;
    }
    if (data?.redirect) {
      window.location.href = data.redirect;
      return false;
    }
    if (resp.status === 202 || data?.job_id || data?.canonicalJobId) {
      if (!this._isAttached(run)) return false;
      this._setStatus(run, 'Started');
      announce(`${run.label}: started`);
      const jobId = data?.canonicalJobId;
      if (!jobId) {
        this._setStatus(run, 'Started. Follow it in the Jobs panel.');
        return false;
      }
      this._poll(jobId, run);
      return true;
    }
    if (data?.success === true) {
      this._succeed(run, data.message || 'Done');
    } else {
      this._fail(run, data?.message || 'Action failed');
    }
    return false;
  }

  _poll(jobId, run) {
    const controller = new AbortController();
    const { signal } = controller;
    const tick = async () => {
      if (signal.aborted) return;
      let job;
      try {
        const resp = await fetch(`/v1/jobs/${encodeURIComponent(jobId)}`, { headers: { Accept: 'application/json' }, signal });
        if (signal.aborted) return;
        if (!resp.ok) throw new Error(String(resp.status));
        job = await resp.json();
        if (signal.aborted) return;
      } catch {
        if (signal.aborted) return;
        this._fail(run, "Could not read the job's progress");
        this._finish();
        return;
      }
      const state = String(job?.state || '').toLowerCase();
      if (terminalStates().includes(state)) {
        const outcome = presentState(job).label;
        if (state === 'succeeded') this._succeed(run, outcome);
        else this._fail(run, outcome);
        this._finish();
        return;
      }
      this._jobPoll.timer = setTimeout(tick, POLL_MS);
    };
    this._jobPoll = {
      controller,
      timer: setTimeout(tick, POLL_MS),
      deadline: setTimeout(() => {
        this._setStatus(run, FOLLOW_IN_PANEL);
        announce(`${run.label}: ${FOLLOW_IN_PANEL}`);
        this._finish();
      }, POLL_WINDOW_MS),
    };
  }

  _succeed(run, message) {
    this._setStatus(run, message);
    announce(`${run.label}: ${message}`);
    // A morphed or detached host must not capture a different enclosing block.
    // The submitted entity's blocks still refresh. No meta: inline editors
    // ignore an update that carries none.
    const source = this._isCurrent(run) ? this : null;
    document.dispatchEvent(new CustomEvent('meta-shortcode-saving', { detail: { source } }));
    document.dispatchEvent(new CustomEvent('meta-shortcode-updated', {
      detail: { entityType: run.entityType, entityId: run.entityId, source },
    }));
  }

  _fail(run, message) {
    this._setStatus(run, message);
    announce(`${run.label}: ${message}`, true);
  }

  _finish() {
    if (this._jobPoll) {
      const { controller, timer, deadline } = this._jobPoll;
      this._jobPoll = null;
      clearTimeout(timer);
      clearTimeout(deadline);
      controller.abort();
    }
    this._running = false;
    this.querySelector(':scope > button')?.removeAttribute('aria-disabled');
  }

  _statusElement() {
    return this.querySelector(':scope > .action-shortcode-status');
  }

  _setStatus(run, text) {
    if (!this._isCurrent(run)) return;
    const status = this._statusElement();
    if (status) status.textContent = text;
  }
}

if (typeof customElements !== 'undefined' && !customElements.get(TAG)) {
  customElements.define(TAG, ActionShortcode);
}
