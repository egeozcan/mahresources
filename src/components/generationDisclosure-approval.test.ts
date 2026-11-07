// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { registerGenerationDisclosureStore, reviewDisclosure, cancelReview } from './generationDisclosure.js';

function makeStore() {
  let state: any;
  registerGenerationDisclosureStore({ store: (_name: string, value: any) => { state = value; } });
  return state;
}

const harness = (hash: string, mrql = false, plugins = false) => ({
  mode: 'harness',
  groundingHash: hash,
  optIns: { previewMRQL: mrql, previewPlugins: plugins },
  destination: { host: 'api.example' },
  sections: [],
});

// A re-disclosure whose answer the test releases by hand, so responses can arrive in any order.
function controllable() {
  const calls: { requested: { previewMRQL: boolean; previewPlugins: boolean }; signal: AbortSignal; resolve: (d: unknown) => void; reject: (e: unknown) => void }[] = [];
  const redisclose = vi.fn((requested, signal) => new Promise((resolve, reject) => { calls.push({ requested, signal, resolve, reject }); }));
  return { calls, redisclose };
}

afterEach(() => { vi.unstubAllGlobals(); document.body.innerHTML = ''; });

describe('approval tuple', () => {
  it('Send resolves an immutable tuple of the shown hash and the opt-ins', async () => {
    const store = makeStore();
    const pending = store.open(harness('h0'), [], { owner: {}, redisclose: vi.fn() });
    store.send();
    const approval = await pending;
    expect(approval).toEqual({ groundingHash: 'h0', previewMRQL: false, previewPlugins: false });
    expect(Object.isFrozen(approval)).toBe(true);
  });

  it('Cancel resolves null, not false', async () => {
    const store = makeStore();
    const pending = store.open(harness('h0'), [], { owner: {} });
    store.dismiss();
    expect(await pending).toBeNull();
  });

  it('a classic disclosure approves with both opt-ins off', async () => {
    const store = makeStore();
    const pending = store.open({ groundingHash: 'c', destination: {}, sections: [] }, []);
    store.send();
    expect(await pending).toEqual({ groundingHash: 'c', previewMRQL: false, previewPlugins: false });
  });

  it('toggling an opt-in re-requests the dry run, disables Send meanwhile, and approves the new hash', async () => {
    const store = makeStore();
    const { calls, redisclose } = controllable();
    const pending = store.open(harness('h0'), [], { owner: {}, redisclose });
    store.setOptIn('previewMRQL', true);
    expect(redisclose).toHaveBeenCalledTimes(1);
    expect(calls[0].requested).toEqual({ previewMRQL: true, previewPlugins: false });
    expect(store.canSend).toBe(false);
    store.send(); // refused while the answer is in flight
    expect(store.isOpen).toBe(true);
    calls[0].resolve(harness('h1', true, false));
    await vi.waitFor(() => expect(store.canSend).toBe(true));
    expect(store.disclosure.groundingHash).toBe('h1');
    store.send();
    expect(await pending).toEqual({ groundingHash: 'h1', previewMRQL: true, previewPlugins: false });
  });

  it('ignores a response older than the latest toggle, whatever order they arrive in', async () => {
    const store = makeStore();
    const { calls, redisclose } = controllable();
    const pending = store.open(harness('h0'), [], { owner: {}, redisclose });
    store.setOptIn('previewMRQL', true);      // request 0: mrql on
    store.setOptIn('previewPlugins', true);   // request 1: both on
    expect(calls[0].signal.aborted).toBe(true);
    expect(calls[1].requested).toEqual({ previewMRQL: true, previewPlugins: true });
    // The newest answers first, then the stale one arrives and must change nothing.
    calls[1].resolve(harness('h-both', true, true));
    await vi.waitFor(() => expect(store.canSend).toBe(true));
    calls[0].resolve(harness('h-mrql', true, false));
    await new Promise((r) => setTimeout(r, 0));
    expect(store.disclosure.groundingHash).toBe('h-both');
    store.send();
    expect(await pending).toEqual({ groundingHash: 'h-both', previewMRQL: true, previewPlugins: true });
  });

  it('a stale response that arrives first never lets Send through for the wrong opt-ins', async () => {
    const store = makeStore();
    const { calls, redisclose } = controllable();
    store.open(harness('h0'), [], { owner: {}, redisclose });
    store.setOptIn('previewMRQL', true);
    store.setOptIn('previewMRQL', false); // back to the approved state, a new request
    calls[0].resolve(harness('h-mrql', true, false)); // stale
    await new Promise((r) => setTimeout(r, 0));
    expect(store.canSend).toBe(false);
    expect(store.disclosure.groundingHash).toBe('h0');
  });

  it('keeps Send disabled after a failed re-disclosure until a later one succeeds', async () => {
    const store = makeStore();
    const { calls, redisclose } = controllable();
    const pending = store.open(harness('h0'), [], { owner: {}, redisclose });
    store.setOptIn('previewPlugins', true);
    calls[0].reject(new Error('boom'));
    await vi.waitFor(() => expect(store.redisclosureError).not.toBe(''));
    expect(store.canSend).toBe(false);
    store.send();
    expect(store.isOpen).toBe(true);
    store.setOptIn('previewPlugins', false);
    expect(store.redisclosureError).toBe('');
    calls[1].resolve(harness('h0', false, false));
    await vi.waitFor(() => expect(store.canSend).toBe(true));
    store.send();
    expect((await pending).previewPlugins).toBe(false);
  });

  it('treats an answer without a hash, or for other opt-ins than asked, as a failure', async () => {
    const store = makeStore();
    const { calls, redisclose } = controllable();
    store.open(harness('h0'), [], { owner: {}, redisclose });
    store.setOptIn('previewMRQL', true);
    calls[0].resolve({ groundingHash: '' });
    await vi.waitFor(() => expect(store.redisclosureError).not.toBe(''));
    expect(store.canSend).toBe(false);
    store.setOptIn('previewMRQL', true);
    calls[1].resolve(harness('wrong', false, false));
    await vi.waitFor(() => expect(calls[1]).toBeTruthy());
    await new Promise((r) => setTimeout(r, 0));
    expect(store.canSend).toBe(false);
    expect(store.redisclosureError).not.toBe('');
  });
});

describe('dialog ownership', () => {
  it('cancel(token) settles that review as null, aborts its re-disclosure and releases the modal', async () => {
    const store = makeStore();
    const owner = {};
    const { calls, redisclose } = controllable();
    document.body.innerHTML = '<div id="page"></div><div id="dlg"></div>';
    const pending = store.open(harness('h0'), [], { owner, redisclose });
    store._applyInert(document.getElementById('dlg'));
    expect((document.getElementById('page') as any).inert).toBe(true);
    store.setOptIn('previewMRQL', true);
    store.cancel(owner);
    expect(await pending).toBeNull();
    expect(store.isOpen).toBe(false);
    expect(calls[0].signal.aborted).toBe(true);
    expect((document.getElementById('page') as any).inert).toBe(false);
  });

  it('a different owner cannot cancel it', async () => {
    const store = makeStore();
    const mine = {};
    const pending = store.open(harness('h0'), [], { owner: mine });
    store.cancel({});
    store.cancel(undefined);
    expect(store.isOpen).toBe(true);
    store.cancel(mine);
    expect(await pending).toBeNull();
  });

  it('a late answer after the review was cancelled changes nothing', async () => {
    const store = makeStore();
    const owner = {};
    const { calls, redisclose } = controllable();
    store.open(harness('h0'), [], { owner, redisclose });
    store.setOptIn('previewMRQL', true);
    store.cancel(owner);
    const next = store.open(harness('n0'), [], { owner: {} });
    calls[0].resolve(harness('late', true, false));
    await new Promise((r) => setTimeout(r, 0));
    expect(store.disclosure.groundingHash).toBe('n0');
    store.send();
    expect((await next).groundingHash).toBe('n0');
  });

  it('reviewDisclosure and cancelReview go through the registered store', async () => {
    const store = makeStore();
    vi.stubGlobal('Alpine', { store: () => store });
    const owner = {};
    const pending = reviewDisclosure(harness('h0'), [], { owner });
    expect(store.isOpen).toBe(true);
    cancelReview(owner);
    expect(await pending).toBeNull();
  });

  it('fails closed without the store', async () => {
    vi.stubGlobal('Alpine', { store: () => undefined });
    vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(await reviewDisclosure(harness('h0'), [], { owner: {} })).toBeNull();
  });
});
