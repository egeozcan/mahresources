// @vitest-environment happy-dom
import { describe, it, expect, vi, afterEach } from 'vitest';
import { templateBundle } from './templateBundle.js';

// entityToBundle is pure (no DOM), so it can be tested directly. The key hazard
// is carrier-dependent JSON field casing: Category/ResourceCategory serialize
// their section config as `sectionConfig` (lowercase json tag), while NoteType
// has no json tag and serializes it as `SectionConfig`. The bundle must capture
// it for all three carriers.
describe('templateBundle.entityToBundle', () => {
  const tb = templateBundle({ carrier: 'category' });
  const sectionConfig = { showMeta: false, showRelated: true };

  it('captures sectionConfig from a Category response (lowercase json key)', () => {
    const obj = { ID: 1, Name: 'Cat', CustomHeader: '<h1>h</h1>', sectionConfig };
    const bundle = tb.entityToBundle(obj, 'category');
    expect(bundle.sectionConfig).toBe(JSON.stringify(sectionConfig));
    expect((bundle.slots as Record<string, string>).header).toBe('<h1>h</h1>');
  });

  it('captures sectionConfig from a Resource Category response (lowercase json key)', () => {
    const obj = { ID: 2, Name: 'RC', sectionConfig };
    const bundle = tb.entityToBundle(obj, 'resourceCategory');
    expect(bundle.sectionConfig).toBe(JSON.stringify(sectionConfig));
  });

  it('captures SectionConfig from a Note Type response (capitalized field name)', () => {
    const obj = { ID: 3, Name: 'NT', SectionConfig: sectionConfig };
    const bundle = tb.entityToBundle(obj, 'noteType');
    expect(bundle.sectionConfig).toBe(JSON.stringify(sectionConfig));
  });

  it('passes through an already-stringified section config unchanged', () => {
    const str = JSON.stringify(sectionConfig);
    const obj = { ID: 4, Name: 'Cat', sectionConfig: str };
    const bundle = tb.entityToBundle(obj, 'category');
    expect(bundle.sectionConfig).toBe(str);
  });

  it('keeps picker HTML and CSS in an exported carrier bundle', () => {
    const bundle = tb.entityToBundle({ ID: 6, Name: 'Person',
      CustomEntityPickerResult: '<b>Person</b>', CustomEntityPickerResultCSS: '.person{color:red}',
    }, 'category');
    expect(bundle.slots.entityPickerResult).toBe('<b>Person</b>');
    expect(bundle.slots.entityPickerResultCSS).toBe('.person{color:red}');
  });

  it('yields an empty section config when the entity has none', () => {
    const bundle = tb.entityToBundle({ ID: 5, Name: 'Cat' }, 'category');
    expect(bundle.sectionConfig).toBe('');
  });
});

// Pair completeness is checked before touching any editor, including valid
// earlier pairs, and no generated slot reaches an editor before Apply.
describe('whole-template generation pairs', () => {
  function bundle(reply: unknown, editors: Record<string, string> | null = null) {
    const tb = templateBundle({ carrier: 'category' }) as ReturnType<typeof templateBundle> & Record<string, unknown>;
    tb.generationPrompt = 'restyle';
    tb.getEditor = editors ? (name: string) => editors[name] ?? '' : () => 'existing';
    tb.setEditor = vi.fn();
    tb.notify = vi.fn();
    Object.assign(tb, { $refs: {}, $nextTick: () => Promise.resolve() });
    const ask = vi.fn(async (_message: string, _options?: unknown) => true);
    (tb as Record<string, unknown>).ask = ask;
    const stores: Record<string, unknown> = { generationDisclosure: { open: vi.fn(async () => true) }, templatePreview: {}, confirmDialog: { ask } };
    vi.stubGlobal('window', { Alpine: { store: (name: string) => stores[name] } });
    vi.stubGlobal('Alpine', { store: (name: string) => stores[name] });
    vi.stubGlobal('fetch', vi.fn(async (_url: string, init: { body: string }) => (JSON.parse(init.body).dryRun
      ? { ok: true, status: 200, json: async () => ({ groundingHash: 'h', sections: [] }) }
      : { ok: true, status: 200, json: async () => reply })));
    return tb;
  }

  it('rejects a draft containing an unpaired field without applying anything', async () => {
    const tb = bundle({ valid: true, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '', CustomMRQLResult: '<p>Incomplete</p>' } });
    await tb.generateBundle();
    expect(tb.setEditor).not.toHaveBeenCalled();
    expect(tb.pendingDraft).toBeNull();
    expect(tb.notify).toHaveBeenLastCalledWith(expect.stringContaining('CustomMRQLResultCSS together'), 'warn');
  });

  it('holds complete pairs, including empty CSS, until Apply', async () => {
    const tb = bundle({ valid: true, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '' } }, {});
    await tb.generateBundle();
    expect(tb.setEditor).not.toHaveBeenCalled();
    expect(tb.pendingDraft?.fields).toEqual(['CustomHeader', 'CustomHeaderCSS']);
    await tb.applyPendingDraft();
    expect(tb.setEditor).toHaveBeenCalledTimes(2);
    expect(tb.setEditor).toHaveBeenCalledWith('CustomHeader', '<h1>New</h1>');
    expect(tb.setEditor).toHaveBeenCalledWith('CustomHeaderCSS', '');
  });

  it('lists a truncated half pair for review but applies only complete pairs', async () => {
    const issue = { severity: 'error', message: 'CustomMRQLResult came back without CustomMRQLResultCSS before the reply was cut off, so this pair cannot be applied.' };
    const tb = bundle({ valid: false, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '', CustomMRQLResult: '<p>Half</p>' },
      issues: [{ severity: 'error', message: "The model's reply was cut off at its output limit" }, issue], slotIssues: { CustomMRQLResult: [issue] } }, {});
    await tb.generateBundle();
    expect(tb.pendingDraft?.fields).toEqual(['CustomHeader', 'CustomHeaderCSS', 'CustomMRQLResult']);
    expect(tb.pendingDraft?.blocked).toEqual(['CustomMRQLResult']);
    await tb.applyPendingDraft();
    expect(tb.setEditor).toHaveBeenCalledTimes(2);
    expect(tb.setEditor).not.toHaveBeenCalledWith('CustomMRQLResult', expect.anything());
  });

  it('offers no Apply when every returned slot is a half pair', async () => {
    const issue = { severity: 'error', message: 'cut' };
    const tb = bundle({ valid: false, slots: { CustomMRQLResult: '<p>Half</p>' }, issues: [issue] });
    await tb.generateBundle();
    expect(tb.pendingDraft?.blocked).toEqual(['CustomMRQLResult']);
    expect(tb.canApplyDraft()).toBe(false);
    await tb.applyPendingDraft();
    expect(tb.setEditor).not.toHaveBeenCalled();
  });
  afterEach(() => vi.unstubAllGlobals());

  // The section picker and the fan-out's failed slots.
  describe('sections and failed slots', () => {
    function body(tb: Record<string, unknown>) {
      const calls = (fetch as unknown as { mock: { calls: [string, { body: string }][] } }).mock.calls;
      return JSON.parse(calls[calls.length - 1][1].body);
    }

    it('sends slots only when at least one section is ticked', async () => {
      const tb = bundle({ valid: true, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '' } }, {}) as Record<string, any>;
      await tb.generateBundle();
      expect('slots' in body(tb)).toBe(false);

      tb.selectedSections = ['CustomSidebar', 'CustomCSS'];
      await tb.generateBundle();
      expect(body(tb).slots).toEqual(['CustomSidebar', 'CustomCSS']);
    });

    it('names the sections that failed beside the draft', async () => {
      const tb = bundle({ valid: true, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '' }, failedSlots: ['CustomSidebar'],
        issues: [{ severity: 'warning', message: 'CustomSidebar could not be generated and was left unchanged: the request ran out of time' }] }, {}) as Record<string, any>;
      await tb.generateBundle();
      expect(tb.pendingDraft.failedSlots).toEqual(['CustomSidebar']);
      expect(tb.pendingDraft.issues).toEqual([]);
      expect(tb.notify).toHaveBeenLastCalledWith(expect.stringContaining('CustomSidebar'), 'warn');
      expect(tb.pendingDraft.fields).toEqual(['CustomHeader', 'CustomHeaderCSS']);
    });

    it('offers the sections the form has an editor for', () => {
      const tb = templateBundle({ carrier: 'category' }) as Record<string, any>;
      const present = new Set(['CustomHeader', 'CustomSidebar', 'CustomOwnEntities', 'CustomCSS', 'CustomHeaderCSS']);
      vi.stubGlobal('document', { querySelector: (selector: string) => (present.has(selector.slice(7, -2)) ? {} : null) });
      expect(tb.sectionOptions().map((o: { field: string }) => o.field)).toEqual(['CustomHeader', 'CustomSidebar', 'CustomOwnEntities', 'CustomCSS']);
      expect(tb.sectionOptions().map((o: { label: string }) => o.label)).toEqual(['Header', 'Sidebar', 'Own Entities', 'Shared CSS (CustomCSS)']);
    });
  });

  // Apply asks one combined question, naming only the fields it would change.
  describe('applying a draft', () => {
    const headerOnly = { valid: true, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '.n{}' } };

    it('does not ask when every field it writes is empty', async () => {
      const tb = bundle(headerOnly, { MetaSchema: '{"type":"object"}', CustomSidebar: '<p>mine</p>' }) as Record<string, any>;
      await tb.generateBundle();
      await tb.applyPendingDraft();
      expect(tb.ask).not.toHaveBeenCalled();
      expect(tb.setEditor).toHaveBeenCalledTimes(2);
    });

    it('names only the returned fields that hold something, and nothing else', async () => {
      const tb = bundle(headerOnly, { CustomHeader: '<h1>Old</h1>', MetaSchema: '{"type":"object"}', CustomSidebar: '<p>mine</p>' }) as Record<string, any>;
      await tb.generateBundle();
      await tb.applyPendingDraft();
      expect(tb.ask).toHaveBeenCalledTimes(1);
      const message = tb.ask.mock.calls[0][0] as string;
      expect(message).toContain('CustomHeader');
      expect(message).not.toMatch(/MetaSchema|CustomSidebar|CustomHeaderCSS/);
      expect(tb.setEditor).toHaveBeenCalledTimes(2);
    });

    it('names a returned field the user cleared while the request ran, and marks edits', async () => {
      const editors: Record<string, string> = { CustomHeader: '<h1>Old</h1>', CustomHeaderCSS: '.old{}' };
      const tb = bundle(headerOnly, editors) as Record<string, any>;
      await tb.generateBundle();
      editors.CustomHeader = '';
      editors.CustomHeaderCSS = '.typed{}';
      await tb.applyPendingDraft();
      expect(tb.ask).toHaveBeenCalledTimes(1);
      const message = tb.ask.mock.calls[0][0] as string;
      expect(message).toContain('CustomHeader');
      expect(message).toContain('CustomHeaderCSS');
      expect(message).toMatch(/edited since/i);
    });

    it('writes nothing when the question is declined', async () => {
      const tb = bundle(headerOnly, { CustomHeader: '<h1>Old</h1>' }) as Record<string, any>;
      await tb.generateBundle();
      tb.ask.mockResolvedValueOnce(false);
      await tb.applyPendingDraft();
      expect(tb.setEditor).not.toHaveBeenCalled();
      expect(tb.pendingDraft).not.toBeNull();
    });
  });
});
