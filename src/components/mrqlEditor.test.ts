import { afterEach, describe, expect, it, vi } from 'vitest';
import { mrqlEditor } from './mrqlEditor.js';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('mrqlEditor request lifecycle', () => {
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
