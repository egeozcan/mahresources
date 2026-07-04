import { describe, it, expect } from 'vitest';
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

  it('yields an empty section config when the entity has none', () => {
    const bundle = tb.entityToBundle({ ID: 5, Name: 'Cat' }, 'category');
    expect(bundle.sectionConfig).toBe('');
  });
});
