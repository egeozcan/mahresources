import { describe, expect, it } from 'vitest';

import { matchPreset, nameToSend, PRESETS } from './reductionSettings.js';

describe('matchPreset', () => {
  it('recognises each preset by its exact ordered rule', () => {
    for (const [name, rule] of Object.entries(PRESETS)) expect(matchPreset(rule)).toBe(name);
  });

  it('calls anything else Custom, including a preset in another order', () => {
    expect(matchPreset(['size_desc', 'pixels_desc', 'created_asc'])).toBe('custom');
    expect(matchPreset([])).toBe('custom');
  });
});

describe('nameToSend', () => {
  it('sends nothing for a name the reviewer did not change, so a bounded display name never overwrites the stored one', () => {
    const shown = 'a'.repeat(341);
    expect(nameToSend(shown, shown)).toBe('');
  });

  it('sends the new name once the reviewer edited it', () => {
    expect(nameToSend('Renamed', 'Old name')).toBe('Renamed');
  });
});
