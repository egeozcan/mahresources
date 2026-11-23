import { describe, expect, it } from 'vitest';

import { progressText } from './reductionCompute.js';

describe('progressText', () => {
  it('counts while the phase has a total', () => {
    expect(progressText({ phase: 'Identical Resources', phaseCount: 1200, phaseTotal: 8000 })).toBe('Comparing… 1,200 of 8,000');
  });

  it('names the phase while it has none', () => {
    expect(progressText({ phase: 'resolving the Extent' })).toBe('Resolving the Extent…');
  });

  it('says it is starting before the first phase', () => {
    expect(progressText({ phase: 'clustering' })).toBe('Starting…');
    expect(progressText({})).toBe('Starting…');
  });
});
