// @vitest-environment happy-dom

import { afterEach, describe, expect, test, vi } from 'vitest';

afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
});

// A server whose initial GET and every PUT answer only when the test says so.
function slowServer() {
    const gets: Array<() => void> = [];
    const puts: Array<{ value: unknown, answer: (ok?: boolean) => void }> = [];
    // No legacy preferences to migrate.
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {}, key: () => null, length: 0 });
    let inFlight = 0;
    let mostInFlight = 0;
    vi.stubGlobal('fetch', vi.fn((_url: string, init: any = {}) => {
        if (!init.method) {
            return new Promise(resolve => gets.push(() => resolve({ ok: true, json: async () => ({}) })));
        }
        inFlight += 1;
        mostInFlight = Math.max(mostInFlight, inFlight);
        return new Promise(resolve => puts.push({
            value: JSON.parse(init.body).value,
            answer: (ok = true) => { inFlight -= 1; resolve({ ok }); },
        }));
    }));
    return { gets, puts, mostInFlight: () => mostInFlight };
}

const tick = () => new Promise(resolve => setTimeout(resolve, 0));

describe('userSettings writes', () => {
    test('writes of one key reach the server one after another, the last one made last, across the initial load', async () => {
        const server = slowServer();
        const settings = await import('./userSettings.js');

        // Chosen before the initial GET has answered: sent at once.
        const first = settings.saveNow('jobsPanelScope', 'everyone');
        await vi.waitFor(() => expect(server.puts.map(put => put.value)).toEqual(['everyone']));
        // The GET answers, and the load flushes the key it left dirty.
        server.gets.shift()!();
        await settings.whenLoaded();
        const second = settings.saveNow('jobsPanelScope', 'mine');
        await tick();
        // Nothing else for the key is sent while its first write is unanswered.
        expect(server.puts).toHaveLength(1);

        for (let turn = 0; turn < server.puts.length; turn += 1) {
            server.puts[turn].answer();
            await tick();
        }
        await Promise.all([first, second]);

        expect(server.mostInFlight()).toBe(1);
        // A write whose turn comes after a later choice sends that choice.
        expect(server.puts.map(put => put.value).slice(1)).toEqual(server.puts.slice(1).map(() => 'mine'));
        expect(server.puts.at(-1)!.value).toBe('mine');
    });

    test('a debounced write waits for an explicit one of the same key still in flight', async () => {
        vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
        try {
            const server = slowServer();
            const settings = await import('./userSettings.js');
            server.gets.shift()!();
            await vi.waitFor(() => expect(settings.isLoaded()).toBe(true));

            settings.saveNow('uiSettings', { a: 1 });
            await vi.waitFor(() => expect(server.puts).toHaveLength(1));
            settings.set('uiSettings', { a: 2 });
            await vi.advanceTimersByTimeAsync(1000);
            expect(server.puts.map(put => put.value)).toEqual([{ a: 1 }]);

            server.puts[0].answer();
            await vi.waitFor(() => expect(server.puts.map(put => put.value)).toEqual([{ a: 1 }, { a: 2 }]));
            server.puts[1].answer();
            expect(server.mostInFlight()).toBe(1);
        } finally {
            vi.useRealTimers();
        }
    });
});
