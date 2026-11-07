import { cancelReview } from './generationDisclosure.js';

/**
 * The client side of a harness Generation run (docs/plans/2026-11-07-template-generation-harness.md
 * sections 5 and 6): an SSE reader over fetch, the progress state the panel
 * (templates/partials/form/templateRunProgress.tpl) binds to, the transcript
 * view model, and the teardown helpers both generation hosts share.
 *
 * fetch rather than EventSource because the request is a POST that carries the
 * CSRF header. Everything the server sends is shown as text; nothing here
 * produces markup.
 */

// Frames are separated by a blank line; CRLF and LF framing both occur.
const FRAME_END = /\r\n\r\n|\n\n|\r\r/;

function parseFrame(raw) {
  let event = 'message';
  const data = [];
  for (const line of raw.split(/\r\n|\n|\r/)) {
    if (line === '' || line.startsWith(':')) continue;
    const colon = line.indexOf(':');
    const field = colon < 0 ? line : line.slice(0, colon);
    let value = colon < 0 ? '' : line.slice(colon + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    if (field === 'event') event = value;
    else if (field === 'data') data.push(value);
  }
  if (!data.length) return null;
  try {
    return { event, data: JSON.parse(data.join('\n')) };
  } catch {
    // A frame the reader cannot parse is dropped; the terminal frame decides the run.
    return null;
  }
}

/**
 * Read a run's event stream. Progress frames go to `onEvent(name, data)` as they
 * arrive, before the stream ends. Resolves the first terminal frame as
 * { kind: 'result' | 'error', data } and ignores everything after it, or
 * { kind: 'closed' } when the stream ends without one. Rejects with the abort
 * error when `signal` aborts.
 */
export async function consumeRun(response, onEvent, signal) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const abort = () => reader.cancel().catch(() => {});
  if (signal?.aborted) {
    abort();
    throw new DOMException('Aborted', 'AbortError');
  }
  signal?.addEventListener('abort', abort, { once: true });
  let buffer = '';
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
      // stream: true keeps a multibyte character split across chunks intact.
      buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
      let match;
      while ((match = FRAME_END.exec(buffer))) {
        const frame = parseFrame(buffer.slice(0, match.index));
        buffer = buffer.slice(match.index + match[0].length);
        if (!frame) continue;
        if (frame.event === 'result' || frame.event === 'error') {
          abort();
          return { kind: frame.event, data: frame.data };
        }
        onEvent(frame.event, frame.data);
      }
      if (done) return { kind: 'closed' };
    }
  } finally {
    signal?.removeEventListener('abort', abort);
  }
}

const isCount = (n) => typeof n === 'number' && Number.isFinite(n);

/** Panel state for one run. `status`: running | done | error | cancelled | closed. */
export function newRun() {
  return {
    runId: '',
    status: 'running',
    maxTurns: 0,
    deadline: '',
    turn: 0,
    turns: [],
    budgetUsed: 0,
    budgetMax: 0,
    budgetPercent: 0,
    finishRequested: false,
    finishError: '',
    // The polite live region says "Turn N started" and the end state, nothing else.
    announcement: '',
    stopReason: '',
    transcript: [],
    // The transcript as plain strings, built once when the run ends.
    records: [],
    error: '',
  };
}

export function applyRunEvent(run, name, data) {
  if (!data || typeof data !== 'object') return;
  const turnEntry = (n) => run.turns.find((t) => t.turn === n);
  switch (name) {
    case 'run':
      if (typeof data.runId === 'string') run.runId = data.runId;
      if (isCount(data.maxTurns)) run.maxTurns = data.maxTurns;
      if (isCount(data.tokenBudget)) run.budgetMax = data.tokenBudget;
      if (typeof data.deadline === 'string') run.deadline = data.deadline;
      break;
    case 'turn':
      if (!isCount(data.turn)) break;
      if (isCount(data.maxTurns)) run.maxTurns = data.maxTurns;
      if (!turnEntry(data.turn)) {
        run.turns.push({ turn: data.turn, finalReason: data.finalReason || '', tools: [], submissions: [] });
      } else if (data.finalReason) {
        turnEntry(data.turn).finalReason = data.finalReason;
      }
      run.turn = data.turn;
      run.announcement = `Turn ${data.turn} started`;
      break;
    case 'tool': {
      const entry = turnEntry(data.turn);
      if (!entry) break;
      entry.tools.push({
        name: String(data.name || 'unknown'),
        args: data.args && typeof data.args === 'object' ? data.args : {},
        bytes: isCount(data.bytes) ? data.bytes : 0,
        isError: !!data.isError,
        durationMs: isCount(data.durationMs) ? data.durationMs : 0,
      });
      break;
    }
    case 'submission': {
      const entry = turnEntry(data.turn);
      if (!entry) break;
      entry.submissions.push({
        field: String(data.field || ''),
        errors: isCount(data.errors) ? data.errors : 0,
        warnings: isCount(data.warnings) ? data.warnings : 0,
      });
      break;
    }
    case 'usage':
      if (isCount(data.budgetUsed)) run.budgetUsed = data.budgetUsed;
      if (isCount(data.budgetMax) && data.budgetMax > 0) run.budgetMax = data.budgetMax;
      run.budgetPercent = run.budgetMax > 0 ? Math.min(1, run.budgetUsed / run.budgetMax) : 0;
      break;
    default:
  }
}

const END_ANNOUNCEMENT = {
  done: 'Run finished',
  error: 'Run failed',
  cancelled: 'Run cancelled',
  closed: 'The connection closed before the run finished',
};

/** End the run in `status`; the transcript and stop reason come from the terminal frame. */
export function finishRunState(run, status, { transcript = [], stopReason = '', error = '' } = {}) {
  run.status = status;
  run.transcript = Array.isArray(transcript) ? transcript : [];
  run.records = run.transcript.map(recordView);
  run.stopReason = stopReason || '';
  run.error = error || '';
  run.announcement = END_ANNOUNCEMENT[status] || '';
}

const KIND_LABEL = {
  seed: 'Seed message',
  assistant: 'Model reply',
  tool_result: 'Tool result',
  instruction: 'Instruction',
};

/** A transcript record as the panel shows it: plain strings only. */
export function recordView(record) {
  const replay = record.replay;
  const replayText = replay === undefined || replay === null ? '' : JSON.stringify(replay, null, 2);
  const replayBytes = replayText ? new TextEncoder().encode(JSON.stringify(replay)).length : 0;
  return {
    turn: record.turn,
    kind: record.kind,
    label: KIND_LABEL[record.kind] || String(record.kind || 'Message'),
    text: record.text || '',
    bytes: isCount(record.bytes) ? record.bytes : new TextEncoder().encode(record.text || '').length,
    isError: !!record.isError,
    callId: record.callId || '',
    notSent: record.sent === false,
    notSentLabel: 'not sent to the provider',
    stubbed: record.stubbedFrom ? `replaced by a one-line stub in requests after Turn ${record.stubbedFrom}` : '',
    calls: Array.isArray(record.calls) ? record.calls.map((c) => ({ id: c.id, name: c.name, arguments: c.arguments })) : [],
    durationMs: record.durationMs || 0,
    replayLabel: replayText ? `provider payload (${replayBytes} bytes)` : '',
    replayText,
  };
}

/**
 * The run and teardown members both generation hosts spread into their data.
 * A host sets `_finishPath` (the generate route plus /finish) when it starts a
 * run, and keeps `_generationRequestId`, `generating` as it always did.
 */
export function runHost() {
  return {
    run: null,
    _runController: null,
    _reviewOwner: null,
    _finishPath: '',

    /**
     * Abort the run and any re-disclosure, settle this host's own open review as
     * null, and invalidate every late callback of the request in progress. Safe
     * to call at any time; called before a new run, and from the real teardown
     * paths.
     */
    stopGeneration() {
      this._generationRequestId = (this._generationRequestId || 0) + 1;
      const controller = this._runController;
      this._runController = null;
      controller?.abort();
      if (this._reviewOwner) {
        cancelReview(this._reviewOwner);
        this._reviewOwner = null;
      }
      this.generating = false;
      if (this.run && this.run.status === 'running') finishRunState(this.run, 'cancelled');
    },

    // The Cancel button: aborts the request; the awaiting flow reports it.
    cancelRun() {
      this._runController?.abort();
    },

    // Finish now: the server makes the next Turn the final one.
    async finishRunNow() {
      const run = this.run;
      if (!run || run.status !== 'running' || run.finishRequested || !run.runId) return;
      run.finishRequested = true;
      run.finishError = '';
      try {
        const resp = await fetch(this._finishPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ runId: run.runId }),
        });
        if (!resp.ok) {
          const body = await resp.json().catch(() => null);
          run.finishRequested = false;
          run.finishError = (body && (body.error || body.Error)) || `Could not finish the run (${resp.status}).`;
        }
      } catch (err) {
        run.finishRequested = false;
        run.finishError = err.message || 'Could not finish the run.';
      }
    },

    recordView,
    runVisible() {
      return !!this.run;
    },

    /**
     * Send a harness request and, when the server streams, drive the panel.
     * `send(signal)` performs the POST. Resolves one of
     *   { kind: 'json', resp }                 not a stream (pre-stream error, 409, JSON result)
     *   { kind: 'result' | 'error', data }     terminal frame
     *   { kind: 'closed' }                     stream ended with no terminal frame
     * and rejects with an AbortError when the run was cancelled or superseded.
     */
    async streamRun(send, finishPath) {
      this._finishPath = finishPath;
      const controller = new AbortController();
      this._runController = controller;
      const resp = await send(controller.signal);
      const type = resp.headers?.get?.('content-type') || '';
      if (!type.startsWith('text/event-stream') || !resp.body) return { kind: 'json', resp };
      this.run = newRun();
      // Through this.run, so Alpine's proxy sees every change.
      return consumeRun(resp, (name, data) => applyRunEvent(this.run, name, data), controller.signal);
    },

    // Record how the streamed run ended, once the host knows.
    endRun(status, details) {
      if (this.run) finishRunState(this.run, status, details);
    },
  };
}
