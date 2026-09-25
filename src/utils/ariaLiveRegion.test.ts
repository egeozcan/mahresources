// @vitest-environment happy-dom
import { afterEach, describe, expect, test, vi } from 'vitest';
import { createLiveRegion } from './ariaLiveRegion.js';

afterEach(() => vi.useRealTimers());

describe('createLiveRegion', () => {
    test('the newest message replaces a pending one', () => {
        vi.useFakeTimers();
        const region = createLiveRegion();
        region.announce('A');
        region.announce('B');
        vi.advanceTimersByTime(50);
        expect(region.element.textContent).toBe('B');
        region.destroy();
    });

    test('cancel withdraws a pending message and returns it', () => {
        vi.useFakeTimers();
        const region = createLiveRegion();
        region.announce('A');
        expect(region.cancel()).toBe('A');
        vi.advanceTimersByTime(50);
        expect(region.element.textContent).toBe('');
        expect(region.cancel()).toBeNull();

        region.announce('B');
        vi.advanceTimersByTime(50);
        expect(region.cancel()).toBeNull();
        expect(region.element.textContent).toBe('B');
        region.destroy();
    });
});
