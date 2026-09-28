import { describe, expect, test } from 'vitest';
import { formatLocalTime } from './localTime.js';

describe('formatLocalTime', () => {
    test('writes an instant as the /jobs cards do, in the reader\'s zone', () => {
        const instant = new Date(2026, 8, 26, 14, 7, 3);
        expect(formatLocalTime(instant)).toBe('2026-09-26 14:07');
        expect(formatLocalTime(instant, { seconds: true })).toBe('2026-09-26 14:07:03');
        expect(formatLocalTime(instant.toISOString(), { seconds: true })).toBe('2026-09-26 14:07:03');
    });

    test('writes nothing for no instant or an unreadable one', () => {
        expect(formatLocalTime(null)).toBe('');
        expect(formatLocalTime(undefined)).toBe('');
        expect(formatLocalTime('')).toBe('');
        expect(formatLocalTime('not a time')).toBe('');
    });
});
