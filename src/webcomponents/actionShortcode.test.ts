// @vitest-environment happy-dom

// The [action] button: what a click sends, what each answer of the run
// endpoint does to the status text, the live region and the [autorefresh]
// signal, and how an async run is followed.
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import Alpine from 'alpinejs';
import morph from '@alpinejs/morph';
import { morphAndReinitChangedComponents } from '../utils/shortcodeElementMorph.js';

const askToConfirm = vi.fn();
vi.mock('../components/confirmDialog.js', () => ({ askToConfirm: (...args: any[]) => askToConfirm(...args) }));

const FIVE_MINUTES = 5 * 60 * 1000;
let POLL_MS: number;
let announce: ReturnType<typeof vi.fn>;
let fetchMock: ReturnType<typeof vi.fn>;
let events: { type: string; detail: any }[];

const record = (e: Event) => events.push({ type: e.type, detail: (e as CustomEvent).detail });

beforeAll(async () => {
  Alpine.plugin(morph);
  ({ POLL_MS } = await import('./actionShortcode.js'));
});

beforeEach(() => {
  announce = vi.fn();
  (window as any).mahAnnounce = announce;
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  askToConfirm.mockReset();
  events = [];
  document.addEventListener('meta-shortcode-saving', record);
  document.addEventListener('meta-shortcode-updated', record);
});

afterEach(() => {
  document.removeEventListener('meta-shortcode-saving', record);
  document.removeEventListener('meta-shortcode-updated', record);
  document.body.innerHTML = '';
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function mount({ confirm = '', async = false, params = '{"scale":2,"tag":"x"}' } = {}) {
  document.body.innerHTML =
    `<div class="list-container"><article><action-shortcode data-plugin="fal-ai" data-action="upscale" data-entity-type="resource" data-entity-id="42"` +
    ` data-async="${async}" data-label="Upscale image"` +
    (confirm ? ` data-confirm="${confirm}"` : '') +
    ` data-params='${params}'>` +
    `<button type="button" class="action-shortcode-button">Upscale image</button><span class="action-shortcode-status"></span></action-shortcode></article></div>`;
  const list = document.querySelector('.list-container')!;
  const host = list.querySelector('action-shortcode') as HTMLElement;
  const serverHTML = list.outerHTML;
  return {
    host,
    button: host.querySelector('button') as HTMLButtonElement,
    status: host.querySelector('.action-shortcode-status') as HTMLElement,
    morphList(entityId: number) {
      vi.stubGlobal('Alpine', Alpine);
      const incoming = new DOMParser().parseFromString(serverHTML, 'text/html').querySelector('.list-container')!;
      (incoming.querySelector('action-shortcode') as HTMLElement).dataset.entityId = String(entityId);
      morphAndReinitChangedComponents(list, incoming);
      expect(list.querySelector('action-shortcode')).toBe(host);
    },
  };
}

function answer(status: number, body: any) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

const settle = () => new Promise((r) => setTimeout(r, 0));

describe('action-shortcode', () => {
  it('posts the action, the entity and the server-resolved params', async () => {
    const { button } = mount();
    fetchMock.mockResolvedValue(answer(200, { success: true }));
    button.click();
    await settle();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/v1/jobs/action/run');
    expect(init.method).toBe('POST');
    expect(init.headers['Content-Type']).toBe('application/json');
    expect(JSON.parse(init.body)).toEqual({ plugin: 'fal-ai', action: 'upscale', entity_ids: [42], params: { scale: 2, tag: 'x' } });
  });

  it('asks the confirmation first and sends nothing when it is declined', async () => {
    const { button } = mount({ confirm: 'Run upscale?' });
    askToConfirm.mockResolvedValue(false);
    button.click();
    await settle();
    expect(askToConfirm).toHaveBeenCalledWith('Run upscale?', expect.objectContaining({ title: 'Upscale image', confirmLabel: 'Upscale image', destructive: false }));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(button.hasAttribute('aria-disabled')).toBe(false);

    askToConfirm.mockResolvedValue(true);
    fetchMock.mockResolvedValue(answer(200, { success: true }));
    button.click();
    await settle();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it.each(['entityId', 'params'] as const)('cancels confirmation if the host changes %s', async (field) => {
    const { host, button } = mount({ confirm: 'Run upscale?' });
    let acceptConfirmation!: (value: boolean) => void;
    askToConfirm.mockReturnValue(new Promise((resolve) => { acceptConfirmation = resolve; }));
    button.click();
    host.dataset[field] = field === 'entityId' ? '43' : '{"scale":4}';
    acceptConfirmation(true);
    await settle();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(events).toEqual([]);
  });

  it('cancels confirmation even if a disconnected host was reinserted before acceptance', async () => {
    const { host, button } = mount({ confirm: 'Run upscale?' });
    let acceptConfirmation!: (value: boolean) => void;
    askToConfirm.mockReturnValue(new Promise((resolve) => { acceptConfirmation = resolve; }));
    button.click();
    host.remove();
    document.body.append(host);
    acceptConfirmation(true);
    await settle();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('completes a retargeted POST against its submitted entity without unlocking the newer run', async () => {
    const { host, button, status, morphList } = mount();
    let reply!: (value: any) => void;
    let replyNew!: (value: any) => void;
    fetchMock.mockReturnValueOnce(new Promise((resolve) => { reply = resolve; }));
    fetchMock.mockReturnValueOnce(new Promise((resolve) => { replyNew = resolve; }));
    button.click();
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).entity_ids).toEqual([42]);
    morphList(43);
    await settle();
    expect(button.hasAttribute('aria-disabled')).toBe(false);
    button.click();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(JSON.parse(fetchMock.mock.calls[1][1].body).entity_ids).toEqual([43]);

    reply(answer(200, { success: true, message: 'Upscaled' }));
    await settle();
    expect(events.at(-1)?.detail).toEqual({ entityType: 'resource', entityId: 42, source: null });
    expect(status.textContent).toBe('Running…');
    expect(announce).toHaveBeenLastCalledWith('Upscale image: Upscaled', { assertive: false });
    expect(button.getAttribute('aria-disabled')).toBe('true');
    button.click();
    expect(fetchMock).toHaveBeenCalledTimes(2);

    replyNew(answer(200, { success: true, message: 'Also upscaled' }));
    await settle();
    expect(events.at(-1)?.detail).toEqual({ entityType: 'resource', entityId: 43, source: host });
    expect(status.textContent).toBe('Also upscaled');
    expect(button.hasAttribute('aria-disabled')).toBe(false);
  });

  it('reports a synchronous success and signals autorefresh: saving, then updated, with no meta', async () => {
    const { host, button, status } = mount();
    let release: (v: any) => void = () => {};
    fetchMock.mockReturnValue(new Promise((r) => { release = r; }));
    button.click();
    await settle();
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(status.textContent).toBe('Running…');
    expect(announce).toHaveBeenCalledWith('Upscale image: running', { assertive: false });

    release(answer(200, { success: true, message: 'Upscaled' }));
    await settle();
    expect(status.textContent).toBe('Upscaled');
    expect(announce).toHaveBeenLastCalledWith('Upscale image: Upscaled', { assertive: false });
    expect(events.map((e) => e.type)).toEqual(['meta-shortcode-saving', 'meta-shortcode-updated']);
    expect(events[0].detail).toEqual({ source: host });
    expect(events[1].detail).toEqual({ entityType: 'resource', entityId: 42, source: host });
    expect('meta' in events[1].detail).toBe(false);
    expect(button.hasAttribute('aria-disabled')).toBe(false);
  });

  it('reports the message and refreshes after HTTP 200 success with a legacy job_id', async () => {
    vi.useFakeTimers();
    const { host, button, status } = mount();
    fetchMock.mockResolvedValueOnce(answer(200, { success: true, message: 'Marked reviewed', job_id: 'legacy-1' }));
    button.click();
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(status.textContent).toBe('Marked reviewed');
    expect(announce).toHaveBeenLastCalledWith('Upscale image: Marked reviewed', { assertive: false });
    expect(events).toEqual([
      { type: 'meta-shortcode-saving', detail: { source: host } },
      { type: 'meta-shortcode-updated', detail: { entityType: 'resource', entityId: 42, source: host } },
    ]);
    expect(button.hasAttribute('aria-disabled')).toBe(false);
  });

  it('says Done when a successful run has no message', async () => {
    const { button, status } = mount();
    fetchMock.mockResolvedValue(answer(200, { success: true }));
    button.click();
    await settle();
    expect(status.textContent).toBe('Done');
  });

  it.each([
    ['a refusal', answer(200, { success: false, message: 'Quota exhausted' }), 'Quota exhausted'],
    ['a refusal with no message', answer(200, { success: false }), 'Action failed'],
    ['validation errors', answer(400, { errors: [{ field: 'scale', message: 'Scale must be at most 4' }, { field: 'x' }] }), 'Scale must be at most 4; x'],
    ['an error', answer(403, { error: 'this plugin is not available to group-limited accounts' }), 'this plugin is not available to group-limited accounts'],
    ['an unreadable answer', { ok: false, status: 500, json: async () => { throw new Error('not json'); } }, 'Action failed'],
  ])('reports %s assertively and signals nothing', async (_name, response, message) => {
    const { button, status } = mount();
    fetchMock.mockResolvedValue(response);
    button.click();
    await settle();
    expect(status.textContent).toBe(message);
    expect(announce).toHaveBeenLastCalledWith(`Upscale image: ${message}`, { assertive: true });
    expect(events).toEqual([]);
    expect(button.hasAttribute('aria-disabled')).toBe(false);
  });

  it('reports a synchronous refusal with job handles without polling or refreshing', async () => {
    vi.useFakeTimers();
    const { button, status } = mount();
    fetchMock.mockResolvedValueOnce(answer(200, {
      success: false, message: 'A job already exists', job_id: 'legacy-1', canonicalJobId: 'job-1',
    }));
    button.click();
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(status.textContent).toBe('A job already exists');
    expect(announce).toHaveBeenLastCalledWith('Upscale image: A job already exists', { assertive: true });
    expect(events).toEqual([]);
  });

  it('reports a network failure', async () => {
    const { button, status } = mount();
    fetchMock.mockRejectedValue(new TypeError('offline'));
    button.click();
    await settle();
    expect(status.textContent).toBe('Action failed');
    expect(events).toEqual([]);
  });

  it('follows a redirect', async () => {
    const original = Object.getOwnPropertyDescriptor(window, 'location');
    const location = { href: '/resource?id=42' };
    Object.defineProperty(window, 'location', { value: location, configurable: true });
    try {
      const { button } = mount();
      fetchMock.mockResolvedValue(answer(200, { success: true, redirect: '/note?id=7' }));
      button.click();
      await settle();
      expect(location.href).toBe('/note?id=7');
      expect(events).toEqual([]);
    } finally {
      if (original) Object.defineProperty(window, 'location', original);
    }
  });

  it('ignores clicks while a run is in flight', async () => {
    const { button } = mount();
    let release: (v: any) => void = () => {};
    fetchMock.mockReturnValue(new Promise((r) => { release = r; }));
    button.click();
    button.click();
    await settle();
    button.click();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    release(answer(200, { success: true }));
    await settle();
    fetchMock.mockResolvedValue(answer(200, { success: true }));
    button.click();
    await settle();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  describe('async runs', () => {
    const started = answer(202, { job_id: 'legacy-1', canonicalJobId: 'job-1' });

    async function startAsync() {
      vi.useFakeTimers();
      const parts = mount({ async: true });
      fetchMock.mockResolvedValueOnce(started);
      parts.button.click();
      await vi.advanceTimersByTimeAsync(0);
      return parts;
    }

    it('polls the Job until it succeeds, then signals', async () => {
      const { button, status } = await startAsync();
      expect(status.textContent).toBe('Started');
      expect(button.getAttribute('aria-disabled')).toBe('true');
      fetchMock.mockResolvedValueOnce(answer(200, { id: 'job-1', state: 'running' }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(fetchMock.mock.calls[1][0]).toBe('/v1/jobs/job-1');
      expect(events).toEqual([]);

      fetchMock.mockResolvedValueOnce(answer(200, { id: 'job-1', state: 'succeeded' }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(status.textContent).toBe('Succeeded');
      expect(announce).toHaveBeenLastCalledWith('Upscale image: Succeeded', { assertive: false });
      expect(events.map((e) => e.type)).toEqual(['meta-shortcode-saving', 'meta-shortcode-updated']);
      expect(button.hasAttribute('aria-disabled')).toBe(false);

      await vi.advanceTimersByTimeAsync(POLL_MS * 3);
      expect(fetchMock).toHaveBeenCalledTimes(3);
    });

    it('keeps the same action busy through a list morph', async () => {
      const { host, button, status, morphList } = await startAsync();
      morphList(42);
      await vi.advanceTimersByTimeAsync(0);
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(status.textContent).toBe('Started');
      button.click();
      expect(fetchMock).toHaveBeenCalledTimes(1);

      fetchMock.mockResolvedValueOnce(answer(200, { id: 'job-1', state: 'succeeded' }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(fetchMock.mock.calls[1][0]).toBe('/v1/jobs/job-1');
      expect(button.hasAttribute('aria-disabled')).toBe(false);
      expect(status.textContent).toBe('Succeeded');
      expect(events.at(-1)?.detail).toEqual({ entityType: 'resource', entityId: 42, source: host });
    });

    it('releases a retargeted action during polling without cancelling its Job', async () => {
      const { host, button, status, morphList } = await startAsync();
      let replyOldPoll!: (value: any) => void;
      fetchMock.mockReturnValueOnce(new Promise((resolve) => { replyOldPoll = resolve; }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      const oldSignal = fetchMock.mock.calls[1][1].signal;
      expect(oldSignal.aborted).toBe(false);

      morphList(43);
      await vi.advanceTimersByTimeAsync(0);
      expect(oldSignal.aborted).toBe(true);
      expect(button.hasAttribute('aria-disabled')).toBe(false);
      expect(status.textContent).toBe('');
      fetchMock.mockResolvedValueOnce(answer(202, { canonicalJobId: 'job-2' }));
      button.click();
      await vi.advanceTimersByTimeAsync(0);
      expect(fetchMock).toHaveBeenCalledTimes(3); // No server request to cancel job-1.
      expect(JSON.parse(fetchMock.mock.calls[2][1].body).entity_ids).toEqual([43]);
      expect(status.textContent).toBe('Started');

      replyOldPoll(answer(200, { id: 'job-1', state: 'succeeded' }));
      await vi.advanceTimersByTimeAsync(0);
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(events).toEqual([]);
      expect(status.textContent).toBe('Started');

      fetchMock.mockResolvedValueOnce(answer(200, { id: 'job-2', state: 'succeeded' }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(fetchMock.mock.calls[3][0]).toBe('/v1/jobs/job-2');
      expect(button.hasAttribute('aria-disabled')).toBe(false);
      expect(events.at(-1)?.detail).toEqual({ entityType: 'resource', entityId: 43, source: host });
    });

    it('reads a partial success as one', async () => {
      const { status } = await startAsync();
      fetchMock.mockResolvedValueOnce(answer(200, { id: 'job-1', state: 'succeeded', phase: 'partial' }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(status.textContent).toBe('Partially completed');
      expect(events).toHaveLength(2);
    });

    it.each(['failed', 'cancelled', 'interrupted'])('reports a %s Job assertively and signals nothing', async (state) => {
      const { status } = await startAsync();
      fetchMock.mockResolvedValueOnce(answer(200, { id: 'job-1', state }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(status.textContent).toBe(state[0].toUpperCase() + state.slice(1));
      expect(announce).toHaveBeenLastCalledWith(`Upscale image: ${status.textContent}`, { assertive: true });
      expect(events).toEqual([]);
    });

    it('reports a Job it cannot read', async () => {
      const { status } = await startAsync();
      fetchMock.mockResolvedValueOnce(answer(404, { error: 'not found' }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      expect(status.textContent).toBe("Could not read the job's progress");
      expect(events).toEqual([]);
    });

    it('stops polling when the button leaves the page, and drops an answer in flight', async () => {
      const { host } = await startAsync();
      let release: (v: any) => void = () => {};
      fetchMock.mockReturnValueOnce(new Promise((r) => { release = r; }));
      await vi.advanceTimersByTimeAsync(POLL_MS);
      const signal = fetchMock.mock.calls[1][1].signal;
      host.remove();
      expect(signal?.aborted).toBe(true);
      release(answer(200, { id: 'job-1', state: 'succeeded' }));
      await vi.advanceTimersByTimeAsync(POLL_MS * 5);
      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(events).toEqual([]);
    });

    it('never starts polling if the host was removed before the POST returned', async () => {
      vi.useFakeTimers();
      const { host, button, status } = mount({ async: true });
      let reply!: (value: any) => void;
      fetchMock.mockReturnValueOnce(new Promise((resolve) => { reply = resolve; }));
      button.click();
      host.remove();
      document.body.append(host); // Being connected again is not the same attachment.
      reply(started);
      await vi.advanceTimersByTimeAsync(POLL_MS * 2);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(announce).not.toHaveBeenCalledWith('Upscale image: started', expect.anything());
      expect(status.textContent).toBe('');
      expect(button.hasAttribute('aria-disabled')).toBe(false);
    });

    it('enforces a five-minute deadline even while a Job response body is pending', async () => {
      const { status, button } = await startAsync();
      let reply!: (value: any) => void;
      fetchMock.mockResolvedValueOnce({ ok: true, json: () => new Promise((resolve) => { reply = resolve; }) });
      await vi.advanceTimersByTimeAsync(FIVE_MINUTES - 1);
      expect(button.getAttribute('aria-disabled')).toBe('true');
      const signal = fetchMock.mock.calls[1][1].signal;
      await vi.advanceTimersByTimeAsync(1);
      expect(status.textContent).toBe('Still running. Follow it in the Jobs panel.');
      expect(signal?.aborted).toBe(true);
      expect(button.hasAttribute('aria-disabled')).toBe(false);
      reply({ id: 'job-1', state: 'succeeded' });
      await vi.advanceTimersByTimeAsync(POLL_MS * 10);
      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(events).toEqual([]);
    });

    it('does not poll without a canonical Job id', async () => {
      vi.useFakeTimers();
      const { button, status } = mount({ async: true });
      fetchMock.mockResolvedValueOnce(answer(202, { job_id: 'legacy-1' }));
      button.click();
      await vi.advanceTimersByTimeAsync(POLL_MS * 3);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(status.textContent).toBe('Started. Follow it in the Jobs panel.');
      expect(button.hasAttribute('aria-disabled')).toBe(false);
    });
  });
});
