import { afterEach, describe, expect, it, vi } from 'vitest';
import { mrqlEditor } from './mrqlEditor.js';

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
