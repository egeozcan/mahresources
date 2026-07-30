import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fetchAllPages, templateBundle } from './templateBundle.js';

// WS12 — the "Copy from existing" picker and the clobber confirm, from the
// 2026-07-29 UI bug hunt.
//
//	28  — loadSources() issued one un-paged GET per carrier, and the list endpoints
//	      cap at 50 rows and ignore maxResults. Measured on a seeded instance:
//	      the picker offered "Category=50, Resource Category=1, Note Type=50" and
//	      'Person' / 'Vendor' were absent.
//	154 — applying a preset replaced authored content with no prompt. Measured:
//	      CustomHeader went from 107 characters of authored template to 353 of
//	      preset, with zero dialogs.

function pagedFetch(total: number, pageSize = 50) {
  const calls: string[] = [];
  const impl = vi.fn(async (url: string) => {
    calls.push(url);
    const page = Number(new URL(url, 'http://x').searchParams.get('page') || '1');
    const start = (page - 1) * pageSize;
    const rows = [];
    for (let i = start; i < Math.min(start + pageSize, total); i++) {
      rows.push({ ID: i + 1, Name: `entity-${i + 1}` });
    }
    return { ok: true, json: async () => rows };
  });
  return { impl, calls };
}

describe('finding 28 — the copy-from picker pages past the 50-row cap', () => {
  it('walks pages until a short one and returns every row', async () => {
    const { impl, calls } = pagedFetch(73);
    const { items, complete } = await fetchAllPages('/v1/categories', impl as never);

    expect(items).toHaveLength(73);
    expect(complete).toBe(true);
    expect(calls).toEqual(['/v1/categories?page=1', '/v1/categories?page=2']);
    // The rows the report named as unreachable are on page 2.
    expect(items.map((i: { Name: string }) => i.Name)).toContain('entity-73');
  });

  it('stops after one request when the first page is short', async () => {
    const { impl, calls } = pagedFetch(3);
    const { items, complete } = await fetchAllPages('/v1/resourceCategories', impl as never);

    expect(items).toHaveLength(3);
    expect(complete).toBe(true);
    expect(calls).toHaveLength(1);
  });

  it('walks an exact multiple of the page size and then stops on the empty page', async () => {
    const { impl, calls } = pagedFetch(100);
    const { items, complete } = await fetchAllPages('/v1/categories', impl as never);

    expect(items).toHaveLength(100);
    expect(complete).toBe(true);
    // 50, 50, then the empty third page that proves the walk is over.
    expect(calls).toHaveLength(3);
  });

  it('reports incompleteness rather than looping forever', async () => {
    // Every page is full, so the walk can only stop at the page cap.
    const impl = vi.fn(async () => ({
      ok: true,
      json: async () => Array.from({ length: 50 }, (_, i) => ({ ID: i, Name: `x${i}` })),
    }));
    const { items, complete, reason } = await fetchAllPages('/v1/categories', impl as never);

    expect(complete).toBe(false);
    expect(reason).toBe('cap');
    expect(items).toHaveLength(20 * 50);
    expect(impl).toHaveBeenCalledTimes(20);
  });

  // A failed page is not the end of the list. Reporting it as complete suppressed
  // the truncation warning, which is finding 28 again: a picker missing sources
  // with nothing on screen saying so.
  it('keeps what arrived when a later page fails, and says the list is incomplete', async () => {
    let n = 0;
    const impl = vi.fn(async () => {
      n++;
      if (n === 1) {
        return { ok: true, json: async () => Array.from({ length: 50 }, (_, i) => ({ ID: i })) };
      }
      return { ok: false, json: async () => [] };
    });
    const { items, complete, reason } = await fetchAllPages('/v1/categories', impl as never);

    expect(items).toHaveLength(50);
    expect(complete).toBe(false);
    expect(reason).toBe('error');
  });

  it('says the list is incomplete when the fetch itself throws', async () => {
    let n = 0;
    const impl = vi.fn(async () => {
      n++;
      if (n === 1) {
        return { ok: true, json: async () => Array.from({ length: 50 }, (_, i) => ({ ID: i })) };
      }
      throw new TypeError('Failed to fetch');
    });
    const { items, complete, reason } = await fetchAllPages('/v1/categories', impl as never);

    expect(items).toHaveLength(50);
    expect(complete).toBe(false);
    expect(reason).toBe('error');
  });

  it('appends page= correctly to a url that already has a query string', async () => {
    const { impl, calls } = pagedFetch(1);
    await fetchAllPages('/v1/categories?Name=x', impl as never);
    expect(calls[0]).toBe('/v1/categories?Name=x&page=1');
  });
});

describe('finding 154 — a clobber is confirmed, and only when there is something to lose', () => {
  const editors: Record<string, string> = {};

  function bundleWith(values: Record<string, string>) {
    for (const k of Object.keys(editors)) delete editors[k];
    Object.assign(editors, values);
    const b = templateBundle({ carrier: 'category' });
    b.getEditor = (name: string) => editors[name] ?? '';
    b.setEditor = (name: string, value: string) => {
      editors[name] = value;
    };
    b.setSectionConfig = () => {};
    return b;
  }

  // The vitest environment here is plain node, so there is no window at all.
  // confirmOverwrite calls window.confirm (the app's own idiom for a blocking
  // prompt), so the double has to be a window object rather than a bare confirm.
  let confirmSpy: ReturnType<typeof vi.fn>;
  function stubConfirm(answer: boolean) {
    confirmSpy = vi.fn(() => answer);
    vi.stubGlobal('window', { confirm: confirmSpy });
  }

  beforeEach(() => {
    stubConfirm(true);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not prompt when every slot is empty', () => {
    const b = bundleWith({});
    expect(b.confirmOverwrite('the "contact-card" preset')).toBe(true);
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it('does not prompt when the only content is whitespace', () => {
    const b = bundleWith({ CustomHeader: '   \n  ' });
    expect(b.confirmOverwrite('the "contact-card" preset')).toBe(true);
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it('prompts once authored content exists, naming the source and the blast radius', () => {
    const b = bundleWith({ CustomHeader: '<div>[property name]</div>' });
    expect(b.confirmOverwrite('the "contact-card" preset')).toBe(true);

    expect(confirmSpy).toHaveBeenCalledTimes(1);
    const msg = confirmSpy.mock.calls[0][0] as string;
    expect(msg).toContain('1 template field');
    expect(msg).toContain('contact-card');
    expect(msg).toContain('discarded');
  });

  it('counts every filled slot, including the Meta JSON Schema', () => {
    const b = bundleWith({
      CustomHeader: '<div>a</div>',
      CustomSidebar: '<div>b</div>',
      MetaSchema: '{"type":"object"}',
    });
    b.confirmOverwrite('the imported bundle "x.json"');
    const msg = confirmSpy.mock.calls[0][0] as string;
    expect(msg).toContain('3 template fields');
  });

  it('applyPreset leaves the editors untouched when the confirm is declined', () => {
    stubConfirm(false);
    const b = bundleWith({ CustomHeader: '<div>authored</div>' });
    b.presets = [{ name: 'contact-card', carrier: 'category', slots: { header: '<div>preset</div>' } }];
    b.presetChoice = 'contact-card';

    b.applyPreset();

    expect(editors.CustomHeader).toBe('<div>authored</div>');
  });

  it('applyPreset replaces the content when the confirm is accepted', () => {
    const b = bundleWith({ CustomHeader: '<div>authored</div>' });
    b.presets = [{ name: 'contact-card', carrier: 'category', slots: { header: '<div>preset</div>' } }];
    b.presetChoice = 'contact-card';

    b.applyPreset();

    expect(editors.CustomHeader).toBe('<div>preset</div>');
  });

  it('copyFrom is gated by the same confirm', () => {
    stubConfirm(false);
    const b = bundleWith({ CustomHeader: '<div>authored</div>' });
    b.sources.category = [{ ID: 7, Name: 'Person', CustomHeader: '<div>copied</div>' }];
    b.copyChoice = 'category:7';

    b.copyFrom();

    expect(editors.CustomHeader).toBe('<div>authored</div>');
    const msg = confirmSpy.mock.calls[0][0] as string;
    expect(msg).toContain('Person');
  });

  // applyBundle writes SectionConfig too, for a same-carrier bundle that carries
  // one, and the confirmation counted only the slots and MetaSchema. A form whose
  // only authored content was the section layout therefore scored zero fields at
  // risk, returned true without prompting, and lost the layout silently.
  it('counts SectionConfig when a same-carrier bundle will replace it', () => {
    const b = bundleWith({ SectionConfig: '{"showMeta":true}' });
    const incoming = { carrier: 'category', slots: {}, sectionConfig: '{"showMeta":false}' };

    expect(b.confirmOverwrite('the "contact-card" preset', incoming)).toBe(true);

    expect(confirmSpy).toHaveBeenCalledTimes(1);
    expect(confirmSpy.mock.calls[0][0] as string).toContain('1 template field');
  });

  it('applyPreset prompts when the section layout is the only thing at risk', () => {
    stubConfirm(false);
    const applied: string[] = [];
    const b = bundleWith({ SectionConfig: '{"showMeta":true}' });
    b.setSectionConfig = (json: string) => { applied.push(json); };
    b.presets = [{ name: 'contact-card', carrier: 'category', slots: {}, sectionConfig: '{"showMeta":false}' }];
    b.presetChoice = 'contact-card';

    b.applyPreset();

    expect(confirmSpy).toHaveBeenCalledTimes(1);
    expect(applied).toEqual([]);
  });

  it('does not count SectionConfig for a cross-carrier bundle, which never writes it', () => {
    // Positive control on the branch: applyBundle skips setSectionConfig when the
    // carriers differ, so counting it there would prompt about a field that is
    // not at risk.
    const b = bundleWith({ SectionConfig: '{"showMeta":true}' });
    const incoming = { carrier: 'noteType', slots: {}, sectionConfig: '{"showMeta":false}' };

    expect(b.confirmOverwrite('the template from "A Note Type"', incoming)).toBe(true);
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it('does not count SectionConfig when the incoming bundle has none', () => {
    // setSectionConfig no-ops on an empty string, so nothing is replaced.
    const b = bundleWith({ SectionConfig: '{"showMeta":true}' });
    const incoming = { carrier: 'category', slots: {}, sectionConfig: '' };

    expect(b.confirmOverwrite('the "contact-card" preset', incoming)).toBe(true);
    expect(confirmSpy).not.toHaveBeenCalled();
  });
});

describe('finding 28 — an incomplete source list always says so', () => {
  let notices: string[];

  function bundleWithFetch(impl: unknown) {
    const b = templateBundle({ carrier: 'category' });
    notices = [];
    b.notify = (msg: string) => { notices.push(msg); };
    vi.stubGlobal('fetch', impl);
    return b;
  }

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('warns that sources are missing when a page request fails', async () => {
    let n = 0;
    await bundleWithFetch(async () => {
      n++;
      // First page full for every carrier, then a 500 — the walk gives up part way.
      if (n <= 3) {
        return { ok: true, json: async () => Array.from({ length: 50 }, (_, i) => ({ ID: i, Name: `c${i}` })) };
      }
      return { ok: false, json: async () => [] };
    }).loadSources();

    expect(notices).toHaveLength(1);
    expect(notices[0]).toContain('a request failed');
  });

  it('says nothing when every list ends cleanly', async () => {
    // Positive control: the warning is about the failure, not about loading.
    await bundleWithFetch(async () => ({
      ok: true,
      json: async () => [{ ID: 1, Name: 'Person' }],
    })).loadSources();

    expect(notices).toEqual([]);
  });
});
