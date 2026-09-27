import { afterEach, describe, expect, test, vi } from 'vitest';

afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
});

describe('Job preference channel', () => {
    test('names the jobs a successful record-keeping command changed, and nothing else', async () => {
        const { preferenceCommandJobIDs } = await import('./jobPreferenceChannel.js');
        expect(preferenceCommandJobIDs('/v1/jobs/j-1/commands/forget', { method: 'POST' })).toEqual(['j-1']);
        expect(preferenceCommandJobIDs('/v1/jobs/commands/pin', { method: 'POST', body: JSON.stringify({ jobIds: ['a', 'b'] }) })).toEqual(['a', 'b']);
        expect(preferenceCommandJobIDs('/v1/jobs/j-1/commands/retry', { method: 'POST' })).toBe(null);
        expect(preferenceCommandJobIDs('/v1/jobs/commands/dismiss', {})).toBe(null);
    });

    test('a Job page\'s command tells every listening panel', async () => {
        const posted: any[] = [];
        vi.stubGlobal('BroadcastChannel', class {
            onmessage: any = null;
            constructor(public name: string) {}
            postMessage(message: any) { posted.push({ name: this.name, message }); }
            close() {}
        });
        vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ result: {} }) })));
        const { jobCenter } = await import('../components/jobCenter.js');
        const center = jobCenter();
        await center.fetchJSON('/v1/jobs/j-9/commands/dismiss', { method: 'POST', body: '{}' });
        await center.fetchJSON('/v1/jobs/j-9', {});
        expect(posted).toEqual([{ name: 'mahresources-job-preferences', message: { command: 'dismiss', jobIds: ['j-9'] } }]);
    });
});
