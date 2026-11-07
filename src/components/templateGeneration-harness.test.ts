// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { codeEditor } from './codeEditor.js';
import { templateGeneration } from './templateGeneration.js';
import { templateBundle } from './templateBundle.js';
import { registerGenerationDisclosureStore } from './generationDisclosure.js';

const enc = new TextEncoder();
const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;

const harnessDisclosure = (hash: string, mrql = false, plugins = false) => ({
  mode: 'harness', groundingHash: hash, optIns: { previewMRQL: mrql, previewPlugins: plugins },
  destination: { host: 'api.example' }, sections: [],
});

function stream() {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const body = new ReadableStream<Uint8Array>({ start(c) { controller = c; } });
  return {
    response: { ok: true, status: 200, headers: { get: () => 'text/event-stream' }, body } as unknown as Response,
    push: (t: string) => controller.enqueue(enc.encode(t)),
    close: () => controller.close(),
  };
}

type Call = { url: string; body: any; headers: Record<string, string>; signal?: AbortSignal };

function setup(kind: 'cluster' | 'editor' | 'bundle' = 'cluster', field = 'CustomMRQLResult') {
  const names = [field, `${field}CSS`];
  document.body.innerHTML = `<form>${names.map((n) => `<div x-data><input name="${n}" value="old-${n}"><div x-ref="editorContainer"></div></div>`).join('')}<input name="MetaSchema" value=""></form>`;
  const views = Array.from(document.querySelectorAll('form > div input')).map((input: any) => {
    const container = input.parentElement.querySelector('div');
    const view = {
      state: { doc: { toString: () => input.value, get length() { return input.value.length; } } },
      dispatch: ({ changes }: any) => { input.value = changes.insert; },
      destroy: () => {},
    };
    Object.assign(container, { _cmView: view });
    return view;
  });
  let storeState: any;
  registerGenerationDisclosureStore({ store: (_n: string, v: any) => { storeState = v; } });
  const stores: Record<string, any> = {
    confirmDialog: { ask: vi.fn(async () => true) },
    generationDisclosure: storeState,
    templatePreview: { generatePath: '/v1/category/generateTemplate' },
  };
  Object.assign(window, { Alpine: { store: (n: string) => stores[n] } });
  const host: any = kind === 'bundle'
    ? Object.assign(templateBundle({ carrier: 'category' }), { generationPrompt: 'a card', $refs: {}, $nextTick: () => Promise.resolve() })
    : Object.assign(kind === 'editor' ? codeEditor({ mode: 'html', generate: true }) : templateGeneration({ fieldName: field, mode: 'html' }), {
      $el: document.querySelector('form')!,
      view: views[0],
      $refs: { hiddenInput: document.querySelector('input'), editorContainer: document.querySelector('[x-ref]') },
      generationPrompt: 'Restyle',
      $nextTick: () => Promise.resolve(),
    });
  return { host, views, store: storeState };
}

// A fetch that answers dry runs from `dry`, and hands the real request to `real`.
function serve(dry: (body: any) => unknown, real: (call: Call) => Promise<unknown>) {
  const calls: Call[] = [];
  const fn = vi.fn(async (url: string, init: any) => {
    const call: Call = { url, body: JSON.parse(init.body), headers: init.headers, signal: init.signal };
    calls.push(call);
    if (call.body.dryRun) {
      const d = dry(call.body);
      return { ok: true, status: 200, json: async () => d };
    }
    return real(call);
  });
  vi.stubGlobal('fetch', fn);
  return calls;
}
const real = (calls: Call[]) => calls.filter((c) => !c.body.dryRun);

afterEach(() => { vi.unstubAllGlobals(); document.body.innerHTML = ''; });

describe('harness approval and final POST body', () => {
  it('sends the frozen body plus the approved hash and opt-ins, asking for a stream', async () => {
    const { host, store } = setup();
    const s = stream();
    const calls = serve((b) => harnessDisclosure(b.previewMRQL ? 'h-mrql' : 'h0', !!b.previewMRQL, !!b.previewPlugins), async () => s.response);
    const run = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.setOptIn('previewMRQL', true);
    await vi.waitFor(() => expect(store.canSend).toBe(true));
    // The re-disclosure repeated the dry run with the same body plus the opt-ins.
    const [first, again] = calls;
    expect(again.body).toEqual({ ...first.body, previewMRQL: true, previewPlugins: false });
    store.send();
    await vi.waitFor(() => expect(real(calls)).toHaveLength(1));
    const final = real(calls)[0];
    expect(final.body).toEqual({ ...first.body, dryRun: undefined, groundingHash: 'h-mrql', previewMRQL: true, previewPlugins: false });
    expect(final.body.dryRun).toBeUndefined();
    expect(final.headers.Accept).toBe('text/event-stream');
    s.push(frame('result', { target: 'cluster', slots: { CustomMRQLResult: 'a', CustomMRQLResultCSS: 'b' }, valid: true }));
    s.close();
    await run;
    expect(host.generatedFields.map((f: any) => f.text)).toEqual(['a', 'b']);
  });

  it('classic sends neither opt-in nor a stream Accept header', async () => {
    const { host, store } = setup();
    const calls = serve(() => ({ groundingHash: 'c', sections: [], destination: {} }), async () => ({ ok: true, status: 200, json: async () => ({ valid: true, slots: { CustomMRQLResult: 'a', CustomMRQLResultCSS: 'b' } }) }));
    const run = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.send();
    await run;
    const final = real(calls)[0];
    expect(Object.keys(final.body)).not.toContain('previewMRQL');
    expect(final.headers.Accept).toBeUndefined();
  });

  it('a failed re-disclosure leaves Send disabled and sends nothing', async () => {
    const { host, store } = setup();
    let n = 0;
    const calls = serve(() => { if (n++) throw new Error('x'); return harnessDisclosure('h0'); }, async () => stream().response);
    // dry() throwing makes fetch reject, as a network failure does.
    const run = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.setOptIn('previewPlugins', true);
    await vi.waitFor(() => expect(store.redisclosureError).not.toBe(''));
    store.send();
    expect(store.isOpen).toBe(true);
    store.dismiss();
    await run;
    expect(real(calls)).toHaveLength(0);
  });
});

describe('progress, Finish now and Cancel', () => {
  async function started() {
    const ctx = setup();
    const s = stream();
    const calls = serve(() => harnessDisclosure('h0'), async () => s.response);
    const run = ctx.host.generateFromPrompt();
    await vi.waitFor(() => expect(ctx.store.isOpen).toBe(true));
    ctx.store.send();
    await vi.waitFor(() => expect(real(calls)).toHaveLength(1));
    return { ...ctx, s, calls, run };
  }

  it('updates the panel from a turn frame while the terminal frame is withheld', async () => {
    const { host, s } = await started();
    s.push(frame('run', { runId: 'r1', maxTurns: 10, tokenBudget: 1000 }));
    s.push(frame('turn', { turn: 1, maxTurns: 10 }));
    await vi.waitFor(() => expect(host.run?.turn).toBe(1));
    expect(host.run.status).toBe('running');
    expect(host.generating).toBe(true);
    s.close();
  });

  it('Finish now posts the run id to the finish route once', async () => {
    const { host, s, calls } = await started();
    s.push(frame('run', { runId: 'r1', maxTurns: 10 }));
    await vi.waitFor(() => expect(host.run?.runId).toBe('r1'));
    await host.finishRunNow();
    await host.finishRunNow();
    const finish = calls.filter((c) => c.url.endsWith('/finish'));
    expect(finish).toHaveLength(1);
    expect(finish[0].url).toBe('/v1/category/generateTemplate/finish');
    expect(finish[0].body).toEqual({ runId: 'r1' });
    s.close();
  });

  it('Cancel aborts the request and leaves no draft', async () => {
    const { host, calls, run } = await started();
    host.cancelRun();
    await run;
    expect(real(calls)[0].signal!.aborted).toBe(true);
    expect(host.run.status).toBe('cancelled');
    expect(host.hasDraft()).toBe(false);
    expect(host.generating).toBe(false);
  });

  it('an error frame keeps the transcript and shows the message', async () => {
    const { host, s, run } = await started();
    s.push(frame('error', { error: 'provider down', transcript: [{ turn: 1, kind: 'seed', text: 't', sent: true }] }));
    s.close();
    await run;
    expect(host.generationError).toBe('provider down');
    expect(host.run.status).toBe('error');
    expect(host.run.transcript).toHaveLength(1);
  });

  it('the result keeps the transcript for the collapsed panel', async () => {
    const { host, s, run } = await started();
    s.push(frame('result', { target: 'cluster', valid: true, slots: { CustomMRQLResult: 'a', CustomMRQLResultCSS: 'b' }, stopReason: 'done', transcript: [{ turn: 1, kind: 'assistant', text: 'hi', sent: true }] }));
    s.close();
    await run;
    expect(host.run.status).toBe('done');
    expect(host.run.stopReason).toBe('done');
    expect(host.run.transcript[0].text).toBe('hi');
  });
});

describe('teardown and supersession', () => {
  it('stopGeneration aborts the run, cancels its own open review and invalidates late callbacks', async () => {
    const { host, store } = setup();
    const s = stream();
    serve(() => harnessDisclosure('h0'), async () => s.response);
    const run = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    host.stopGeneration();
    expect(store.isOpen).toBe(false);
    await run;
    expect(host.hasDraft()).toBe(false);
  });

  it('does not cancel a review another host opened', async () => {
    const { host, store } = setup();
    serve(() => harnessDisclosure('h0'), async () => stream().response);
    const other = store.open(harnessDisclosure('other'), [], { owner: {} });
    host.stopGeneration();
    expect(store.isOpen).toBe(true);
    store.dismiss();
    expect(await other).toBeNull();
  });

  it.each(['cluster', 'editor', 'bundle'] as const)('%s teardown aborts a streaming request', async (kind) => {
    const { host, store } = setup(kind);
    const s = stream();
    const calls = serve(() => harnessDisclosure('h0'), async () => s.response);
    const run = host.generateFromPrompt ? host.generateFromPrompt() : host.generateBundle();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.send();
    await vi.waitFor(() => expect(real(calls)).toHaveLength(1));
    host.destroy();
    expect(real(calls)[0].signal!.aborted).toBe(true);
    await run;
    expect(host.hasDraft?.() ?? host.pendingDraft).toBeFalsy();
  });

  it('a new run aborts the previous one', async () => {
    const { host, store } = setup();
    const s1 = stream();
    const calls = serve(() => harnessDisclosure('h0'), async () => s1.response);
    const first = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.send();
    await vi.waitFor(() => expect(real(calls)).toHaveLength(1));
    const second = host.generateFromPrompt();
    expect(real(calls)[0].signal!.aborted).toBe(true);
    await first;
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.dismiss();
    await second;
  });
});

describe('an explicitly submitted empty field', () => {
  const emptyResult = { target: 'slot', content: '', valid: true, submitted: ['CustomCSS'] };

  it('is a pending draft in a harness result that clears the editor only on Apply', async () => {
    const { host, store, views } = setup('editor', 'CustomCSS');
    host.mode = 'css';
    const s = stream();
    serve(() => harnessDisclosure('h0'), async () => s.response);
    // A standalone slot: only one editor field.
    document.body.innerHTML = '<form><div x-data><input name="CustomCSS" value="body{}"><div x-ref="editorContainer"></div></div></form>';
    const input = document.querySelector('input')!;
    const view = { state: { doc: { toString: () => input.value, get length() { return input.value.length; } } }, dispatch: ({ changes }: any) => { input.value = changes.insert; } };
    Object.assign(input.parentElement!.querySelector('div')!, { _cmView: view });
    Object.assign(host, { view, $el: document.querySelector('form'), $refs: { hiddenInput: input, editorContainer: input.parentElement!.querySelector('div') } });
    void views;
    const run = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.send();
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    s.push(frame('result', emptyResult));
    s.close();
    await run;
    expect(host.hasDraft()).toBe(true);
    expect(host.generatedFields[0].text).toBe('');
    expect(input.value).toBe('body{}');
    await host.applyGenerated();
    expect(input.value).toBe('');
  });

  it('classic still rejects empty standalone output', async () => {
    const { host, store } = setup('editor', 'CustomCSS');
    document.body.innerHTML = '<form><div x-data><input name="CustomCSS" value="body{}"><div x-ref="editorContainer"></div></div></form>';
    const input = document.querySelector('input')!;
    const view = { state: { doc: { toString: () => input.value, get length() { return input.value.length; } } }, dispatch: () => {} };
    Object.assign(input.parentElement!.querySelector('div')!, { _cmView: view });
    Object.assign(host, { view, $el: document.querySelector('form'), $refs: { hiddenInput: input, editorContainer: input.parentElement!.querySelector('div') } });
    serve(() => ({ groundingHash: 'c', sections: [], destination: {} }), async () => ({ ok: true, status: 200, json: async () => ({ target: 'slot', content: '', valid: true, submitted: ['CustomCSS'] }) }));
    const run = host.generateFromPrompt();
    await vi.waitFor(() => expect(store.isOpen).toBe(true));
    store.send();
    await run;
    expect(host.hasDraft()).toBe(false);
    expect(host.generationError).toBe('The model returned no content.');
  });
});
