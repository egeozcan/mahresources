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
  function bundle(reply: unknown) {
    const tb = templateBundle({ carrier: 'category' }) as ReturnType<typeof templateBundle> & Record<string, unknown>;
    tb.generationPrompt = 'restyle';
    tb.getEditor = () => 'existing';
    tb.setEditor = vi.fn();
    tb.notify = vi.fn();
    Object.assign(tb, { $refs: {}, $nextTick: () => Promise.resolve() });
    const stores: Record<string, unknown> = { generationDisclosure: { open: vi.fn(async () => true) }, templatePreview: {} };
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
    const tb = bundle({ valid: true, slots: { CustomHeader: '<h1>New</h1>', CustomHeaderCSS: '' } });
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
      issues: [{ severity: 'error', message: "The model's reply was cut off at its output limit" }, issue], slotIssues: { CustomMRQLResult: [issue] } });
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
});
