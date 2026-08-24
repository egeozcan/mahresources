import { describe, expect, it, vi } from 'vitest';
import {
  aggregateProgress,
  formatBytes,
  isPermanentFailure,
  resourceUpload,
  shouldUseClientUpload,
} from './resourceUpload.js';
import { parseUploadError } from '../utils/uploadError.js';

const GIB = 1 << 30;

describe('shouldUseClientUpload', () => {
  const base = { hasUrl: false, sizeThreshold: GIB, countThreshold: 10 };

  it('switches on file count alone', () => {
    expect(shouldUseClientUpload({ ...base, fileCount: 11, totalBytes: 1024 })).toBe(true);
  });

  it('switches on total size alone', () => {
    expect(shouldUseClientUpload({ ...base, fileCount: 2, totalBytes: GIB + 1 })).toBe(true);
  });

  it('leaves an ordinary selection on the native post', () => {
    expect(shouldUseClientUpload({ ...base, fileCount: 3, totalBytes: 5 * 1024 * 1024 })).toBe(false);
  });

  it('treats both thresholds as strictly greater', () => {
    // Exactly at the boundary must NOT switch, or "more than 10 files" would
    // silently mean "10 or more".
    expect(shouldUseClientUpload({ ...base, fileCount: 10, totalBytes: GIB })).toBe(false);
    expect(shouldUseClientUpload({ ...base, fileCount: 11, totalBytes: GIB })).toBe(true);
    expect(shouldUseClientUpload({ ...base, fileCount: 10, totalBytes: GIB + 1 })).toBe(true);
  });

  it('always defers to a URL, whatever was picked', () => {
    // Filling the URL makes the server ignore the picker entirely, so the
    // widget must not claim the submit.
    expect(shouldUseClientUpload({ ...base, hasUrl: true, fileCount: 500, totalBytes: 50 * GIB })).toBe(false);
  });

  it('does nothing with an empty picker', () => {
    expect(shouldUseClientUpload({ ...base, fileCount: 0, totalBytes: 0 })).toBe(false);
  });

  it('reads the thresholds from its input, not from constants', () => {
    const lowered = { hasUrl: false, sizeThreshold: 1024, countThreshold: 1 };
    expect(shouldUseClientUpload({ ...lowered, fileCount: 2, totalBytes: 10 })).toBe(true);
    const raised = { hasUrl: false, sizeThreshold: 100 * GIB, countThreshold: 10000 };
    expect(shouldUseClientUpload({ ...raised, fileCount: 11, totalBytes: 2 * GIB })).toBe(false);
  });
});

describe('aggregateProgress', () => {
  it('counts bytes, not files', () => {
    // One big file and nine small ones would sit at 0% then jump to 90% if this
    // counted completed files.
    const files = [
      { size: 1000, loaded: 500, status: 'uploading' },
      { size: 1000, loaded: 0, status: 'queued' },
    ];
    expect(aggregateProgress(files)).toEqual({ loaded: 500, total: 2000, percent: 25 });
  });

  it('counts a finished file at its full size regardless of its last progress event', () => {
    const files = [{ size: 1000, loaded: 12, status: 'done' }];
    expect(aggregateProgress(files).loaded).toBe(1000);
  });

  it('clamps a file that reports more loaded than its size', () => {
    const files = [{ size: 100, loaded: 999, status: 'uploading' }];
    expect(aggregateProgress(files).percent).toBe(100);
  });

  it('reports 0 rather than NaN for an empty batch', () => {
    expect(aggregateProgress([])).toEqual({ loaded: 0, total: 0, percent: 0 });
  });
});

describe('progress bar ARIA', () => {
  it('omits aria-valuenow when the total is unknown', () => {
    // ARIA says an indeterminate progressbar omits aria-valuenow; Alpine drops
    // an attribute bound to null. Pinning it at 0 announces "0 percent"
    // repeatedly while bytes are arriving.
    const c = resourceUpload();
    c.files = [];
    expect(c.progressValueNow()).toBeNull();
    expect(c.progressValueText()).toBe('Preparing upload');
  });

  it('supplies a percentage once the total is known', () => {
    const c = resourceUpload();
    c.files = [{ size: 200, loaded: 100, status: 'uploading' }];
    expect(c.progressValueNow()).toBe(50);
    expect(c.progressValueText()).toBeNull();
  });

  it('names the batch rather than emitting a bare prefix', () => {
    const c = resourceUpload();
    c.files = [{ size: 100, loaded: 100, status: 'done' }];
    c.doneCount = 1;
    expect(c.progressLabel()).not.toBe('Upload progress: ');
    expect(c.progressLabel()).toContain('1 of 1 files');
  });

  it('names each in-flight file in its own bar', () => {
    const c = resourceUpload();
    const entry = { name: 'holiday.mp4', size: 400, loaded: 100, status: 'uploading' };
    expect(c.fileValueNow(entry)).toBe(25);
    expect(c.fileLabel(entry)).toContain('holiday.mp4');
  });
});

describe('the upload pool', () => {
  /** Builds a component with uploadOne stubbed to a controllable async task. */
  function pooled(count: number, concurrency: number, run: (i: number) => Promise<void>) {
    const c = resourceUpload();
    c.concurrency = concurrency;
    c.files = Array.from({ length: count }, (_, i) => ({
      name: `f${i}`, size: 10, loaded: 0, status: 'queued', error: '', errorResourceId: null, httpStatus: 0,
    }));
    c.uploadOne = (i: number) => run(i);
    c.navigateAfterSuccess = () => {};
    return c;
  }

  it('never exceeds the configured concurrency', async () => {
    let inFlight = 0;
    let peak = 0;
    const c = pooled(12, 3, async () => {
      inFlight++;
      peak = Math.max(peak, inFlight);
      await new Promise((r) => setTimeout(r, 1));
      inFlight--;
    });

    await c.run(c.files.map((_, i) => i));

    expect(peak).toBe(3);
  });

  it('runs every file exactly once', async () => {
    const seen: number[] = [];
    const c = pooled(7, 3, async (i) => { seen.push(i); });

    await c.run(c.files.map((_, i) => i));

    expect(seen.sort((a, b) => a - b)).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  it('keeps draining the queue after a file fails', async () => {
    let completed = 0;
    const c = pooled(6, 2, async (i) => {
      if (i === 1) {
        c.files[i].status = 'failed';
        c.files[i].error = 'boom';
        return;
      }
      c.files[i].status = 'done';
      c.doneCount++;
      completed++;
    });

    await c.run(c.files.map((_, i) => i));

    expect(completed).toBe(5);
    expect(c.phase).toBe('partial');
  });

  it('survives an uploadOne that rejects rather than resolving', async () => {
    // uploadOne resolves on every path it knows about, so this is the unforeseen
    // case. It matters because an escaping rejection rejects the Promise.all in
    // run(), skips finish(), and leaves the panel stuck in 'uploading' forever
    // with Save disabled — a dead page, not a failed file.
    let completed = 0;
    const c = pooled(6, 2, async (i) => {
      if (i === 1) throw new Error('boom');
      c.files[i].status = 'done';
      c.doneCount++;
      completed++;
    });

    await expect(c.run(c.files.map((_, i) => i))).resolves.toBeUndefined();

    expect(completed).toBe(5);
    expect(c.phase).toBe('partial');
    expect(c.files[1].status).toBe('failed');
    expect(c.files[1].error).toBe('boom');
  });

  it('stops pulling new work once cancelled', async () => {
    let started = 0;
    const c = pooled(20, 2, async (i) => {
      started++;
      if (started === 2) c.cancel();
      await new Promise((r) => setTimeout(r, 1));
    });
    c.abortAll = () => {};

    await c.run(c.files.map((_, i) => i));

    expect(started).toBeLessThan(20);
    expect(c.phase).toBe('partial');
  });
});

describe('retry eligibility', () => {
  it('excludes every deterministic 4xx, not only the duplicate', () => {
    // Each of these answers identically however many times the same bytes are
    // sent, so offering Retry on them is offering a button that cannot work:
    // 409 duplicate, 413 oversized (refused in the browser), 400 undecodable
    // image, 403 stale CSRF token.
    for (const status of [400, 403, 409, 413, 422]) {
      expect(isPermanentFailure({ httpStatus: status })).toBe(true);
    }
    // The two 4xx codes that mean "try again", plus transport failures and 5xx.
    for (const status of [0, 408, 429, 500, 502, 503]) {
      expect(isPermanentFailure({ httpStatus: status })).toBe(false);
    }
  });

  it('offers retry only for the failures a retry could change', () => {
    const c = resourceUpload();
    c.files = [
      { name: 'dupe', size: 1, loaded: 0, status: 'failed', error: 'exists', httpStatus: 409 },
      { name: 'flaky', size: 1, loaded: 0, status: 'failed', error: 'network', httpStatus: 0 },
      { name: 'fine', size: 1, loaded: 1, status: 'done', error: '', httpStatus: 200 },
      { name: 'huge', size: 1, loaded: 0, status: 'failed', error: 'too big', httpStatus: 413 },
      { name: 'aborted', size: 1, loaded: 0, status: 'cancelled', error: '', httpStatus: 0 },
    ];
    expect(c.retryableIndices).toEqual([1]);
  });
});

/**
 * A stand-in for XMLHttpRequest that records what the component sent and lets a
 * test drive the outcome. Everything below goes through the real uploadOne, so
 * these assertions cannot pass against a handler that was never wired.
 */
class FakeXHR {
  static instances: FakeXHR[] = [];
  headers: Record<string, string> = {};
  method = '';
  url = '';
  status = 0;
  responseText = '';
  upload = { onprogress: null as null | ((e: { lengthComputable: boolean; loaded: number }) => void) };
  onload: null | (() => void) = null;
  onerror: null | (() => void) = null;
  onabort: null | (() => void) = null;
  sent: unknown = null;
  aborted = false;

  constructor() {
    FakeXHR.instances.push(this);
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v;
  }
  send(body: unknown) {
    this.sent = body;
  }
  abort() {
    this.aborted = true;
    this.onabort?.();
  }
}

function withFakeXHR(metaToken: string | null = 'tok-123') {
  FakeXHR.instances = [];
  // The component legitimately reaches for window/document in a browser; node
  // has neither, and an exception thrown inside a worker would be swallowed by
  // the pool's catch and misread as an ordinary upload failure.
  (globalThis as never as { window: unknown }).window = {
    addEventListener() {},
    removeEventListener() {},
  };
  (globalThis as never as { XMLHttpRequest: unknown }).XMLHttpRequest = FakeXHR;
  (globalThis as never as { document: unknown }).document = {
    addEventListener() {},
    removeEventListener() {},
    querySelector: (sel: string) =>
      sel === 'meta[name="csrf-token"]' && metaToken !== null
        ? { getAttribute: () => metaToken }
        : null,
  };
  (globalThis as never as { FormData: unknown }).FormData = class {
    entries: Array<[string, unknown]> = [];
    append(k: string, v: unknown) {
      this.entries.push([k, v]);
    }
  };
}

function componentWithOneFile() {
  const c = resourceUpload();
  c._xhrs = new Set();
  c._baseEntries = [['Name', 'batch']];
  c._action = '/v1/resource';
  c.files = [
    { file: { name: 'a.png' }, name: 'a.png', size: 100, loaded: 0, status: 'queued', error: '', errorResourceId: null, httpStatus: 0 },
  ];
  return c;
}

describe('one request', () => {
  it('carries the CSRF token and asks for JSON', async () => {
    // The global fetch wrapper in csrf.js cannot cover XMLHttpRequest, so the
    // header is set here or nowhere. Accept matters too: without it the endpoint
    // answers a 302 that XHR follows silently.
    withFakeXHR();
    const c = componentWithOneFile();

    const pending = c.uploadOne(0);
    const xhr = FakeXHR.instances[0];
    expect(xhr.headers['X-CSRF-Token']).toBe('tok-123');
    expect(xhr.headers['Accept']).toBe('application/json');
    expect(xhr.method).toBe('POST');

    xhr.status = 200;
    xhr.responseText = JSON.stringify([{ ID: 7 }]);
    xhr.onload!();
    await pending;

    expect(c.files[0].status).toBe('done');
    expect(c.files[0].resourceId).toBe(7);
    expect(c.doneCount).toBe(1);
  });

  it('omits the CSRF header entirely when auth is off', () => {
    // The meta tag renders empty under auth-off; sending an empty header would
    // be worse than sending none.
    withFakeXHR('');
    const c = componentWithOneFile();
    c.uploadOne(0);
    expect(FakeXHR.instances[0].headers).not.toHaveProperty('X-CSRF-Token');
  });

  it('reports byte progress from the upload stream', async () => {
    withFakeXHR();
    const c = componentWithOneFile();
    const pending = c.uploadOne(0);
    const xhr = FakeXHR.instances[0];

    xhr.upload.onprogress!({ lengthComputable: true, loaded: 40 });
    expect(c.files[0].loaded).toBe(40);
    expect(c.progress.percent).toBe(40);

    xhr.status = 200;
    xhr.responseText = '[]';
    xhr.onload!();
    await pending;
  });
});

describe('cancelling mid-flight', () => {
  it('does not claim an aborted upload was left unsaved', async () => {
    // Driven through cancel() and the real onabort handler, not by fabricating
    // the end state: with status set back to 'queued' this test goes red.
    //
    // Aborting an XHR stops the browser reading the response, not the server
    // processing the request. A request whose body already arrived may have been
    // committed, so an aborted row is neither counted as done nor reported as a
    // failure — it is surfaced as unknown.
    withFakeXHR();
    const c = componentWithOneFile();

    const pending = c.uploadOne(0);
    expect(c.files[0].status).toBe('uploading');

    c.cancel();
    await pending;

    expect(FakeXHR.instances[0].aborted).toBe(true);
    expect(c.files[0].status).toBe('cancelled');
    expect(c.cancelledInFlight).toHaveLength(1);
    expect(c.failed).toEqual([]);
    expect(c.retryableIndices).toEqual([]);
    expect(c.doneCount).toBe(0);
  });

  it('stops the pool and does not navigate when the component is destroyed', async () => {
    // destroy() used to abort without cancelling: the aborted request resolved,
    // the worker read `cancelled` as false, dequeued the next file and kept
    // uploading from a detached component — and finish() could navigate.
    withFakeXHR();
    const c = resourceUpload();
    c._xhrs = new Set();
    c.concurrency = 1;
    c.navigateAfterSuccess = vi.fn();
    let started = 0;
    c.uploadOne = async (i: number) => {
      started++;
      if (started === 1) c.destroy();
      c.files[i].status = 'done';
      c.doneCount++;
    };
    c.files = Array.from({ length: 5 }, (_, i) => ({
      name: `f${i}`, size: 1, loaded: 0, status: 'queued', error: '', errorResourceId: null, httpStatus: 0,
    }));

    await c.run(c.files.map((_, i) => i));

    expect(started).toBe(1);
    expect(c.navigateAfterSuccess).not.toHaveBeenCalled();
  });
});

describe('starting a new batch', () => {
  it('clears a finished batch when files are chosen again', () => {
    // Otherwise the panel from a partial run stays on screen describing files
    // that are no longer selected, and Save stays disabled with no way back.
    const c = resourceUpload();
    c.countThreshold = 10;
    c.sizeThreshold = 1 << 30;
    c.phase = 'partial';
    c.doneCount = 4;
    c.files = [{ name: 'old', size: 1, loaded: 0, status: 'failed', error: 'x', httpStatus: 500 }];

    c.onFilesChosen({ target: { files: [{ size: 10 }, { size: 10 }] } } as never);

    expect(c.phase).toBe('idle');
    expect(c.files).toEqual([]);
    expect(c.doneCount).toBe(0);
  });

  it('leaves a running batch alone', () => {
    const c = resourceUpload();
    c.phase = 'uploading';
    c.doneCount = 2;
    c.files = [{ name: 'live', size: 1, loaded: 0, status: 'uploading', error: '', httpStatus: 0 }];

    c.onFilesChosen({ target: { files: [{ size: 10 }] } } as never);

    expect(c.phase).toBe('uploading');
    expect(c.doneCount).toBe(2);
  });
});

describe('submit interception', () => {
  function submitEvent(files: Array<{ size: number }>, defaultPrevented = false) {
    const input = { files, type: 'file', name: 'resource' };
    const form = {
      querySelector: () => input,
      getAttribute: () => '/v1/resource',
    };
    return {
      target: form,
      defaultPrevented,
      preventDefault: vi.fn(),
    };
  }

  it('leaves a small batch to the native post', () => {
    const c = resourceUpload();
    const e = submitEvent([{ size: 10 }, { size: 10 }]);
    c.onSubmit(e as never);
    expect(e.preventDefault).not.toHaveBeenCalled();
    expect(c.phase).toBe('idle');
  });

  it('does not upload past a schema validation failure', () => {
    const c = resourceUpload();
    c.run = vi.fn();
    const e = submitEvent(Array.from({ length: 20 }, () => ({ size: 10 })), true);
    c.onSubmit(e as never);
    expect(c.run).not.toHaveBeenCalled();
    expect(e.preventDefault).not.toHaveBeenCalled();
  });
});

describe('formatBytes', () => {
  it('scales units and keeps whole bytes unfractioned', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(GIB)).toBe('1.0 GB');
  });
});

describe('parseUploadError after the move out of pasteUpload', () => {
  it('prefers the per-file detail and carries the colliding resource id', () => {
    const body = JSON.stringify({
      error: '1 of 1 files could not be saved: resource already exists',
      details: [{ error: 'resource already exists', existingResourceId: 52 }],
    });
    expect(parseUploadError(body, 409)).toEqual({
      message: 'Resource already exists',
      resourceId: 52,
    });
  });

  it('falls back to the aggregate error, then to raw text', () => {
    expect(parseUploadError(JSON.stringify({ error: 'nope' }), 500).message).toBe('Nope');
    expect(parseUploadError('plain text failure', 500).message).toBe('Plain text failure');
    expect(parseUploadError('', 503).message).toBe('Upload failed (HTTP 503)');
  });
});
