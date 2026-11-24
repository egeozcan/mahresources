import { afterEach, describe, expect, it, vi } from 'vitest';

import { progressText, computeMethods } from './reductionCompute.js';

describe('progressText', () => {
  it('counts while the phase has a total', () => {
    expect(progressText({ phase: 'Identical Resources', phaseCount: 1200, phaseTotal: 8000 })).toBe('Comparing… 1,200 of 8,000');
  });

  it('names the phase while it has none', () => {
    expect(progressText({ phase: 'resolving the Extent' })).toBe('Resolving the Extent…');
  });

  it('says it is starting before the first phase', () => {
    expect(progressText({ phase: 'clustering' })).toBe('Starting…');
    expect(progressText({})).toBe('Starting…');
  });
});

class FakeEventSource {
  static last: FakeEventSource | null = null;
  listeners: Record<string, (event: { data: string }) => void> = {};
  onerror: (() => void) | null = null;
  constructor() { FakeEventSource.last = this; }
  addEventListener(name: string, fn: (event: { data: string }) => void) { this.listeners[name] = fn; }
  close() {}
}

function fakeStore() {
  return { ...computeMethods, reductionId: 4, jobId: 'job-1', progress: { text: '', count: 0, total: 0 }, mahAnnounce: undefined } as any;
}

describe('following a compute job', () => {
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

  it('shows "Comparing… N of M" when a progress event arrives', () => {
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('window', {});
    const store = fakeStore();
    store.watchCompute();
    FakeEventSource.last!.listeners.updated({ data: JSON.stringify({ job: { id: 'job-1', status: 'running', phase: 'comparing', phaseCount: 3, phaseTotal: 1200 } }) });
    expect(store.progress.text).toBe('Comparing… 3 of 1,200');
    expect(store.progress.total).toBe(1200);
    store.stopWatching();
  });

  it('polls the row beside the stream, so a viewer who sees no events still finishes', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', FakeEventSource);
    const reload = vi.fn();
    vi.stubGlobal('window', { location: { reload } });
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ reduction: { StatusEffective: 'ready' }, raw: {} }) })));
    const store = fakeStore();
    store.watchCompute();
    await vi.advanceTimersByTimeAsync(5100);
    expect(reload).toHaveBeenCalled();
  });
});
