// A reload is slow enough that the page can change underneath it, and Alpine's
// morph is the thing most likely to do so. These tests cover what _activate owes
// the reader when that happens: a button that works again immediately, and no
// announcement, failure marker, or focus jump belonging to markup that is gone.
//
// reloadInto's own staleness handling is covered in deferredReload.test.ts; this
// file is about the layer above it, which decides what the reader is told.
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

let ReloadShortcode: any;
let announce: ReturnType<typeof vi.fn>;

function fakeSpan() {
  return { className: '', textContent: '', remove: vi.fn() };
}

beforeAll(async () => {
  vi.stubGlobal('HTMLElement', class {});
  vi.stubGlobal('Node', { TEXT_NODE: 3, ELEMENT_NODE: 1 });
  const defined: Record<string, any> = {};
  vi.stubGlobal('customElements', {
    get: () => undefined,
    define: (name: string, cls: any) => void (defined[name] = cls),
  });
  vi.stubGlobal('window', {});
  vi.stubGlobal('document', { activeElement: null, body: {}, createElement: () => fakeSpan() });

  await import('./reloadShortcode.js');
  ReloadShortcode = defined['reload-shortcode'];
});

beforeEach(() => {
  announce = vi.fn();
  (globalThis as any).window.mahAnnounce = announce;
  (globalThis as any).document.activeElement = null;
});

function fakeButton() {
  const attrs: Record<string, string> = {};
  return {
    isConnected: true,
    focus: vi.fn(),
    closest: () => null,
    // A morph schedules a name check against whatever button is in place; give it
    // a plain text face so it settles without needing a name.
    childNodes: [{ nodeType: 3, textContent: 'Refresh' }],
    getAttribute: (k: string) => attrs[k] ?? null,
    hasAttribute: (k: string) => k in attrs,
    setAttribute: (k: string, v: string) => void (attrs[k] = v),
    removeAttribute: (k: string) => void delete attrs[k],
  };
}

// A host wired to a target whose reload can be settled by the test.
function makeHost() {
  const button = fakeButton();
  const container = { isConnected: true, querySelectorAll: () => [button] };

  let settle: { resolve: (v: any) => void; reject: (e: Error) => void };
  const run = () =>
    new Promise((resolve, reject) => {
      settle = { resolve, reject };
    });

  const host = Object.create(ReloadShortcode.prototype);
  host._init = true;
  host._busy = false;
  host._activationId = 0;
  host._resolveTarget = () => ({ container, run });
  host.appended = [] as any[];
  host.appendChild = (node: any) => host.appended.push(node);
  host.querySelector = (selector: string) => {
    if (selector.includes('reload-shortcode-error')) {
      return host.appended.find((n: any) => n.className === 'reload-shortcode-error') ?? null;
    }
    return button;
  };

  return { host, button, settle: () => settle };
}

describe('reload activation across a morph', () => {
  it('frees the button for a fresh click as soon as the markup is replaced', async () => {
    const { host, button } = makeHost();

    host._activate(button);
    expect(host._busy).toBe(true);

    // The morph installs a new face and a new region token; the request still in
    // flight belongs to markup that no longer exists. Staying busy until it
    // settles would leave the reader clicking a dead button.
    host.refreshFromMorph();

    expect(host._busy).toBe(false);
  });

  it('does not report a failure that belongs to replaced markup', async () => {
    const { host, button, settle } = makeHost();

    const activation = host._activate(button);
    host.refreshFromMorph();
    settle().reject(new Error('deferred render failed: 400'));
    await activation;

    expect(announce).not.toHaveBeenCalledWith('Could not reload the content', { assertive: true });
    // A visible marker would contradict the content the morph just rendered.
    expect(host.appended).toHaveLength(0);
  });

  it('does not announce success for content nobody is looking at', async () => {
    const { host, button, settle } = makeHost();

    const activation = host._activate(button);
    host.refreshFromMorph();
    settle().resolve(true);
    await activation;

    expect(announce).not.toHaveBeenCalledWith('Content reloaded', { assertive: false });
  });

  it('does not pull focus back into markup that was replaced', async () => {
    const { host, button, settle } = makeHost();
    (globalThis as any).document.activeElement = button;

    const activation = host._activate(button);
    host.refreshFromMorph();
    settle().resolve(true);
    await activation;

    expect(button.focus).not.toHaveBeenCalled();
  });

  it('still reports and restores focus for an activation nothing superseded', async () => {
    const { host, button, settle } = makeHost();
    (globalThis as any).document.activeElement = button;

    const activation = host._activate(button);
    settle().resolve(true);
    await activation;

    expect(announce).toHaveBeenCalledWith('Content reloaded', { assertive: false });
    expect(button.focus).toHaveBeenCalled();
    expect(host._busy).toBe(false);
  });

  it('still shows the failure marker for an activation nothing superseded', async () => {
    const { host, button, settle } = makeHost();

    const activation = host._activate(button);
    settle().reject(new Error('deferred render failed: 400'));
    await activation;

    expect(announce).toHaveBeenCalledWith('Could not reload the content', { assertive: true });
    expect(host.appended).toHaveLength(1);
    expect(host.appended[0].textContent).toBe('Reload failed.');
  });
});

// Focus parking, exercised through _activate because that is the only way in.
// A region wrapper is display:contents: it generates no box, so browsers do not
// reliably let it hold focus.
function parkable(canFocus: boolean, extra: Record<string, any> = {}) {
  const attrs: Record<string, string> = {};
  const el: any = {
    isConnected: true,
    children: [],
    parentElement: null,
    querySelectorAll: () => [],
    matches: () => false,
    getAttribute: (k: string) => attrs[k] ?? null,
    hasAttribute: (k: string) => k in attrs,
    setAttribute: (k: string, v: string) => void (attrs[k] = v),
    removeAttribute: (k: string) => void delete attrs[k],
    classList: { add: () => {}, remove: () => {}, contains: () => false },
    ...extra,
  };
  el.focus = vi.fn(() => {
    if (canFocus) (globalThis as any).document.activeElement = el;
  });
  return el;
}

describe('focus parking when the activated button is gone', () => {
  it('walks out to an ancestor when the region holds only text', async () => {
    // A conditional dropped the button from the refreshed content, and what came
    // back is bare text: no element child anywhere to land on.
    const ancestor = parkable(true);
    const region = parkable(false, { parentElement: ancestor });
    const button = fakeButton();

    let settle: any;
    const host = Object.create(ReloadShortcode.prototype);
    host._init = true;
    host._busy = false;
    host._activationId = 0;
    host.appended = [];
    host.appendChild = (n: any) => host.appended.push(n);
    host.querySelector = () => null;
    host._resolveTarget = () => ({
      container: region,
      run: () => new Promise((resolve) => { settle = resolve; }),
    });

    (globalThis as any).document.activeElement = button;
    const activation = host._activate(button);
    button.isConnected = false; // the swap removed it
    settle(true);
    await activation;

    // Leaving the reader on <body> is what the fallback exists to prevent.
    expect(region.focus).toHaveBeenCalled();
    expect(ancestor.focus).toHaveBeenCalled();
    expect((globalThis as any).document.activeElement).toBe(ancestor);
  });

  it('prefers an element child over an ancestor', async () => {
    const ancestor = parkable(true);
    const child = parkable(true);
    const region = parkable(false, { parentElement: ancestor, children: [child] });
    const button = fakeButton();

    let settle: any;
    const host = Object.create(ReloadShortcode.prototype);
    host._init = true;
    host._busy = false;
    host._activationId = 0;
    host.appended = [];
    host.appendChild = (n: any) => host.appended.push(n);
    host.querySelector = () => null;
    host._resolveTarget = () => ({
      container: region,
      run: () => new Promise((resolve) => { settle = resolve; }),
    });

    (globalThis as any).document.activeElement = button;
    const activation = host._activate(button);
    button.isConnected = false;
    settle(true);
    await activation;

    expect(child.focus).toHaveBeenCalled();
    expect(ancestor.focus).not.toHaveBeenCalled();
  });
});

