// @vitest-environment happy-dom

// The [tabs] shortcode's client half: upgrading the server's stacked markup to
// the tabs pattern, the keyboard model, nesting, surviving a real Alpine morph,
// and the lazy panels it hides (LazyShortcode.suspended). Focus loss in the
// browser's own morph gap and native Enter/Space activation are only real in a
// browser; the E2E spec covers those.
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import Alpine from 'alpinejs';
import morph from '@alpinejs/morph';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';

// One fake IntersectionObserver for the shared observer deferredShortcodes.js
// creates on first use: tests decide when something intersects.
const observed = new Set<Element>();
let ioCallback: ((entries: any[], obs: any) => void) | null = null;
class FakeIntersectionObserver {
  constructor(cb: (entries: any[], obs: any) => void) {
    ioCallback = cb;
  }
  observe(el: Element) {
    observed.add(el);
  }
  unobserve(el: Element) {
    observed.delete(el);
  }
  disconnect() {}
}
function intersect(el: Element) {
  ioCallback!([{ target: el, isIntersecting: true }], { unobserve: (target: Element) => observed.delete(target) });
}

let reloadInto: (host: any, contentEl: any, token: string) => Promise<boolean>;

beforeAll(async () => {
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
  // happy-dom has no layout; native Tab order and CSS visibility are checked in E2E.
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(function (this: Element) {
    return (this.closest('[hidden]') ? [] : [new DOMRect(0, 0, 1, 1)]) as unknown as DOMRectList;
  });
  Alpine.plugin(morph);
  ({ reloadInto } = await import('./deferredShortcodes.js'));
  await import('./tabsShortcode.js');
});

afterAll(() => vi.restoreAllMocks());

let fetches: string[];
beforeEach(() => {
  fetches = [];
  observed.clear();
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, init: any) => {
      const { token } = JSON.parse(init.body);
      fetches.push(token);
      return new Response(JSON.stringify({ html: `<a href="#${token}">loaded ${token}</a>`, entity: null }), { status: 200 });
    }),
  );
});

afterEach(() => {
  document.body.innerHTML = '';
});

type Panel = string | { lazy: string };

// The markup RenderTabsShortcode emits (shortcodes/tabs_handler.go).
function serverTabs(panels: Record<string, Panel>, listLabel = 'Views') {
  const labels = Object.keys(panels);
  const buttons = labels
    .map((l, i) => `<button type="button" role="tab" class="tabs-tab" aria-selected="${i === 0}" tabindex="${i === 0 ? 0 : -1}">${l}</button>`)
    .join('');
  const bodies = labels
    .map((l) => {
      const p = panels[l];
      const body =
        typeof p === 'string'
          ? p
          : `<lazy-shortcode data-token="${p.lazy}"><noscript>This content requires JavaScript to load.</noscript></lazy-shortcode>`;
      return `<div class="tabs-panel" role="group" aria-label="${l}"><p class="tabs-fallback-label" aria-hidden="true">${l}</p>${body}</div>`;
    })
    .join('');
  return `<tabs-shortcode class="tabs-shortcode" data-tabs-key="${listLabel}:${labels.join(',')}"><div class="tabs-list" role="tablist" aria-label="${listLabel}" hidden>${buttons}</div>${bodies}</tabs-shortcode>`;
}

const THREE = { One: 'text only', Two: '<a href="/x">a link</a>', Three: { lazy: 'tok-3' } } as Record<string, Panel>;

// Parsed detached, then attached, as a browser inserts parsed markup: the host
// connects with its children already in place. (happy-dom's innerHTML on a
// connected element connects each node as it is built.)
function mount(html: string) {
  document.body.innerHTML = '';
  const main = document.createElement('main');
  main.innerHTML = html;
  document.body.append(main);
  return document.querySelector('tabs-shortcode') as HTMLElement;
}
const tabsOf = (host: Element) => Array.from(host.querySelectorAll(':scope > .tabs-list > [role="tab"]')) as HTMLButtonElement[];
const panelsOf = (host: Element) => Array.from(host.querySelectorAll(':scope > .tabs-panel')) as HTMLElement[];
const key = (el: Element, k: string) => el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }));
const microtask = () => Promise.resolve();
const settle = () => new Promise((r) => setTimeout(r, 0));
const morphTo = (html: string) =>
  Alpine.morph(document.querySelector('main')!, `<main>${html}</main>`, morphOptionsWithShortcodeElements());

describe('tabs-shortcode upgrade', () => {
  it('wires the tab pattern over the stacked fallback', () => {
    const host = mount(serverTabs(THREE));
    const tabs = tabsOf(host);
    const panels = panelsOf(host);

    expect(host.querySelector('.tabs-list')!.hasAttribute('hidden')).toBe(false);
    tabs.forEach((tab, i) => {
      expect(tab.id).toMatch(/^tabs-sc-\d+-tab-\d$/);
      expect(tab.getAttribute('aria-controls')).toBe(panels[i].id);
      expect(panels[i].getAttribute('role')).toBe('tabpanel');
      expect(panels[i].getAttribute('aria-labelledby')).toBe(tab.id);
      expect(panels[i].hasAttribute('aria-label')).toBe(false);
      expect((panels[i].querySelector('.tabs-fallback-label') as HTMLElement).hidden).toBe(true);
    });
    expect(tabs.map((t) => t.getAttribute('aria-selected'))).toEqual(['true', 'false', 'false']);
    expect(panels.map((p) => p.hidden)).toEqual([false, true, true]);
    // Text alone gives the reader nothing to Tab to, so the panel takes a stop.
    expect(panels[0].getAttribute('tabindex')).toBe('0');

    tabs[1].click();
    expect(panels.map((p) => p.hidden)).toEqual([true, false, true]);
    expect(panels[1].hasAttribute('tabindex')).toBe(false);
    expect(panels[0].hasAttribute('tabindex')).toBe(false);
  });

  it('leaves the stacked layout when tabs and panels do not pair up', () => {
    const html = serverTabs(THREE).replace(/<div class="tabs-panel"[^]*?<\/div>(?=<div class="tabs-panel")/, '');
    const host = mount(html);
    expect(host.querySelector('.tabs-list')!.hasAttribute('hidden')).toBe(true);
    expect(panelsOf(host).every((p) => p.getAttribute('role') === 'group' && !p.hidden)).toBe(true);
  });
});

describe('tabs-shortcode keyboard', () => {
  it('moves focus with arrows, Home and End, wrapping, without selecting', () => {
    const host = mount(serverTabs(THREE));
    const tabs = tabsOf(host);
    tabs[0].focus();

    key(tabs[0], 'ArrowRight');
    expect(document.activeElement).toBe(tabs[1]);
    key(tabs[1], 'End');
    expect(document.activeElement).toBe(tabs[2]);
    key(tabs[2], 'ArrowRight');
    expect(document.activeElement).toBe(tabs[0]);
    key(tabs[0], 'ArrowLeft');
    expect(document.activeElement).toBe(tabs[2]);
    key(tabs[2], 'Home');
    expect(document.activeElement).toBe(tabs[0]);
    key(tabs[0], 'ArrowRight');

    expect(tabs.map((t) => t.getAttribute('aria-selected'))).toEqual(['true', 'false', 'false']);
    expect(tabs.map((t) => t.getAttribute('tabindex'))).toEqual(['-1', '0', '-1']);

    // Focus leaving the list hands the one tab stop back to the selected tab.
    tabs[1].dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: document.body }));
    expect(tabs.map((t) => t.getAttribute('tabindex'))).toEqual(['0', '-1', '-1']);

    // Moving within the list keeps it on the focused one.
    tabs[0].focus();
    key(tabs[0], 'ArrowRight');
    tabs[1].dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: tabs[2] }));
    expect(tabs[1].getAttribute('tabindex')).toBe('0');
  });

  it('selects on activation', () => {
    const host = mount(serverTabs(THREE));
    const tabs = tabsOf(host);
    tabs[1].focus();
    tabs[1].click();
    expect(tabs.map((t) => t.getAttribute('aria-selected'))).toEqual(['false', 'true', 'false']);
    expect(tabs.map((t) => t.getAttribute('tabindex'))).toEqual(['-1', '0', '-1']);
  });
});

describe('nested tab sets', () => {
  it('each set reacts only to its own tabs', () => {
    const inner = serverTabs({ In1: 'i1', In2: 'i2' }, 'Inner');
    const outerHost = mount(serverTabs({ Outer1: inner, Outer2: 'o2' }, 'Outer'));
    const innerHost = outerHost.querySelector('tabs-shortcode tabs-shortcode') as HTMLElement;
    const outerTabs = tabsOf(outerHost);
    const innerTabs = tabsOf(innerHost);

    // The outer set does not hide the inner set's fallback labels, the inner
    // set does; the outer panel holds tab buttons, so it needs no stop.
    expect(panelsOf(outerHost)[0].hasAttribute('tabindex')).toBe(false);
    expect(panelsOf(innerHost).every((p) => (p.querySelector('.tabs-fallback-label') as HTMLElement).hidden)).toBe(true);

    innerTabs[0].focus();
    key(innerTabs[0], 'ArrowRight');
    expect(document.activeElement).toBe(innerTabs[1]);
    expect(outerTabs.map((t) => t.getAttribute('tabindex'))).toEqual(['0', '-1']);

    innerTabs[1].click();
    expect(panelsOf(innerHost).map((p) => p.hidden)).toEqual([true, false]);
    expect(outerTabs.map((t) => t.getAttribute('aria-selected'))).toEqual(['true', 'false']);
    expect(panelsOf(outerHost).map((p) => p.hidden)).toEqual([false, true]);

    innerTabs[1].dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: document.body }));
    expect(outerTabs.map((t) => t.getAttribute('tabindex'))).toEqual(['0', '-1']);
  });
});

describe('tabs-shortcode across a morph', () => {
  it('keeps selection, IDs and focus through a real Alpine morph', async () => {
    const html = serverTabs({ One: 'a', Two: 'b', Three: 'c' });
    const host = mount(html);
    const tabs = tabsOf(host);
    tabs[2].click();
    tabs[2].focus();
    const ids = [...tabs, ...panelsOf(host)].map((el) => [el.id, el.getAttribute('aria-controls'), el.getAttribute('aria-labelledby')]);

    morphTo(html);
    // The morph put the server's markup back; the microtask has not run yet.
    expect(host.querySelector('.tabs-list')!.hasAttribute('hidden')).toBe(true);
    expect(tabs[2].getAttribute('aria-selected')).toBe('false');

    await microtask();
    expect(document.querySelector('tabs-shortcode')).toBe(host);
    expect(tabs.map((t) => t.getAttribute('aria-selected'))).toEqual(['false', 'false', 'true']);
    expect(panelsOf(host).map((p) => p.hidden)).toEqual([true, true, false]);
    expect(host.querySelector('.tabs-list')!.hasAttribute('hidden')).toBe(false);
    expect(panelsOf(host).every((p) => (p.querySelector('.tabs-fallback-label') as HTMLElement).hidden)).toBe(true);
    expect([...tabs, ...panelsOf(host)].map((el) => [el.id, el.getAttribute('aria-controls'), el.getAttribute('aria-labelledby')])).toEqual(ids);
    expect(document.activeElement).toBe(tabs[2]);
    expect(tabs[2].getAttribute('tabindex')).toBe('0');
  });

  it('gives focus back to the tab when the morph gap lost it', async () => {
    const html = serverTabs({ One: 'a', Two: 'b' });
    const host = mount(html);
    const tabs = tabsOf(host);
    tabs[1].focus();
    // What a browser's focus fixup does when the list is hidden mid-morph.
    Alpine.morph(
      document.querySelector('main')!,
      `<main>${html}</main>`,
      morphOptionsWithShortcodeElements({
        updated(el: Element) {
          if (el.classList?.contains('tabs-list')) (document.activeElement as HTMLElement)?.blur();
        },
      }),
    );
    expect(document.activeElement).not.toBe(tabs[1]);
    await microtask();
    expect(document.activeElement).toBe(tabs[1]);
    expect(tabs.map((t) => t.getAttribute('tabindex'))).toEqual(['-1', '0']);
  });

  it('matches a surviving tab set by identity when a page morph moves it', async () => {
    const optional = serverTabs({ One: 'a', Two: 'b' }, 'Optional');
    const persistent = serverTabs({ One: 'c', Two: 'd' }, 'Persistent');
    mount(optional + persistent);
    const before = document.querySelectorAll('tabs-shortcode')[1];
    tabsOf(before)[1].click();
    const selectedId = tabsOf(before)[1].id;

    morphTo(persistent);
    await microtask();

    const after = document.querySelector('tabs-shortcode')!;
    expect(tabsOf(after)[1].getAttribute('aria-selected')).toBe('true');
    expect(tabsOf(after)[1].id).toBe(selectedId);
  });

  it('discards ambiguous state in a second morph in the same turn', async () => {
    const left = serverTabs({ One: 'a', Two: 'b' }, 'Left');
    const right = serverTabs({ One: 'c', Two: 'd' }, 'Right');
    mount(`<div id="left">${left}</div><div id="right">${right}${right}</div>`);
    const rightRegion = document.getElementById('right')!;
    tabsOf(rightRegion.querySelector('tabs-shortcode')!)[1].click();

    Alpine.morph(document.getElementById('left')!, `<div id="left">${left}</div>`, morphOptionsWithShortcodeElements());
    Alpine.morph(rightRegion, `<div id="right">${right}</div>`, morphOptionsWithShortcodeElements());
    await microtask();

    const [first, second] = tabsOf(rightRegion.querySelector('tabs-shortcode')!);
    expect(first.getAttribute('aria-selected')).toBe('true');
    expect(second.getAttribute('aria-selected')).toBe('false');
  });

  it('starts a changed tab set at its default instead of inheriting the old selection', async () => {
    const host = mount(serverTabs({ One: 'a', Two: 'b', Three: 'c' }));
    tabsOf(host)[2].click();
    morphTo(serverTabs({ One: 'a', Two: 'b' }));
    await microtask();
    expect(tabsOf(host).map((t) => t.getAttribute('aria-selected'))).toEqual(['true', 'false']);
    expect(panelsOf(host).map((p) => p.hidden)).toEqual([false, true]);
  });
});

describe('lazy panels', () => {
  it('a hidden lazy panel waits until it is shown and intersects', async () => {
    const host = mount(serverTabs(THREE));
    const lazy = host.querySelector('lazy-shortcode') as any;
    expect(lazy.suspended).toBe(true);
    expect(observed.has(lazy)).toBe(true);
    await settle();
    expect(fetches).toEqual([]);

    tabsOf(host)[2].click();
    expect(lazy.suspended).toBe(false);
    intersect(lazy);
    await settle();
    expect(fetches).toEqual(['tok-3']);
    expect(lazy.textContent).toContain('loaded tok-3');
  });

  it('drops the panel tab stop once the loaded content brings a link', async () => {
    const host = mount(serverTabs(THREE));
    tabsOf(host)[2].click();
    const panel = panelsOf(host)[2];
    expect(panel.getAttribute('tabindex')).toBe('0');
    intersect(host.querySelector('lazy-shortcode')!);
    await settle();
    await settle();
    expect(panel.querySelector('a')).not.toBeNull();
    expect(panel.hasAttribute('tabindex')).toBe(false);
  });

  // The server seals a fresh token on every render, so a real morph changes
  // data-token (attributeChangedCallback) as well as calling refreshFromMorph.
  it.each([
    ['the same token', 'tok-3'],
    ['a freshly sealed token', 'tok-3b'],
  ])('a morph with %s re-arms a visited lazy panel that is hidden instead of re-fetching it', async (_, next) => {
    const host = mount(serverTabs(THREE));
    const lazy = host.querySelector('lazy-shortcode') as any;
    tabsOf(host)[2].click();
    intersect(lazy);
    await settle();
    expect(fetches).toEqual(['tok-3']);

    tabsOf(host)[0].click();
    morphTo(serverTabs({ ...THREE, Three: { lazy: next } }));
    await microtask();
    await settle();
    expect(fetches).toEqual(['tok-3']);
    expect(lazy.suspended).toBe(true);
    expect(observed.has(lazy)).toBe(true);
    expect(panelsOf(host)[2].hidden).toBe(true);

    tabsOf(host)[2].click();
    intersect(lazy);
    await settle();
    expect(fetches).toEqual(['tok-3', next]);
  });

  it.each(['tok-3', 'tok-3b'])('a visible lazy panel issues exactly one POST on morph to %s', async (next) => {
    const html = serverTabs(THREE);
    const host = mount(html);
    const lazy = host.querySelector('lazy-shortcode') as any;
    tabsOf(host)[2].click();
    intersect(lazy);
    await settle();

    expect(fetches).toEqual(['tok-3']);
    morphTo(serverTabs({ ...THREE, Three: { lazy: next } }));
    await settle();
    expect(fetches).toEqual(['tok-3', next]);
    expect(lazy.textContent).toContain(`loaded ${next}`);
    expect(lazy.suspended).toBe(false);
  });
});

describe('LazyShortcode.suspended', () => {
  function loadedLazy() {
    mount('<lazy-shortcode data-token="tok-l"></lazy-shortcode>');
    const lazy = document.querySelector('lazy-shortcode') as any;
    intersect(lazy);
    return lazy;
  }

  it('defaults to false, and a morph of a loaded block reveals it again', async () => {
    const lazy = loadedLazy();
    await settle();
    expect(lazy.suspended).toBe(false);
    lazy.refreshFromMorph(lazy);
    await settle();
    expect(fetches).toEqual(['tok-l', 'tok-l']);
  });

  it('when set, a morph resets the block and observes it instead of fetching', async () => {
    const lazy = loadedLazy();
    await settle();
    lazy.suspended = true;
    observed.clear();
    lazy.refreshFromMorph(lazy);
    await settle();
    expect(fetches).toEqual(['tok-l']);
    expect(lazy._revealed).toBe(false);
    expect(observed.has(lazy)).toBe(true);
  });

  it('keeps a value set before the element upgraded', () => {
    document.body.innerHTML = '<div></div>';
    const lazy = document.createElement('lazy-shortcode') as any;
    lazy.suspended = true;
    lazy.setAttribute('data-token', 'tok-early');
    document.body.firstElementChild!.appendChild(lazy);
    expect(lazy.suspended).toBe(true);
  });
});

describe('a reload swap', () => {
  it.each(['old', 'new'])('does not restore a duplicate identity on the %s side', async (repeatedSide) => {
    const html = serverTabs({ One: 'a', Two: 'b' });
    mount(`<div id="region">${html}${repeatedSide === 'old' ? html : ''}</div>`);
    const region = document.getElementById('region')!;
    const before = Array.from(region.querySelectorAll('tabs-shortcode'));
    before.forEach((host) => tabsOf(host)[1].click());
    const oldIds = before.map((host) => tabsOf(host)[0].id);

    const next = html + (repeatedSide === 'new' ? html : '');
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ html: next, entity: null }), { status: 200 }));
    await expect(reloadInto(region, region, 'region-token')).resolves.toBe(true);

    for (const host of region.querySelectorAll('tabs-shortcode')) {
      expect(tabsOf(host)[0].getAttribute('aria-selected')).toBe('true');
      expect(oldIds).not.toContain(tabsOf(host)[0].id);
    }
  });

  it('carries each tab set selection and IDs into the re-rendered content', async () => {
    const html = serverTabs({ One: 'a', Two: 'b' });
    mount(`<div id="region">${html}</div>`);
    const region = document.getElementById('region')!;
    const before = region.querySelector('tabs-shortcode') as HTMLElement;
    tabsOf(before)[1].click();
    const ids = tabsOf(before).map((t) => t.id);

    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ html, entity: null }), { status: 200 }));
    await expect(reloadInto(region, region, 'region-token')).resolves.toBe(true);

    const after = region.querySelector('tabs-shortcode') as HTMLElement;
    expect(after).not.toBe(before);
    expect(tabsOf(after).map((t) => t.getAttribute('aria-selected'))).toEqual(['false', 'true']);
    expect(panelsOf(after).map((p) => p.hidden)).toEqual([true, false]);
    expect(tabsOf(after).map((t) => t.id)).toEqual(ids);
  });
});
