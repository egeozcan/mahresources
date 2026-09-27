// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  backgroundDownloadBody,
  backgroundDownloadOutcome,
  resourceUpload,
} from './resourceUpload.js';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('backgroundDownloadBody', () => {
  it('url-encodes the text fields and leaves a picked file out', () => {
    const body = backgroundDownloadBody([
      ['URL', 'https://example.test/a.png'],
      ['tags', '3'],
      ['tags', '4'],
      ['resource', new File(['x'], 'ignored.png')],
    ]);
    expect(body.toString()).toBe('URL=https%3A%2F%2Fexample.test%2Fa.png&tags=3&tags=4');
  });
});

describe('backgroundDownloadOutcome', () => {
  it('counts what started and lists each refusal with its reason', () => {
    expect(backgroundDownloadOutcome(1, [])).toEqual({
      notice: 'Download started. Follow it in the Jobs panel.',
      refusals: [],
    });
    expect(backgroundDownloadOutcome(2, [{ url: 'ftp://x', reason: 'unsupported scheme' }])).toEqual({
      notice: '2 downloads started. Follow them in the Jobs panel.',
      refusals: ['ftp://x: unsupported scheme'],
    });
  });
});

describe('a background download submitted from the form', () => {
  function answer(status: number, body: unknown) {
    return vi.fn(async () => new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }));
  }

  it('stays on the page, says what started and opens the Jobs panel on the new Jobs', async () => {
    const fetchMock = answer(202, {
      queued: true,
      jobs: [{ id: 'q1', canonicalJobId: 'job-1' }, { id: 'q2', canonicalJobId: 'job-2' }],
    });
    vi.stubGlobal('fetch', fetchMock);
    const opened: unknown[] = [];
    window.addEventListener('jobs-panel-open', (event) => opened.push((event as CustomEvent).detail));
    const c = resourceUpload();
    c.url = 'https://example.test/a.png\nhttps://example.test/b.png';
    c.background = true;
    const save = document.createElement('button');

    await c.postBackgroundDownload('/v1/resource/remote?background=true', new URLSearchParams({ URL: c.url }), save);

    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect((init.headers as Record<string, string>).Accept).toBe('application/json');
    expect(c.backgroundNotice).toBe('2 downloads started. Follow them in the Jobs panel.');
    expect(c.backgroundError).toBe('');
    expect(c.url).toBe('');
    expect(opened).toEqual([{ returnFocusTo: save, jobIds: ['job-1', 'job-2'] }]);
    expect(c.backgroundSubmitting).toBe(false);
  });

  it('keeps the URL and says why when the server refuses the batch', async () => {
    vi.stubGlobal('fetch', answer(400, { error: 'no valid URLs provided' }));
    const opened: unknown[] = [];
    window.addEventListener('jobs-panel-open', (event) => opened.push((event as CustomEvent).detail));
    const c = resourceUpload();
    c.url = 'not a url';
    c.background = true;

    await c.postBackgroundDownload('/v1/resource/remote?background=true', new URLSearchParams({ URL: c.url }), null);

    expect(c.backgroundError).toBe('no valid URLs provided');
    expect(c.backgroundNotice).toBe('');
    expect(c.url).toBe('not a url');
    expect(opened).toEqual([]);
  });

  it('is intercepted before the native post only when a URL is to be fetched in the background', () => {
    const c = resourceUpload();
    c.submitInBackground = vi.fn();
    const form = { querySelector: () => ({ files: [] }), getAttribute: () => '/v1/resource/remote?background=true' };
    const event = { target: form, defaultPrevented: false, preventDefault: vi.fn(), submitter: null };

    c.url = 'https://example.test/a.png';
    c.background = false;
    c.onSubmit(event as never);
    expect(event.preventDefault).not.toHaveBeenCalled();

    c.background = true;
    c.onSubmit(event as never);
    expect(event.preventDefault).toHaveBeenCalled();
    expect(c.submitInBackground).toHaveBeenCalledWith(form, null);
  });
});
