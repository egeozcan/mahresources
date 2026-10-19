/**
 * The per-block save queue in blockEditor (design doc
 * docs/plans/2026-10-19-markdown-description-editor-design.md, decision 13):
 * at most one PUT in flight per block, the latest text sent last, a stale
 * response never applied over a newer local edit, failures reported.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { reactive } from '@vue/reactivity';
import { blockEditor } from './blockEditor.js';

type Pending = { url: string; body: any; resolve: (r: Response) => void };

let pending: Pending[];

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

// Wrapped in the reactivity Alpine uses, so a check that breaks behind its
// proxies (stored objects read back as proxies) fails here too.
function editor(blocks: any[]) {
  const ed: any = reactive(blockEditor(1, blocks));
  ed.$nextTick = (fn: () => void) => fn();
  return ed;
}

beforeEach(() => {
  pending = [];
  vi.stubGlobal('fetch', vi.fn((url: string, init: any) => new Promise<Response>((resolve) => {
    pending.push({ url, body: init?.body ? JSON.parse(init.body) : null, resolve });
  })));
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const flush = () => new Promise((r) => setTimeout(r, 0));

describe('blockEditor save queue', () => {
  it('keeps one PUT in flight per block and sends only the latest text next', async () => {
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    const first = ed.updateBlockContent(7, { text: 'a' });
    await flush(); // 'a' is now in flight
    ed.updateBlockContent(7, { text: 'ab' });
    const last = ed.updateBlockContent(7, { text: 'abc' });
    await flush();
    expect(pending).toHaveLength(1);
    expect(pending[0].body.content.text).toBe('a');

    pending[0].resolve(jsonResponse(200, { content: { text: 'a' } }));
    await first;
    await flush();
    expect(pending).toHaveLength(2);
    expect(pending[1].body.content.text).toBe('abc');
    pending[1].resolve(jsonResponse(200, { content: { text: 'abc' } }));
    await last;
    expect(ed.blocks[0].content.text).toBe('abc');
  });

  it('does not apply a save response over a newer local edit', async () => {
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    const save = ed.updateBlockContent(7, { text: 'a' });
    await flush();
    vi.useFakeTimers();
    ed.updateBlockContentDebounced(7, { text: 'ab' });
    pending[0].resolve(jsonResponse(200, { content: { text: 'a' } }));
    await save;
    expect(ed.blocks[0].content.text).toBe('ab');
  });

  it('an immediate save cancels the pending debounced one', async () => {
    vi.useFakeTimers();
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    ed.updateBlockContentDebounced(7, { text: 'a' });
    ed.updateBlockContent(7, { text: 'a' });
    await vi.advanceTimersByTimeAsync(1000);
    expect(pending).toHaveLength(1);
  });

  it('keeps separate debounce timers per block', async () => {
    vi.useFakeTimers();
    const ed = editor([
      { id: 7, type: 'text', content: { text: 'x' } },
      { id: 8, type: 'text', content: { text: 'y' } },
    ]);
    ed.updateBlockContentDebounced(7, { text: 'x1' });
    ed.updateBlockContentDebounced(8, { text: 'y1' });
    await vi.advanceTimersByTimeAsync(1000);
    expect(pending.map((p) => p.body.content.text).sort()).toEqual(['x1', 'y1']);
  });

  it('reports a failed save and refuses to leave edit mode', async () => {
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    ed.editMode = true;
    const save = ed.updateBlockContent(7, { text: 'a' });
    const leave = ed.toggleEditMode();
    await flush();
    pending[0].resolve(jsonResponse(500, { error: 'disk full' }));
    expect(await save).toBe(false);
    await leave;
    expect(ed.editMode).toBe(true);
    expect(ed.error).toBe('disk full');
  });

  it('leaves edit mode once every pending save succeeds', async () => {
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    ed.editMode = true;
    ed.updateBlockContent(7, { text: 'a' });
    const leave = ed.toggleEditMode();
    await flush();
    expect(ed.editMode).toBe(true);
    pending[0].resolve(jsonResponse(200, { content: { text: 'a' } }));
    await leave;
    expect(ed.editMode).toBe(false);
  });

  it('does not write an already-saved older value back on unload', async () => {
    const ed = editor([{ id: 7, type: 'heading', content: { text: 'g', level: 2 } }]);
    // Typing (debounced), then blur (immediate) with an equal but new object,
    // as blockHeading does.
    ed.updateBlockContentDebounced(7, { text: 'h', level: 2 });
    const one = ed.updateBlockContent(7, { text: 'h', level: 2 });
    await flush();
    pending[0].resolve(jsonResponse(200, { content: { text: 'h', level: 2 } }));
    await one;
    const two = ed.updateBlockContent(7, { text: 'h', level: 3 });
    await flush();
    pending[1].resolve(jsonResponse(200, { content: { text: 'h', level: 3 } }));
    await two;
    ed.flushPendingUpdates();
    expect(pending).toHaveLength(2);
  });

  it('does not apply a queued save response over an edit made before it was sent', async () => {
    vi.useFakeTimers();
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    const a = ed.updateBlockContent(7, { text: 'A' });
    await vi.advanceTimersByTimeAsync(0);
    const b = ed.updateBlockContent(7, { text: 'B' }); // queued behind A
    ed.updateBlockContentDebounced(7, { text: 'C' }); // its timer is still pending
    pending[0].resolve(jsonResponse(200, { content: { text: 'A' } }));
    await a;
    await vi.advanceTimersByTimeAsync(0);
    expect(pending[1].body.content.text).toBe('B');
    pending[1].resolve(jsonResponse(200, { content: { text: 'B' } }));
    await b;
    expect(ed.blocks[0].content.text).toBe('C');
  });

  it('a save that fails after its block was deleted does not block Done', async () => {
    const ed = editor([
      { id: 7, type: 'text', content: { text: 'x' } },
      { id: 8, type: 'text', content: { text: 'y' } },
    ]);
    ed.editMode = true;
    const save = ed.updateBlockContent(7, { text: 'x1' });
    await flush();
    const del = ed.deleteBlock(7);
    await flush();
    pending[1].resolve(jsonResponse(200, {}));
    await del;
    pending[0].resolve(jsonResponse(404, { error: 'block not found' }));
    await save;
    await ed.toggleEditMode();
    expect(ed.editMode).toBe(false);
  });

  it('Done waits for edits typed while it was waiting', async () => {
    vi.useFakeTimers();
    const ed = editor([{ id: 7, type: 'text', content: { text: 'old' } }]);
    ed.editMode = true;
    ed.updateBlockContent(7, { text: 'A' });
    const leave = ed.toggleEditMode();
    await vi.advanceTimersByTimeAsync(0);
    ed.updateBlockContentDebounced(7, { text: 'AC' });
    pending[0].resolve(jsonResponse(200, { content: { text: 'A' } }));
    await vi.advanceTimersByTimeAsync(0);
    expect(ed.editMode).toBe(true);
    await vi.advanceTimersByTimeAsync(600);
    expect(pending[1]?.body.content.text).toBe('AC');
    pending[1].resolve(jsonResponse(200, { content: { text: 'AC' } }));
    await leave;
    expect(ed.editMode).toBe(false);
  });
});
