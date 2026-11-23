import { describe, expect, it } from 'vitest';
import { defaultReductionName, plural, CLUSTER_NOUN, COPIES } from './reductionText.js';

describe('plural', () => {
  it('uses the singular only for one', () => {
    expect(plural(1, ...CLUSTER_NOUN)).toBe('1 Cluster');
    expect(plural(8, ...CLUSTER_NOUN)).toBe('8 Clusters');
    expect(plural(0, ...COPIES)).toBe('0 copies');
    expect(plural(1, ...COPIES)).toBe('1 copy');
  });
});

describe('defaultReductionName', () => {
  const date = /\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/;
  it('names the selection and the date', () => {
    expect(defaultReductionName({ all: true })).toMatch(/^Duplicates in everything, /);
    expect(defaultReductionName({ resources: 12 })).toMatch(/^Duplicates in 12 Resources, /);
    expect(defaultReductionName({ groups: 1 })).toMatch(/^Duplicates in 1 Group, /);
    expect(defaultReductionName({ groupName: 'Holiday' })).toMatch(/^Duplicates in Holiday, /);
    expect(defaultReductionName({ all: true })).toMatch(date);
  });
});
