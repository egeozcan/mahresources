import { describe, expect, test } from 'vitest';
import {
    applyProgressFrame,
    formatAmount,
    formatBytes,
    formatEta,
    formatMetric,
    formatQuantity,
    formatRate,
    graphSeries,
    graphSummary,
    liveEtaText,
    liveRateText,
    mergeFetchedProgress,
    mergeLivePoint,
    sparklinePath,
} from './jobProgress.js';

describe('job progress formatting', () => {
    test('scales bytes and formats quantities by unit', () => {
        expect(formatBytes(0)).toBe('0 B');
        expect(formatBytes(1536)).toBe('1.5 KB');
        expect(formatBytes(12.3 * 1024 * 1024)).toBe('12.3 MB');
        expect(formatBytes(250 * 1024 * 1024)).toBe('250 MB');
        expect(formatQuantity(42, 'percent')).toBe('42%');
        expect(formatQuantity(1200, 'items')).toBe('1,200');
        expect(formatQuantity(3.25, 'frames')).toBe('3.3 frames');
        expect(formatQuantity(90, 'seconds')).toBe('2 min');
    });

    test('formats a rate, and none for a percent', () => {
        expect(formatRate(2.1 * 1024 * 1024, 'bytes')).toBe('2.1 MB/s');
        expect(formatRate(4.5, 'items')).toBe('4.5/s');
        expect(formatRate(12, 'rows')).toBe('12 rows/s');
        expect(formatRate(3, 'percent')).toBe('');
        expect(formatRate(undefined, 'bytes')).toBe('');
    });

    test('says "about" only for an estimated ETA', () => {
        const now = Date.parse('2026-09-25T10:00:00Z');
        expect(formatEta({ eta: '2026-09-25T10:00:14Z', etaEstimated: true }, now)).toBe('about 14 s left');
        expect(formatEta({ eta: '2026-09-25T10:03:00Z' }, now)).toBe('3 min left');
        expect(formatEta({ eta: '2026-09-25T09:59:00Z', etaEstimated: true }, now)).toBe('almost done');
        expect(formatEta({}, now)).toBe('');
    });

    test('formats an amount and a metric with its total', () => {
        expect(formatAmount({ completed: 1024, total: 4096, unit: 'bytes' })).toBe('1.0 KB of 4.0 KB');
        expect(formatAmount({ completed: 3, total: 12, unit: 'items' })).toBe('3 of 12 items');
        expect(formatAmount({ completed: 2048, unit: 'bytes' })).toBe('2.0 KB');
        expect(formatAmount({ completed: 40, total: 100, unit: 'percent' })).toBe('');
        expect(formatMetric({ key: 'segments', value: 12, total: 40, unit: 'items' })).toBe('12 of 40');
        expect(formatMetric({ key: 'downloaded', value: 5 * 1024 * 1024, unit: 'bytes' })).toBe('5.0 MB');
    });
});

describe('job progress graphs', () => {
    const job = {
        id: 'job-1',
        version: 3,
        progress: {
            unit: 'bytes',
            metrics: [
                { key: 'segments', label: 'Segments', value: 3, graph: true },
                { key: 'skipped', label: 'Skipped', value: 1 },
            ],
            series: {
                intervalMs: 1000,
                unit: 'bytes',
                points: [
                    { t: 0, c: 0, v: { segments: 1 } },
                    { t: 1000, c: 100, r: 100, v: { segments: 2 } },
                    { t: 2000, c: 400, r: 300, v: { segments: 3 } },
                ],
            },
        },
    };

    test('draws the speed and each graphed metric, never an ungraphed one', () => {
        const series = graphSeries(job);
        expect(series.map(s => s.key)).toEqual([':speed', 'segments']);
        expect(series[0].points).toEqual([{ t: 1000, v: 100 }, { t: 2000, v: 300 }]);
        expect(series[1].points).toHaveLength(3);
        expect(graphSeries({ progress: { ...job.progress, unit: 'percent', series: { ...job.progress.series, unit: 'percent' } } })
            .map(s => s.key)).toEqual(['segments']);
    });

    test('places points by time and scales from zero', () => {
        const path = sparklinePath([{ t: 0, v: 0 }, { t: 3000, v: 10 }, { t: 4000, v: 5 }], 120, 28);
        expect(path).toBe('M0.0 26.0 L90.0 2.0 L120.0 14.0');
        expect(sparklinePath([{ t: 0, v: 5 }], 120, 28)).toBe('M0 2.0 L120 2.0');
        expect(sparklinePath([], 120, 28)).toBe('');
    });

    test('summarizes a graph for a reader who cannot see it', () => {
        const [speed] = graphSeries(job);
        expect(graphSummary(speed)).toBe('Speed over 1 s: latest 300 B/s, peak 300 B/s, average 200 B/s');
    });

    test('a live frame replaces the snapshot and extends the series without touching the version', () => {
        const next = applyProgressFrame(job, {
            jobId: 'job-1', version: 3, intervalMs: 1000,
            progress: { completed: 700, total: 1000, unit: 'bytes', rate: 300 },
            point: { t: 3000, c: 700, r: 300, v: { segments: 4 } },
        });
        expect(next.version).toBe(3);
        expect(next.progress.completed).toBe(700);
        expect(next.progress.series.points).toHaveLength(4);
        expect(next.progress.series.unit).toBe('bytes');

        const replaced = applyProgressFrame(next, {
            jobId: 'job-1', version: 3, progress: { completed: 750 }, point: { t: 3000, c: 750 },
        });
        expect(replaced.progress.series.points).toHaveLength(4);
        expect(replaced.progress.series.points[3].c).toBe(750);

        expect(applyProgressFrame(job, { jobId: 'other', progress: {} })).toBe(job);
        expect(applyProgressFrame(job, { jobId: 'job-1', version: 2, progress: {} })).toBe(job);
    });

    test('a live series stays bounded and keeps where the Job started', () => {
        let series = { intervalMs: 1000, points: [] as Array<{ t: number, c: number }> };
        for (let t = 0; t < 1000; t++) series = mergeLivePoint(series, { t: t * 1000, c: t }, 1000);
        expect(series.points.length).toBeLessThanOrEqual(241);
        expect(series.points[0].t).toBe(0);
        expect(series.points[series.points.length - 1].t).toBe(999000);
    });
});

describe('job progress freshness', () => {
    test('a stalled Job stops showing a speed and an estimated time left', () => {
        const now = Date.parse('2026-09-25T10:00:30Z');
        const progress = {
            unit: 'bytes', rate: 2048, eta: '2026-09-25T10:01:00Z', etaEstimated: true,
            updatedAt: '2026-09-25T10:00:25Z',
        };
        expect(liveRateText(progress, now)).toBe('2.0 KB/s');
        expect(liveEtaText(progress, now)).toBe('about 30 s left');
        const stalled = { ...progress, updatedAt: '2026-09-25T10:00:10Z' };
        expect(liveRateText(stalled, now)).toBe('');
        expect(liveEtaText(stalled, now)).toBe('');
        // An executor's own ETA is its statement, not an estimate from a speed.
        expect(liveEtaText({ ...stalled, etaEstimated: false }, now)).toBe('30 s left');
    });

    test('a frame older than the snapshot the row holds is ignored', () => {
        const job = { id: 'j', version: 1, progress: { completed: 90, updatedAt: '2026-09-25T10:00:05Z' } };
        const stale = { jobId: 'j', version: 1, progress: { completed: 30, updatedAt: '2026-09-25T10:00:02Z' }, point: { t: 1, c: 30 } };
        expect(applyProgressFrame(job, stale)).toBe(job);
        const fresh = { jobId: 'j', version: 1, progress: { completed: 95, updatedAt: '2026-09-25T10:00:06Z' } };
        expect(applyProgressFrame(job, fresh).progress.completed).toBe(95);
    });
});

describe('fetched copies of a Job', () => {
    test('never roll back newer live progress, and keep the drawn series', () => {
        const held = { id: 'j', version: 2, progress: { completed: 90, updatedAt: '2026-09-25T10:00:09Z', series: { points: [{ t: 1 }] } } };
        const older = { id: 'j', version: 2, state: 'running', progress: { completed: 40, updatedAt: '2026-09-25T10:00:03Z' } };
        expect(mergeFetchedProgress(older, held).progress.completed).toBe(90);
        expect(mergeFetchedProgress(older, held).state).toBe('running');
        const newerNoSeries = { id: 'j', version: 3, progress: { completed: 95, updatedAt: '2026-09-25T10:00:10Z' } };
        const merged = mergeFetchedProgress(newerNoSeries, held);
        expect(merged.progress.completed).toBe(95);
        expect(merged.progress.series).toBe(held.progress.series);
        expect(mergeFetchedProgress(older, undefined)).toBe(older);
    });
});

describe('graph gaps and newer versions', () => {
    test('a pause breaks the speed line instead of bridging it', () => {
        const job = { progress: { unit: 'bytes', series: { unit: 'bytes', points: [
            { t: 0, c: 0 }, { t: 1000, c: 100, r: 100 }, { t: 2000, c: 200, r: 100 },
            { t: 600000, c: 300 }, { t: 601000, c: 400, r: 100 }, { t: 602000, c: 500, r: 100 },
        ] } } };
        const [speed] = graphSeries(job);
        expect(speed.points[0]).toEqual({ t: 1000, v: 100 });
        const path = sparklinePath(speed.points, 120, 28);
        expect(path.match(/M/g)).toHaveLength(2);
    });

    test('a newer version wins over an older-looking timestamp', () => {
        const held = { id: 'j', version: 2, progress: { completed: 90, updatedAt: '2026-09-25T10:00:30Z' } };
        const resumed = { id: 'j', version: 3, progress: { completed: 10, updatedAt: '2026-09-25T10:00:10Z' } };
        expect(mergeFetchedProgress(resumed, held).progress.completed).toBe(10);
        const frame = { jobId: 'j', version: 3, progress: { completed: 12, updatedAt: '2026-09-25T10:00:11Z' } };
        expect(applyProgressFrame(held, frame).progress.completed).toBe(12);
    });
});

describe('unit changes and key collisions', () => {
    test('a frame in a new unit ends the old rates', () => {
        const job = { id: 'j', version: 1, progress: { unit: 'bytes', series: { unit: 'bytes', points: [
            { t: 0, c: 0 }, { t: 1000, c: 100, r: 100, v: { rows: 1 } },
        ] } } };
        const next = applyProgressFrame(job, { jobId: 'j', version: 1, progress: { unit: 'items', completed: 1 }, point: { t: 2000, c: 1 } });
        expect(next.progress.series.unit).toBe('items');
        expect(next.progress.series.points[1]).toEqual({ t: 1000, v: { rows: 1 } });
        expect(graphSeries(next).map(s => s.key)).toEqual([]);
    });

    test('a graphed metric that changes unit in a frame loses its old values', () => {
        const job = { id: 'j', version: 1, progress: {
            metrics: [{ key: 'size', label: 'Size', value: 2048, unit: 'bytes', graph: true }],
            series: { points: [{ t: 0, v: { size: 1024 } }, { t: 1000, v: { size: 2048 } }] } } };
        const next = applyProgressFrame(job, { jobId: 'j', version: 1,
            progress: { metrics: [{ key: 'size', label: 'Size', value: 3, unit: 'items', graph: true }] },
            point: { t: 2000, v: { size: 3 } } });
        const [size] = graphSeries(next);
        expect(size.points).toEqual([{ t: 2000, v: 3 }]);
    });

    test('a metric keyed rate does not collide with the speed graph', () => {
        const job = { progress: { unit: 'bytes', metrics: [{ key: 'rate', label: 'Rate', value: 1, graph: true }],
            series: { unit: 'bytes', points: [{ t: 0, c: 0, v: { rate: 1 } }, { t: 1000, c: 10, r: 10, v: { rate: 2 } }] } } };
        expect(graphSeries(job).map(s => s.key)).toEqual([':speed', 'rate']);
    });
});
