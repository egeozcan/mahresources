import { describe, expect, it, vi } from 'vitest';
import { consumeRun, newRun, applyRunEvent, recordView, finishRunState } from './templateRun.js';

const enc = new TextEncoder();
const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;

function controllableStream() {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const body = new ReadableStream<Uint8Array>({ start(c) { controller = c; } });
  return {
    response: { body } as unknown as Response,
    push: (text: string | Uint8Array) => controller.enqueue(typeof text === 'string' ? enc.encode(text) : text),
    close: () => controller.close(),
  };
}

describe('stream reader', () => {
  it('processes a turn frame while the terminal frame is still withheld, then one result', async () => {
    const s = controllableStream();
    const seen: string[] = [];
    const done = consumeRun(s.response, (name: string) => seen.push(name));
    s.push(frame('turn', { turn: 1 }));
    await vi.waitFor(() => expect(seen).toEqual(['turn']));
    let settled = false;
    done.then(() => { settled = true; });
    await new Promise((r) => setTimeout(r, 5));
    expect(settled).toBe(false);
    s.push(frame('result', { target: 'slot', content: 'x' }));
    s.close();
    expect(await done).toEqual({ kind: 'result', data: { target: 'slot', content: 'x' } });
  });

  it('handles a delimiter split across chunks and CRLF framing', async () => {
    const s = controllableStream();
    const seen: unknown[] = [];
    const done = consumeRun(s.response, (n: string, d: unknown) => seen.push([n, d]));
    s.push('event: turn\r\ndata: {"turn":2}\r\n');
    s.push('\r\n: heartbeat\n\n');
    s.push(frame('result', { ok: 1 }));
    s.close();
    await done;
    expect(seen).toEqual([['turn', { turn: 2 }]]);
  });

  it('decodes a multibyte character split across chunks', async () => {
    const s = controllableStream();
    const bytes = enc.encode(frame('result', { content: 'café \u{1F600}' }));
    const cut = bytes.indexOf(0xc3) + 1; // between the two bytes of the e-acute
    const done = consumeRun(s.response, () => {});
    s.push(bytes.slice(0, cut));
    s.push(bytes.slice(cut));
    s.close();
    expect(await done).toEqual({ kind: 'result', data: { content: 'café \u{1F600}' } });
  });

  it('delivers exactly one terminal result and ignores frames after it', async () => {
    const s = controllableStream();
    const seen: string[] = [];
    const done = consumeRun(s.response, (n: string) => seen.push(n));
    s.push(frame('result', { n: 1 }) + frame('result', { n: 2 }) + frame('turn', { turn: 9 }) + frame('error', { error: 'late' }));
    s.close();
    expect(await done).toEqual({ kind: 'result', data: { n: 1 } });
    expect(seen).toEqual([]);
  });

  it('an error frame is terminal and carries its transcript', async () => {
    const s = controllableStream();
    const done = consumeRun(s.response, () => {});
    s.push(frame('error', { error: 'nope', transcript: [{ turn: 1, kind: 'seed' }] }));
    s.close();
    expect(await done).toEqual({ kind: 'error', data: { error: 'nope', transcript: [{ turn: 1, kind: 'seed' }] } });
  });

  it('a stream that ends with no terminal frame is reported as closed', async () => {
    const s = controllableStream();
    const done = consumeRun(s.response, () => {});
    s.push(frame('turn', { turn: 1 }));
    s.close();
    expect(await done).toEqual({ kind: 'closed' });
  });

  it('a frame with malformed JSON is skipped, not fatal', async () => {
    const s = controllableStream();
    const seen: string[] = [];
    const done = consumeRun(s.response, (n: string) => seen.push(n));
    s.push('event: turn\ndata: {oops\n\n' + frame('result', { a: 1 }));
    s.close();
    expect((await done).kind).toBe('result');
    expect(seen).toEqual([]);
  });

  it('joins multi-line data fields', async () => {
    const s = controllableStream();
    const done = consumeRun(s.response, () => {});
    s.push('event: result\ndata: {"a":\ndata: 1}\n\n');
    s.close();
    expect(await done).toEqual({ kind: 'result', data: { a: 1 } });
  });

  it('rejects with AbortError when the signal aborts mid-stream', async () => {
    const s = controllableStream();
    const ac = new AbortController();
    const done = consumeRun(s.response, () => {}, ac.signal);
    ac.abort();
    await expect(done).rejects.toMatchObject({ name: 'AbortError' });
  });
});

describe('run state', () => {
  it('tracks run, turns, tools, submissions and the budget meter', () => {
    const run = newRun();
    applyRunEvent(run, 'run', { runId: 'r1', maxTurns: 10, tokenBudget: 400000, deadline: '5m0s' });
    applyRunEvent(run, 'turn', { turn: 1, maxTurns: 10 });
    applyRunEvent(run, 'tool', { turn: 1, name: 'get_entity', args: { type: 'group', id: '3' }, bytes: 120, isError: false, durationMs: 4 });
    applyRunEvent(run, 'submission', { turn: 1, field: 'CustomHeader', errors: 0, warnings: 1 });
    applyRunEvent(run, 'usage', { turn: 1, budgetUsed: 1000, budgetMax: 400000 });
    applyRunEvent(run, 'turn', { turn: 2, maxTurns: 10, finalReason: 'finish_now' });
    expect(run.runId).toBe('r1');
    expect(run.turn).toBe(2);
    expect(run.turns).toHaveLength(2);
    expect(run.turns[0].tools[0]).toMatchObject({ name: 'get_entity', bytes: 120 });
    expect(run.turns[0].submissions[0]).toMatchObject({ field: 'CustomHeader', warnings: 1 });
    expect(run.turns[1].finalReason).toBe('finish_now');
    expect(run.budgetPercent).toBeCloseTo(0.0025);
    expect(run.announcement).toBe('Turn 2 started');
  });

  it('announces only turn starts and the end state', () => {
    const run = newRun();
    applyRunEvent(run, 'turn', { turn: 1, maxTurns: 3 });
    expect(run.announcement).toBe('Turn 1 started');
    applyRunEvent(run, 'tool', { turn: 1, name: 'lint', args: {}, bytes: 1 });
    applyRunEvent(run, 'usage', { turn: 1, budgetUsed: 1, budgetMax: 2 });
    expect(run.announcement).toBe('Turn 1 started');
    finishRunState(run, 'done');
    expect(run.announcement).toBe('Run finished');
    expect(run.status).toBe('done');
  });

  it('ignores unknown events and values that are not numbers', () => {
    const run = newRun();
    applyRunEvent(run, 'whatever', {});
    applyRunEvent(run, 'turn', { turn: 'x' });
    expect(run.turns).toHaveLength(0);
  });
});

describe('transcript records', () => {
  it('exposes text exactly, and never as markup', () => {
    const v = recordView({ turn: 1, kind: 'assistant', text: '<img src=x onerror=alert(1)>', sent: true });
    expect(v.text).toBe('<img src=x onerror=alert(1)>');
    expect(v.notSent).toBe(false);
  });

  it('labels an unsent record and collapses replay as a byte count', () => {
    const v = recordView({ turn: 3, kind: 'tool_result', text: 'r', sent: false, replay: { big: 'x'.repeat(100) } });
    expect(v.notSent).toBe(true);
    expect(v.notSentLabel).toBe('not sent to the provider');
    expect(v.replayLabel).toMatch(/^provider payload \(\d+ bytes\)$/);
    expect(v.replayText).toContain('"big"');
  });

  it('shows the tool calls the model made and a stub origin', () => {
    const v = recordView({ turn: 5, kind: 'tool_result', text: '[stub]', sent: true, stubbedFrom: 1, callId: 'c1', isError: true, calls: [{ id: 'c1', name: 'lint', arguments: '{"a":1}' }] });
    expect(v.stubbed).toBe('replaced by a one-line stub in requests after Turn 1');
    expect(v.calls[0]).toMatchObject({ name: 'lint', arguments: '{"a":1}' });
    expect(v.isError).toBe(true);
  });
});
