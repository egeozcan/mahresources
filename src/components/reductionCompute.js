/**
 * Finding duplicates, live: the compute request, and following the job it starts.
 *
 * Spread into the review store, so `this` is the store.
 *
 * The request is a fetch rather than a form post so a refusal can be shown where
 * the reviewer is. On success the page is reloaded, not patched: the server
 * already renders the computing state (job id and all), and one rendering path is
 * cheaper to keep right than two.
 *
 * While computing, the page follows the job on /v1/jobs/events, the stream the
 * jobs panel and the admin export use, and shows its phase progress. Two things
 * make that less simple than subscribing:
 *
 *  - The row holds a `pending:` nonce, not a job id, from the request until the
 *    worker starts and swaps in the queue job's own id. A page loaded in that
 *    window polls the page's JSON twin until the id appears.
 *  - A stream that errors (a proxy that buffers, a dropped connection) falls back
 *    to polling the same route every few seconds.
 *
 * A terminal event is not trusted to mean the row has moved: the page waits for
 * the row itself to leave `computing` before it reloads.
 */

const POLL_MS = 3000;
const SETTLE_MS = 500;
const SETTLE_TRIES = 40;

const TERMINAL = new Set(['completed', 'failed', 'cancelled']);

export function progressText(job) {
  const total = Number(job?.phaseTotal) || 0;
  const count = Number(job?.phaseCount) || 0;
  if (total > 0) return `Comparing… ${count.toLocaleString('en-US')} of ${total.toLocaleString('en-US')}`;
  const phase = (job?.phase || '').trim();
  if (!phase || phase === 'clustering') return 'Starting…';
  return `${phase.charAt(0).toUpperCase()}${phase.slice(1)}…`;
}

export const computeMethods = {
  async compute() {
    if (this.busy) return;
    this.busy = true;
    this.error = '';
    this.startError = '';
    try {
      const response = await fetch('/v1/reduction/compute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ id: this.reductionId, version: this.version }),
      });
      if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
      // Stays busy: the page is about to be replaced by its computing state.
      window.location.reload();
    } catch (err) {
      this.busy = false;
      this.error = err.message;
      window.mahAnnounce?.(`Could not start finding duplicates: ${err.message}`, { assertive: true });
    }
  },

  watchCompute() {
    this.stopWatching();
    this.progress = { text: 'Starting…', count: 0, total: 0 };
    if (!this.jobId || this.jobId.startsWith('pending:')) {
      this._poll();
    } else {
      this._stream(this.jobId);
    }
  },

  stopWatching() {
    if (this._events) {
      this._events.close();
      this._events = null;
    }
    if (this._pollTimer) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
  },

  _stream(jobId) {
    let events;
    try {
      events = new EventSource('/v1/jobs/events');
    } catch {
      this._poll();
      return;
    }
    this._events = events;
    const onJob = (job) => {
      if (!job || job.id !== jobId) return;
      this._showProgress(job);
      if (TERMINAL.has(job.status)) this._settle();
    };
    const handler = (event) => {
      try { onJob(JSON.parse(event.data).job); } catch { /* ignore a frame that is not ours */ }
    };
    events.addEventListener('init', (event) => {
      try { onJob((JSON.parse(event.data).jobs || []).find((j) => j.id === jobId)); } catch { /* ignore */ }
    });
    events.addEventListener('added', handler);
    events.addEventListener('updated', handler);
    events.addEventListener('removed', handler);
    events.onerror = () => {
      // Closed rather than left to reconnect: polling is the fallback, and a
      // stream that keeps failing would otherwise run beside it.
      if (this._events === events) {
        events.close();
        this._events = null;
        this._poll();
      }
    };
  },

  _showProgress(job) {
    const text = progressText(job);
    this.progress = { text, count: Number(job.phaseCount) || 0, total: Number(job.phaseTotal) || 0 };
    // The phase changes a handful of times; the counts change constantly and are
    // not announced.
    const phase = job.phase || '';
    if (phase && phase !== this._announcedPhase) {
      this._announcedPhase = phase;
      window.mahAnnounce?.(text);
    }
  },

  _poll() {
    if (this._pollTimer) return;
    const tick = async () => {
      const row = await this._fetchRow();
      if (!row) return;
      if (row.status !== 'computing') {
        this.stopWatching();
        window.location.reload();
        return;
      }
      if (row.jobId && !row.jobId.startsWith('pending:') && row.jobId !== this.jobId) {
        // The worker has claimed the row: follow the job itself from here.
        this.jobId = row.jobId;
        this.stopWatching();
        this._stream(row.jobId);
      }
    };
    this._pollTimer = setInterval(tick, POLL_MS);
    setTimeout(tick, 700);
  },

  async _fetchRow() {
    try {
      const response = await fetch(`/reduction.json?id=${this.reductionId}`, { headers: { Accept: 'application/json' } });
      if (!response.ok) return null;
      const page = await response.json();
      return { status: page.reduction?.StatusEffective, jobId: page.raw?.computeJobId || '' };
    } catch {
      return null;
    }
  },

  async _settle() {
    this.stopWatching();
    for (let i = 0; i < SETTLE_TRIES; i++) {
      const row = await this._fetchRow();
      if (row && row.status !== 'computing') break;
      await new Promise((resolve) => setTimeout(resolve, SETTLE_MS));
    }
    window.location.reload();
  },
};
