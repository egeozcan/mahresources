import { describe, expect, test } from 'vitest';
import table from '../../server/jobview/job_states.json';
import { JOB_STATES, isWorking, presentState, scheduledStartText, statesInGroup, terminalStates } from './jobStates.js';

describe('the shared state table', () => {
    test('names every state once, in the lifecycle order the Go side presents', () => {
        expect(JOB_STATES).toEqual(['scheduled', 'queued', 'running', 'paused', 'blocked', 'succeeded', 'failed', 'cancelled', 'interrupted']);
        expect(Object.keys(table.states)).toEqual(JOB_STATES);
    });

    test('groups states as the drawer and the Job Center quick filters do', () => {
        expect(statesInGroup('attention')).toEqual(['blocked', 'failed', 'interrupted']);
        expect(statesInGroup('active')).toEqual(['scheduled', 'queued', 'running', 'paused']);
        expect(statesInGroup('finished')).toEqual(['succeeded', 'cancelled']);
        expect(terminalStates()).toEqual(['succeeded', 'failed', 'cancelled', 'interrupted']);
    });

    test('a partial success reads as partial, and only running work is working', () => {
        expect(presentState({ state: 'succeeded', phase: 'partial' })).toMatchObject({ label: 'Partially completed', tone: 'warning' });
        expect(presentState({ state: 'running', phase: 'partial' }).label).toBe('Running');
        expect(presentState({ state: 'paused' })).toMatchObject({ label: 'Paused', tone: 'paused', group: 'active' });
        expect(presentState({ state: 'bogus' })).toMatchObject({ label: 'Unknown', group: 'other' });
        expect(JOB_STATES.filter(state => isWorking(state))).toEqual(['running']);
    });
});

describe('scheduledStartText', () => {
    const now = Date.parse('2026-09-27T12:00:00Z');
    const instant = (date: Date) => date.toISOString();

    test('says when scheduled work starts and how long that is from now', () => {
        const job = { state: 'scheduled', scheduledFor: '2026-09-27T13:30:00Z' };
        expect(scheduledStartText(job, now, instant)).toBe('Starts 2026-09-27T13:30:00.000Z (in 1 h 30 min)');
        const later = { state: 'scheduled', scheduledFor: '2026-09-29T12:00:00Z' };
        expect(scheduledStartText(later, now, instant)).toBe('Starts 2026-09-29T12:00:00.000Z (in 2 d)');
    });

    test('a scheduled time that has come is due, and other states say nothing', () => {
        expect(scheduledStartText({ state: 'scheduled', scheduledFor: '2026-09-27T11:59:00Z' }, now, instant))
            .toBe('Starts 2026-09-27T11:59:00.000Z (due now)');
        expect(scheduledStartText({ state: 'running', scheduledFor: '2026-09-27T13:30:00Z' }, now, instant)).toBe('');
        expect(scheduledStartText({ state: 'scheduled' }, now, instant)).toBe('');
        expect(scheduledStartText({ state: 'scheduled', scheduledFor: 'nonsense' }, now, instant)).toBe('');
    });
});
