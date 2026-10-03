import { afterEach, describe, expect, it, vi } from 'vitest';
import { mrqlEditor } from './mrqlEditor.js';
import { listRendering } from '../utils/listContainer.js';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function editorWithSelection() {
  const selection = { reset: vi.fn(), refresh: null as null | (() => Promise<void> | void) };
  vi.stubGlobal('window', { Alpine: { store: (name: string) => name === 'selection:mrql-note' ? selection : null } });
  const editor = mrqlEditor() as any;
  editor.$nextTick = (callback: () => void) => callback();
  editor.executedQuery = { query: 'type = note AND name = $name', params: { name: 'original' } };
  editor.executedEntityTypes = ['note'];
  editor.displayPage = 2;
  editor.getQuery = () => 'type = note AND name = "draft"';
  editor.paramsPayload = () => ({});
  editor.connectSelections();
  return { editor, selection };
}

const noteResponse = { ok: true, json: async () => ({ notes: [{ ID: 1 }], listPage: { page: 2, total: 30 } }) };

describe('mrqlEditor request lifecycle', () => {
  it('does not let an old mutation abort a newer Run, even before its response arrives', async () => {
    const { editor, selection } = editorWithSelection();
    const oldRefresh = selection.refresh!;
    let complete!: (response: typeof noteResponse) => void;
    const request = vi.fn(() => new Promise(resolve => { complete = resolve; }));
    vi.stubGlobal('fetch', request);

    const running = editor.execute({ pushState: false });
    const signal = (request.mock.calls[0] as any)[1].signal;
    const refreshing = oldRefresh();

    expect(request).toHaveBeenCalledTimes(1);
    expect(signal.aborted).toBe(false);
    complete(noteResponse);
    await Promise.all([running, refreshing]);
    expect(editor.executedQuery.query).toBe(editor.getQuery());
  });

  it('tells the media viewer a Run is rendering until the newest one settles', async () => {
    const { editor } = editorWithSelection();
    const completes: Array<(response: typeof noteResponse) => void> = [];
    vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => { completes.push(resolve); })));

    const first = editor.execute({ pushState: false });
    // A Run replaces it (Ctrl+Enter while Forward waits for the cards).
    const second = editor.execute({ pushState: false });
    completes[0](noteResponse);
    await first;
    expect(listRendering()).not.toBeNull();
    completes[1](noteResponse);
    await second;
    await Promise.resolve();
    expect(listRendering()).toBeNull();
  });

  it('holds the render until the tick that collects the viewer\'s gallery has run', async () => {
    const { editor } = editorWithSelection();
    const ticks: Array<() => void> = [];
    editor.$nextTick = (callback: () => void) => { ticks.push(callback); };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(noteResponse));
    await editor.execute({ pushState: false });
    await Promise.resolve();
    expect(listRendering()).not.toBeNull();
    ticks.forEach(tick => tick());
    await Promise.resolve();
    await Promise.resolve();
    expect(listRendering()).toBeNull();
  });

  it('runs a refresh asked for as soon as the previous one has finished', async () => {
    const { editor, selection } = editorWithSelection();
    // Alpine runs $nextTick in a setTimeout, a macrotask after execute() resolves.
    editor.$nextTick = (callback: () => void) => setTimeout(callback, 0);
    const request = vi.fn().mockResolvedValue(noteResponse);
    vi.stubGlobal('fetch', request);

    await selection.refresh!();
    // The lightbox's trailing page refresh comes a few microtasks later, before that tick.
    await selection.refresh!();

    expect(request).toHaveBeenCalledTimes(2);
    // Let the deferred ticks run while window is still stubbed.
    await new Promise(resolve => setTimeout(resolve, 0));
  });

  it('does not interrupt paging with a callback from the page being replaced', async () => {
    const { editor, selection } = editorWithSelection();
    const oldRefresh = selection.refresh!;
    let complete!: (response: typeof noteResponse) => void;
    const request = vi.fn(() => new Promise(resolve => { complete = resolve; }));
    vi.stubGlobal('fetch', request);

    const running = editor.execute({ pushState: false, snapshot: editor.executedQuery, displayPage: 2, reuseResult: true });
    const refreshing = oldRefresh();
    expect(request).toHaveBeenCalledTimes(1);
    complete(noteResponse);
    await Promise.all([running, refreshing]);
  });

  it('invalidates scoped and background callbacks when results are deliberately cleared', async () => {
    vi.useFakeTimers();
    const { editor, selection } = editorWithSelection();
    const oldRefresh = selection.refresh!;
    editor.execute = vi.fn();
    editor.refreshForCompletedAction({ entityType: 'note' });

    editor.clearResult();
    await oldRefresh();
    editor.refreshForCompletedAction({ entityType: 'note' });
    vi.runAllTimers();

    expect(editor.execute).not.toHaveBeenCalled();
    expect(editor.executedQuery).toBeNull();
  });

  it('still refreshes the displayed snapshot after editing an unexecuted draft', async () => {
    const { editor, selection } = editorWithSelection();
    const original = editor.executedQuery;
    const request = vi.fn().mockResolvedValue(noteResponse);
    vi.stubGlobal('fetch', request);

    editor.cancelStaleQueryRequests();
    await selection.refresh!();

    const payload = JSON.parse(request.mock.calls[0][1].body);
    expect(payload).toMatchObject({ ...original, displayPage: 2 });
    expect(editor.getQuery()).toBe('type = note AND name = "draft"');
  });

  it('cancels execute and explain requests when the query changes', () => {
    const editor = mrqlEditor() as any;
    const executeAbort = vi.fn();
    const explainAbort = vi.fn();
    editor._executeController = { abort: executeAbort };
    editor._explainController = { abort: explainAbort };
    editor._executeRequestId = 3;
    editor._explainRequestId = 4;
    editor.executing = true;
    editor.explaining = true;

    editor.cancelStaleQueryRequests();

    expect(executeAbort).toHaveBeenCalledOnce();
    expect(explainAbort).toHaveBeenCalledOnce();
    expect(editor._executeRequestId).toBe(4);
    expect(editor._explainRequestId).toBe(5);
    expect(editor.executing).toBe(false);
    expect(editor.explaining).toBe(false);
  });

  it('surfaces export preflight errors before opening a download frame', async () => {
    const editor = mrqlEditor() as any;
    editor.getQuery = () => 'type = resource LIMIT 10001';
    editor.paramsPayload = () => ({});
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ error: 'MRQL limit exceeds maximum' }),
    }));

    await editor.exportResults('json');

    expect(editor.error).toBe('MRQL limit exceeds maximum');
    expect(editor.exporting).toBe(false);
  });
});

describe('refresh keeps the bulk selection', () => {
  function editorWithRealSelection() {
    const options: Record<number, unknown> = {};
    const selection: any = {
      selectedIds: new Set<number>(),
      options,
      reset: vi.fn(() => { selection.selectedIds.clear(); for (const id in options) delete options[id]; }),
      restoreSelection: vi.fn((ids: number[]) => { for (const id of ids) if (options[id]) selection.selectedIds.add(id); }),
      refresh: null,
    };
    vi.stubGlobal('window', { Alpine: { store: (name: string) => name === 'selection:mrql-note' ? selection : null } });
    const editor = mrqlEditor() as any;
    // The result cards register as they render, before the tick's callback runs.
    editor.$nextTick = (callback: () => void) => {
      for (const note of editor.result?.notes ?? []) options[note.ID] = { itemId: note.ID };
      callback();
    };
    editor.executedQuery = { query: 'type = note', params: {} };
    editor.executedEntityTypes = ['note'];
    editor.connectSelections();
    return { editor, selection };
  }

  it('re-selects the cards still listed after the selection store asks for a refresh', async () => {
    const { selection } = editorWithRealSelection();
    selection.selectedIds = new Set([1, 2]);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ notes: [{ ID: 1 }, { ID: 3 }] }) }));
    await selection.refresh();
    expect([...selection.selectedIds]).toEqual([1]);
  });

  it('keeps the selection across a second refresh asked for before the first one\'s tick', async () => {
    const { editor, selection } = editorWithRealSelection();
    const ticks: Array<() => void> = [];
    // Alpine runs $nextTick in a macrotask: the lightbox's trailing refresh starts before it.
    editor.$nextTick = (callback: () => void) => ticks.push(() => {
      for (const note of editor.result?.notes ?? []) selection.options[note.ID] = { itemId: note.ID };
      callback();
    });
    selection.selectedIds = new Set([1, 2]);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ notes: [{ ID: 1 }, { ID: 2 }] }) }));
    await selection.refresh();
    await selection.refresh();
    for (const tick of ticks) tick();
    expect([...selection.selectedIds].sort()).toEqual([1, 2]);
  });

  it('does not put a refresh\'s selection back into the results of a newer Run', async () => {
    const { editor, selection } = editorWithRealSelection();
    const ticks: Array<() => void> = [];
    editor.$nextTick = (callback: () => void) => ticks.push(() => {
      for (const note of editor.result?.notes ?? []) selection.options[note.ID] = { itemId: note.ID };
      callback();
    });
    editor.getQuery = () => 'type = note AND name ~ "a"';
    editor.paramsPayload = () => ({});
    selection.selectedIds = new Set([1]);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ notes: [{ ID: 1 }] }) }));
    await selection.refresh();
    await editor.execute({ pushState: false });
    for (const tick of ticks) tick();
    expect(selection.selectedIds.size).toBe(0);
  });

  it('still clears the selection when the reader runs a query', async () => {
    const { editor, selection } = editorWithRealSelection();
    selection.selectedIds = new Set([1]);
    editor.getQuery = () => 'type = note';
    editor.paramsPayload = () => ({});
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ notes: [{ ID: 1 }] }) }));
    await editor.execute({ pushState: false });
    expect(selection.selectedIds.size).toBe(0);
  });
});

describe('background action completion', () => {
  it('collects another action completion after a refresh of the same snapshot finishes', async () => {
    vi.useFakeTimers();
    const { editor, selection } = editorWithSelection();
    let complete!: (response: typeof noteResponse) => void;
    const request = vi.fn().mockImplementationOnce(() => new Promise(resolve => { complete = resolve; }))
      .mockResolvedValue(noteResponse);
    vi.stubGlobal('fetch', request);

    const running = selection.refresh!();
    editor.refreshForCompletedAction({ entityType: 'note' });
    await vi.advanceTimersByTimeAsync(100);
    expect(request).toHaveBeenCalledTimes(1);
    complete(noteResponse);
    await running;
    await vi.runAllTimersAsync();

    expect(request).toHaveBeenCalledTimes(2);
    expect(JSON.parse(request.mock.calls[1][1].body)).toMatchObject({
      query: 'type = note AND name = $name', params: { name: 'original' }, displayPage: 2,
    });
  });

  it('refreshes the executed query after a relevant action, keeping its page and parameters', () => {
    vi.useFakeTimers();
    try {
      const editor = mrqlEditor() as any;
      const snapshot = {query: 'type = note AND name = $name', params: {name: 'saved'}};
      editor.executedQuery = snapshot;
      editor.executedEntityTypes = ['note'];
      editor.displayPage = 2;
      editor.execute = vi.fn();
      editor.refreshForCompletedAction({entityType: 'resource'});
      vi.runAllTimers();
      expect(editor.execute).not.toHaveBeenCalled();
      editor.refreshForCompletedAction({entityType: 'note'});
      vi.runAllTimers();
      expect(editor.execute).toHaveBeenCalledWith({pushState:false, snapshot, displayPage:2});
      editor.execute.mockClear();
      editor.refreshForCompletedAction({entityType: 'note'});
      editor.executedQuery = {query:'type = group', params:{}};
      vi.runAllTimers();
      expect(editor.execute).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
