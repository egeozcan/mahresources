// @vitest-environment happy-dom
import { beforeEach, describe, expect, it } from 'vitest';
import { readTimelineState, writeTimelineState } from './timelineState.js';

describe('timeline URL state', () => {
    beforeEach(() => {
        window.history.replaceState(null, '', '/tags/timeline?Name=applied&SortBy=Name');
        document.body.innerHTML = '<aside><form aria-label="Filter tags"><input name="Name" value="unsaved"></form></aside>';
    });

    it('restores all chart settings', () => {
        expect(readTimelineState('?timelineMode=updated&timelineGranularity=week&timelineAnchor=2026-08-01')).toEqual({
            timelineMode: 'updated', granularity: 'week', anchor: '2026-08-01',
        });
    });

    it('uses defaults for invalid settings and impossible dates', () => {
        for (const anchor of ['2026-02-30', 'garbage', '2026-13-01']) {
            expect(readTimelineState(`?timelineMode=bad&timelineGranularity=bad&timelineAnchor=${anchor}`, '2026-09-07')).toEqual({
                timelineMode: 'created', granularity: 'month', anchor: '2026-09-07',
            });
        }
    });

    it('updates the URL and GET form without applying pending edits or duplicating fields', () => {
        const state = { timelineMode: 'updated', granularity: 'year', anchor: '2025-01-01' };
        writeTimelineState(state);
        writeTimelineState({ ...state, granularity: 'week' });
        const params = new URLSearchParams(window.location.search);
        expect(params.get('Name')).toBe('applied');
        expect(params.get('SortBy')).toBe('Name');
        expect(readTimelineState(window.location.search)).toEqual({ ...state, granularity: 'week' });
        const form = document.querySelector('form')!;
        expect(form.querySelectorAll('input[type="hidden"]')).toHaveLength(3);
        const submitted = new FormData(form);
        expect(submitted.get('Name')).toBe('unsaved');
        expect(submitted.get('timelineGranularity')).toBe('week');
        expect(submitted.get('timelineAnchor')).toBe('2025-01-01');
    });
});
