import { describe, expect, it } from 'vitest';

import { isNameEdited, matchPreset, nameToSend, PRESETS } from './reductionSettings.js';

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

  it('sends nothing for a stored name with a newline the input stripped from its value', () => {
    // The typed value and the default differ only by the stripped line breaks, so the
    // comparison can say "unchanged" through the normalisation alone.
    const original = 'First line\nSecond line\r\nthird';
    const typed = 'First lineSecond linethird';
    expect(typed).not.toBe(original);
    expect(nameToSend(typed, original)).toBe('');
    expect(isNameEdited(typed, original)).toBe(false);
    expect(isNameEdited(typed, 'a different default', original)).toBe(false);
    expect(isNameEdited(typed + '!', original)).toBe(true);
  });

  it('sends the typed name when the reviewer edited a name that had a newline', () => {
    const original = 'First line\nSecond line';
    expect(nameToSend('First lineSecond line!', 'First lineSecond line', original)).toBe('First lineSecond line!');
    expect(isNameEdited('First lineSecond line!', 'First lineSecond line', original)).toBe(true);
  });

  it('sends the new name once the reviewer edited it', () => {
    expect(nameToSend('Renamed', 'Old name')).toBe('Renamed');
  });
});
