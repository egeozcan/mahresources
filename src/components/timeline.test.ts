import { describe, it, expect, beforeEach, vi } from 'vitest';
import timeline from './timeline.js';

// Mock ResizeObserver to be constructable
global.ResizeObserver = vi.fn(function () {
    return {
        observe: vi.fn(),
        unobserve: vi.fn(),
        disconnect: vi.fn(),
    };
});

describe('timeline component', () => {
    let component;
    const mockApiUrl = '/v1/test/timeline';
    const mockEntityType = 'test';
    const mockDefaultView = '/test';

    beforeEach(() => {
        vi.clearAllMocks();
        component = timeline({
            apiUrl: mockApiUrl,
            entityType: mockEntityType,
            defaultView: mockDefaultView
        });
        // Mock $el
        component.$el = { clientWidth: 1200 };
    });

    it('should calculate columns without mutating state in calculateColumns', () => {
        component.columns = 10;
        const result = component.calculateColumns();

        expect(result).toBe(20); // 1200 / 60 = 20
        expect(component.columns).toBe(10); // State should not have changed
    });

    it('should update columns in init', () => {
        // Mock fetchBuckets
        component.fetchBuckets = vi.fn();

        component.init();
        expect(component.columns).toBe(20);
        expect(component.fetchBuckets).toHaveBeenCalled();
        expect(global.ResizeObserver).toHaveBeenCalled();
    });

    it('should respect min/max column bounds', () => {
        component.$el.clientWidth = 100;
        expect(component.calculateColumns()).toBe(5); // Math.max(5, floor(100/60))

        component.$el.clientWidth = 3000;
        expect(component.calculateColumns()).toBe(30); // Math.min(30, floor(3000/60))
    });

    it('should update columns on resize if they change', () => {
        component.fetchBuckets = vi.fn();
        component.init();

        const resizeCallback = (global.ResizeObserver as any).mock.calls[0][0];

        // Change width to trigger new column count
        component.$el.clientWidth = 600; // should be 10 cols

        resizeCallback();

        expect(component.columns).toBe(10);
        expect(component.fetchBuckets).toHaveBeenCalledTimes(2); // once in init, once on resize
    });

    it('should NOT update buckets on resize if columns stay same', () => {
        component.fetchBuckets = vi.fn();
        component.init();

        const resizeCallback = (global.ResizeObserver as any).mock.calls[0][0];

        // Change width but keep cols same
        component.$el.clientWidth = 1210; // still 20 cols

        resizeCallback();

        expect(component.columns).toBe(20);
        expect(component.fetchBuckets).toHaveBeenCalledTimes(1); // only once in init
    });
});
