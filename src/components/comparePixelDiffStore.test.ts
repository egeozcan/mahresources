import { afterEach, describe, expect, test } from 'vitest';
// @ts-expect-error -- plain JS module with no type declarations
import { registerComparePixelDiffStore } from './comparePixelDiffStore.js';

describe('compare pixel-diff store', () => {
  const originalWindow = (globalThis as any).window;

  afterEach(() => {
    if (originalWindow === undefined) delete (globalThis as any).window;
    else (globalThis as any).window = originalWindow;
  });

  test('bridges the comparator event into the registered Alpine store', () => {
    const stores: Record<string, any> = {};
    const Alpine = {
      store(name: string, value?: any) {
        if (arguments.length === 2) stores[name] = value;
        return stores[name];
      },
    };
    let listener: ((event: any) => void) | undefined;
    (globalThis as any).window = {
      addEventListener(type: string, fn: (event: any) => void) {
        if (type === 'compare-pixel-diff') listener = fn;
      },
    };

    registerComparePixelDiffStore(Alpine);
    expect(stores.comparePixelDiff).toEqual({ percent: null, overlapEmpty: false });

    listener!({ detail: { percent: 37, overlapEmpty: false } });
    expect(stores.comparePixelDiff).toEqual({ percent: 37, overlapEmpty: false });

    listener!({ detail: { percent: null, overlapEmpty: true } });
    expect(stores.comparePixelDiff).toEqual({ percent: null, overlapEmpty: true });
  });
});
