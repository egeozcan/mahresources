// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { setupDownloadListRefresh } from './downloadListRefresh.js';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function completion(id: number, resourceId: number | null = id) {
  return new CustomEvent('download-completed', {
    detail: { id, resourceId, status: 'completed' },
  });
}

function pageResponse(markup = '<div data-list-container>new</div>', status = 200) {
  return new Response(`<!doctype html><body>${markup}</body>`, { status });
}

describe('download completion list refresh', () => {
  let teardown: (() => void) | undefined;

  beforeEach(() => {
    vi.useFakeTimers();
    document.body.innerHTML = '<main><div data-list-container>old</div></main>';
  });

  afterEach(() => {
    teardown?.();
    teardown = undefined;
    vi.useRealTimers();
    vi.restoreAllMocks();
    document.body.replaceChildren();
  });

  test('coalesces a 120-completion burst into one page fetch', async () => {
    const pending = deferred<Response>();
    const fetchImpl = vi.fn(() => pending.promise);
    teardown = setupDownloadListRefresh({
      fetchImpl,
      currentURL: () => '/resources?page=32',
      morphList: vi.fn(),
      afterRefresh: vi.fn(),
      debounceMs: 500,
    });

    for (let id = 1; id <= 120; id += 1) {
      window.dispatchEvent(completion(id));
    }

    expect(fetchImpl).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(500);
    expect(fetchImpl).toHaveBeenCalledTimes(1);

    pending.resolve(pageResponse());
    await vi.runAllTimersAsync();
  });

  test('serializes an active refresh and performs one trailing dirty refresh', async () => {
    const first = deferred<Response>();
    const second = deferred<Response>();
    const fetchImpl = vi.fn()
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);
    teardown = setupDownloadListRefresh({
      fetchImpl,
      morphList: vi.fn(),
      afterRefresh: vi.fn(),
      debounceMs: 500,
    });

    window.dispatchEvent(completion(1));
    await vi.advanceTimersByTimeAsync(500);
    expect(fetchImpl).toHaveBeenCalledTimes(1);

    for (let id = 2; id <= 120; id += 1) window.dispatchEvent(completion(id));
    await vi.advanceTimersByTimeAsync(500);
    expect(fetchImpl).toHaveBeenCalledTimes(1);

    first.resolve(pageResponse());
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(499);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchImpl).toHaveBeenCalledTimes(2);

    second.resolve(pageResponse());
    await vi.runAllTimersAsync();
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  test('refreshes within the debounce bound and identifies the request origin', async () => {
    const morphList = vi.fn();
    const afterRefresh = vi.fn();
    const fetchImpl = vi.fn().mockResolvedValue(pageResponse());
    teardown = setupDownloadListRefresh({
      fetchImpl,
      currentURL: () => '/resources?ownerId=371',
      morphList,
      afterRefresh,
      debounceMs: 500,
    });

    window.dispatchEvent(completion(1));
    await vi.advanceTimersByTimeAsync(499);
    expect(fetchImpl).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);

    expect(fetchImpl).toHaveBeenCalledWith('/resources?ownerId=371', {
      headers: {
        'Accept': 'text/html',
        'X-Mahresources-Refresh-Reason': 'download-completed',
      },
    });
    expect(morphList).toHaveBeenCalledTimes(1);
    expect(afterRefresh).toHaveBeenCalledTimes(1);
  });

  test('ignores events without a resource and pages without a resource list', async () => {
    const fetchImpl = vi.fn();
    teardown = setupDownloadListRefresh({ fetchImpl, debounceMs: 500 });

    window.dispatchEvent(completion(1, null));
    document.body.replaceChildren();
    window.dispatchEvent(completion(2));
    await vi.runAllTimersAsync();

    expect(fetchImpl).not.toHaveBeenCalled();
  });

  test.each([
    ['HTTP failure', () => Promise.resolve(pageResponse('failure', 503))],
    ['response parse failure', () => Promise.resolve({
      ok: true,
      status: 200,
      text: () => Promise.reject(new Error('invalid response body')),
    } as Response)],
  ])('recovers from an %s on the next completion', async (_name, firstResult) => {
    const logger = { error: vi.fn() };
    const fetchImpl = vi.fn()
      .mockImplementationOnce(firstResult)
      .mockResolvedValueOnce(pageResponse());
    teardown = setupDownloadListRefresh({
      fetchImpl,
      morphList: vi.fn(),
      afterRefresh: vi.fn(),
      logger,
      debounceMs: 500,
    });

    window.dispatchEvent(completion(1));
    await vi.advanceTimersByTimeAsync(500);
    expect(logger.error).toHaveBeenCalledTimes(1);

    window.dispatchEvent(completion(2));
    await vi.advanceTimersByTimeAsync(500);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  test('morphs every list when the refreshed page has the same multi-list shape', async () => {
    document.body.innerHTML = `
      <main>
        <div data-list-container id="current-primary">old primary</div>
        <div data-list-container id="current-related">old related</div>
      </main>`;
    const morphList = vi.fn();
    const afterRefresh = vi.fn();
    teardown = setupDownloadListRefresh({
      fetchImpl: vi.fn().mockResolvedValue(pageResponse(`
        <div data-list-container id="fresh-primary">new primary</div>
        <div data-list-container id="fresh-related">new related</div>`)),
      morphList,
      afterRefresh,
      debounceMs: 500,
    });

    window.dispatchEvent(completion(1));
    await vi.advanceTimersByTimeAsync(500);

    expect(morphList).toHaveBeenCalledTimes(2);
    expect(morphList.mock.calls.map(([current, refreshed]) => [current.id, refreshed.id])).toEqual([
      ['current-primary', 'fresh-primary'],
      ['current-related', 'fresh-related'],
    ]);
    expect(afterRefresh).toHaveBeenCalledTimes(1);
  });

  test('falls back to the first list when the refreshed multi-list shape differs', async () => {
    document.body.innerHTML = `
      <main>
        <div data-list-container id="current-primary">old primary</div>
        <div data-list-container id="current-related">old related</div>
      </main>`;
    const morphList = vi.fn();
    teardown = setupDownloadListRefresh({
      fetchImpl: vi.fn().mockResolvedValue(pageResponse(
        '<div data-list-container id="fresh-primary">new primary</div>',
      )),
      morphList,
      debounceMs: 500,
    });

    window.dispatchEvent(completion(1));
    await vi.advanceTimersByTimeAsync(500);

    expect(morphList).toHaveBeenCalledTimes(1);
    expect(morphList.mock.calls[0].map((element) => element.id)).toEqual([
      'current-primary',
      'fresh-primary',
    ]);
  });
});
