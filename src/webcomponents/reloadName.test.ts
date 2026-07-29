// The reload button's accessible name is repaired on the client because only the
// real DOM knows what a screen reader can reach: the server sees an author's
// <span aria-hidden="true">↻</span> as text, an assistive technology sees nothing.
//
// The timing matters as much as the rule. Alpine's morph calls the updated() hook
// BEFORE it patches an element's children (@alpinejs/morph calls updated() and
// only then patchChildren()), so repairing synchronously would inspect the face
// that is about to be replaced.
import { beforeAll, describe, expect, it, vi } from 'vitest';

let ReloadShortcode: any;

beforeAll(async () => {
  vi.stubGlobal('HTMLElement', class {});
  vi.stubGlobal('Node', { TEXT_NODE: 3, ELEMENT_NODE: 1 });
  vi.stubGlobal('window', {});
  // The module registers itself on import; the stub captures the class so the
  // repair logic can be driven directly.
  const defined: Record<string, any> = {};
  vi.stubGlobal('customElements', {
    get: () => undefined,
    define: (name: string, cls: any) => void (defined[name] = cls),
  });

  await import('./reloadShortcode.js');
  ReloadShortcode = defined['reload-shortcode'];
});

function textNode(text: string) {
  return { nodeType: 3, textContent: text };
}

function elementNode(attrs: Record<string, string>, children: any[], style?: Record<string, string>) {
  return {
    nodeType: 1,
    childNodes: children,
    getAttribute: (k: string) => attrs[k] ?? null,
    hasAttribute: (k: string) => k in attrs,
    // Only present when a test cares about CSS: with no ownerDocument the naming
    // code cannot consult styles and treats the node as displayed, which is the
    // same position the non-browser test environment is in.
    ownerDocument: style
      ? { defaultView: { getComputedStyle: () => ({ display: 'block', visibility: 'visible', ...style }) } }
      : undefined,
  };
}

function fakeButton(children: any[]) {
  const attrs: Record<string, string> = {};
  return {
    childNodes: children,
    getAttribute: (k: string) => attrs[k] ?? null,
    hasAttribute: (k: string) => k in attrs,
    setAttribute: (k: string, v: string) => void (attrs[k] = v),
  };
}

function hostFor(button: any) {
  const el = Object.create(ReloadShortcode.prototype);
  el.querySelector = () => button;
  el.addEventListener = () => {};
  return el;
}

// Lets every queued microtask drain, which is where the deferred repair runs.
const flush = () => new Promise((resolve) => queueMicrotask(() => resolve(null)));

describe('reload button accessible name', () => {
  it('names a button whose whole face is hidden from assistive tech', async () => {
    const button = fakeButton([
      elementNode({ 'aria-hidden': 'true' }, [textNode('↻')]),
    ]);

    hostFor(button).connectedCallback();
    await flush();

    expect(button.getAttribute('aria-label')).toBe('Reload');
    expect(button.getAttribute('title')).toBe('Reload');
  });

  it('leaves a button that exposes its own text alone', async () => {
    const button = fakeButton([textNode('Refresh')]);

    hostFor(button).connectedCallback();
    await flush();

    // Overriding this would replace the visible name (WCAG 2.5.3 Label in Name).
    expect(button.getAttribute('aria-label')).toBeNull();
  });

  it('ignores subtrees hidden with the hidden attribute', async () => {
    const button = fakeButton([elementNode({ hidden: '' }, [textNode('Refresh')])]);

    hostFor(button).connectedCallback();
    await flush();

    expect(button.getAttribute('aria-label')).toBe('Reload');
  });

  it('ignores text CSS has taken off the page', async () => {
    // The server sees words here and leaves the button unnamed; a screen reader
    // reaches nothing at all. Only computed style can tell the two apart.
    const button = fakeButton([elementNode({}, [textNode('Refresh')], { display: 'none' })]);

    hostFor(button).connectedCallback();
    await flush();

    expect(button.getAttribute('aria-label')).toBe('Reload');
  });

  it('ignores text hidden with visibility:hidden', async () => {
    const button = fakeButton([elementNode({}, [textNode('Refresh')], { visibility: 'hidden' })]);

    hostFor(button).connectedCallback();
    await flush();

    expect(button.getAttribute('aria-label')).toBe('Reload');
  });

  it('keeps displayed text as the name when styles are readable', async () => {
    const button = fakeButton([elementNode({}, [textNode('Refresh')], { display: 'inline' })]);

    hostFor(button).connectedCallback();
    await flush();

    expect(button.getAttribute('aria-label')).toBeNull();
  });

  it('keeps a name the server already supplied', async () => {
    const button = fakeButton([elementNode({ 'aria-hidden': 'true' }, [textNode('↻')])]);
    button.setAttribute('aria-label', 'Refresh totals');

    hostFor(button).connectedCallback();
    await flush();

    expect(button.getAttribute('aria-label')).toBe('Refresh totals');
  });

  it('waits for morph to patch children before re-checking the name', async () => {
    // The face morph is about to install: hidden from assistive tech.
    const nextFace = elementNode({ 'aria-hidden': 'true' }, [textNode('↻')]);
    // The face that is still in the DOM when the hook fires: plain text.
    const button = fakeButton([textNode('Refresh')]);
    const host = hostFor(button);

    host.refreshFromMorph();

    // Reading now would see "Refresh" and wrongly conclude the button is named.
    expect(button.getAttribute('aria-label')).toBeNull();

    // Alpine patches children after the hook; the morph pass is synchronous.
    button.childNodes = [nextFace];
    await flush();

    expect(button.getAttribute('aria-label')).toBe('Reload');
  });
});
