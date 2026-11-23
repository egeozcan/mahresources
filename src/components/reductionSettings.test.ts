import { describe, expect, it } from 'vitest';

import { matchPreset, PRESETS } from './reductionSettings.js';

describe('matchPreset', () => {
  it('recognises each preset by its exact ordered rule', () => {
    for (const [name, rule] of Object.entries(PRESETS)) expect(matchPreset(rule)).toBe(name);
  });

  it('calls anything else Custom, including a preset in another order', () => {
    expect(matchPreset(['size_desc', 'pixels_desc', 'created_asc'])).toBe('custom');
    expect(matchPreset([])).toBe('custom');
  });
});
