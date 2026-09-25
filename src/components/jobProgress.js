// Formatting and graph helpers for a Job's progress: the figures the Jobs
// drawer and the detail page show beside the bar, and the small charts drawn
// from the progress series the server keeps for each Job.
//
// The server derives rate and ETA (progress.rate, progress.eta,
// progress.etaEstimated) so every surface agrees; this module only formats them.

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

// Client-side bound for a series extended by live frames. The server compacts
// its own copy to 120 points; the drawer can hold a little more between
// refetches before it thins what it has.
const MAX_LIVE_POINTS = 240;

function finite(value) {
    return typeof value === 'number' && Number.isFinite(value);
}

export function formatBytes(bytes) {
    if (!finite(bytes) || bytes < 0) return '';
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
        value /= 1024;
        unit += 1;
    }
    const digits = unit === 0 || value >= 100 ? 0 : 1;
    return `${value.toFixed(digits)} ${BYTE_UNITS[unit]}`;
}

function formatNumber(value) {
    if (!finite(value)) return '';
    const rounded = Math.abs(value) >= 100 || Number.isInteger(value) ? Math.round(value) : Math.round(value * 10) / 10;
    return rounded.toLocaleString('en-US');
}

export function formatDuration(seconds) {
    if (!finite(seconds) || seconds < 0) return '';
    if (seconds < 1) return 'under 1 s';
    if (seconds < 60) return `${Math.round(seconds)} s`;
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) return `${minutes} min`;
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    if (hours < 24) return rest ? `${hours} h ${rest} min` : `${hours} h`;
    const days = Math.floor(hours / 24);
    const hoursLeft = hours % 24;
    return hoursLeft ? `${days} d ${hoursLeft} h` : `${days} d`;
}

/** A value in its unit: bytes are scaled, percent gets a sign, others are counted. */
export function formatQuantity(value, unit = '') {
    if (!finite(value)) return '';
    switch (unit) {
    case 'bytes':
        return formatBytes(value);
    case 'percent':
        return `${formatNumber(value)}%`;
    case 'seconds':
        return formatDuration(value);
    case 'ms':
        return `${formatNumber(value)} ms`;
    case '':
    case 'items':
        return formatNumber(value);
    default:
        return `${formatNumber(value)} ${unit}`;
    }
}

/** A rate per second in its unit, or '' where a speed says nothing (percent). */
export function formatRate(rate, unit = '') {
    if (!finite(rate) || rate < 0 || unit === 'percent') return '';
    if (unit === 'bytes') return `${formatBytes(rate)}/s`;
    if (unit === '' || unit === 'items') return `${formatNumber(rate)}/s`;
    return `${formatNumber(rate)} ${unit}/s`;
}

// How long a speed stays true without a new report, matching the server's own
// rule. A stalled transfer sends no frame, so the last speed would otherwise
// stay on screen for as long as the page did.
const STALE_AFTER_MS = 10_000;

/** Whether a running Job's speed and estimated time left are still current. */
export function progressIsFresh(progress, now = Date.now()) {
    const updated = Date.parse(progress?.updatedAt || '');
    return !Number.isFinite(updated) || now - updated <= STALE_AFTER_MS;
}

/** The speed of a running Job, or '' once it has gone stale. */
export function liveRateText(progress, now = Date.now()) {
    return progressIsFresh(progress, now) ? formatRate(progress?.rate, progress?.unit) : '';
}

/** The time left of a running Job; an estimate is dropped once it has gone stale. */
export function liveEtaText(progress, now = Date.now()) {
    if (progress?.etaEstimated && !progressIsFresh(progress, now)) return '';
    return formatEta(progress, now);
}

/** "about 14 s left" for an estimate, "14 s left" for an executor's own ETA. */
export function formatEta(progress, now = Date.now()) {
    const eta = progress?.eta ? Date.parse(progress.eta) : NaN;
    if (!Number.isFinite(eta)) return '';
    const remaining = (eta - now) / 1000;
    if (remaining <= 0) return progress.etaEstimated ? 'almost done' : '';
    const text = `${formatDuration(remaining)} left`;
    return progress.etaEstimated ? `about ${text}` : text;
}

/** "12.3 MB of 40 MB", "3 of 12 items", or just the completed amount. */
export function formatAmount(progress) {
    const completed = progress?.completed;
    if (!finite(completed) || progress?.unit === 'percent') return '';
    const unit = progress.unit || '';
    const total = progress.total;
    if (finite(total) && total > 0) {
        if (unit === 'bytes') return `${formatBytes(completed)} of ${formatBytes(total)}`;
        const suffix = unit && unit !== 'items' ? ` ${unit}` : unit === 'items' ? ' items' : '';
        return `${formatNumber(completed)} of ${formatNumber(total)}${suffix}`;
    }
    if (unit === 'items') return `${formatNumber(completed)} items`;
    return formatQuantity(completed, unit);
}

/** One metric's display value, including its total when it has one. */
export function formatMetric(metric) {
    if (!metric) return '';
    const value = formatQuantity(metric.value, metric.unit || '');
    if (!finite(metric.total)) return value;
    if (metric.unit === 'bytes') return `${value} of ${formatBytes(metric.total)}`;
    const suffix = metric.unit && metric.unit !== 'items' ? ` ${metric.unit}` : '';
    return `${formatNumber(metric.value)} of ${formatNumber(metric.total)}${suffix}`;
}

/**
 * The series a Job's graphs draw: its speed when it counts something in a
 * unit other than percent, then each metric it asked to have graphed.
 */
export function graphSeries(job) {
    const progress = job?.progress || {};
    const points = progress.series?.points || [];
    if (points.length === 0) return [];
    const out = [];
    const unit = progress.series?.unit || progress.unit || '';
    // A point with no value stays in the series as a gap (v: null), so a pause
    // is drawn as a break in the line rather than bridged.
    if (unit !== 'percent' && points.some(point => finite(point.r))) {
        out.push({
            key: 'rate',
            label: 'Speed',
            unit,
            rate: true,
            points: trimGaps(points.map(point => ({ t: point.t, v: finite(point.r) ? point.r : null }))),
        });
    }
    for (const metric of progress.metrics || []) {
        if (!metric.graph) continue;
        const values = points.map(point => ({ t: point.t, v: finite(point?.v?.[metric.key]) ? point.v[metric.key] : null }));
        if (!values.some(point => finite(point.v))) continue;
        out.push({ key: metric.key, label: metric.label || metric.key, unit: metric.unit || '', rate: false, points: trimGaps(values) });
    }
    return out;
}

// Drops the gaps before the first value and after the last, which carry no
// information: the very first point never has a rate.
function trimGaps(points) {
    const first = points.findIndex(point => finite(point.v));
    let last = points.length - 1;
    while (last >= 0 && !finite(points[last].v)) last -= 1;
    return first < 0 ? [] : points.slice(first, last + 1);
}

function seriesValue(series, value) {
    return series.rate ? formatRate(value, series.unit) : formatQuantity(value, series.unit);
}

/**
 * An SVG path for a sparkline in a width × height box. x is placed by time, so
 * a pause shows as a gap in the line's slope rather than being squeezed out.
 * The vertical scale starts at zero so a steady rate reads as steady.
 */
export function sparklinePath(points, width = 120, height = 32) {
    const timed = (points || []).filter(point => finite(point?.t));
    const usable = timed.filter(point => finite(point.v));
    if (usable.length === 0) return '';
    const first = usable[0].t;
    const last = usable[usable.length - 1].t;
    const span = last - first;
    const peak = Math.max(...usable.map(point => point.v));
    const pad = 2;
    const x = t => (span > 0 ? ((t - first) / span) * width : width);
    const y = v => (peak > 0 ? height - pad - (v / peak) * (height - pad * 2) : height - pad);
    if (usable.length === 1) {
        const py = y(usable[0].v).toFixed(1);
        return `M0 ${py} L${width} ${py}`;
    }
    // A gap lifts the pen: the next value starts a new segment.
    const parts = [];
    let penDown = false;
    for (const point of timed) {
        if (!finite(point.v)) {
            penDown = false;
            continue;
        }
        parts.push(`${penDown ? 'L' : 'M'}${x(point.t).toFixed(1)} ${y(point.v).toFixed(1)}`);
        penDown = true;
    }
    return parts.join(' ');
}

/** The accessible summary of one graph: its span, latest, peak and average. */
export function graphSummary(series) {
    const values = (series?.points || []).map(point => point.v).filter(finite);
    if (values.length === 0) return `${series?.label || 'Graph'}: no samples yet`;
    const points = series.points.filter(point => finite(point.v));
    const span = (points[points.length - 1].t - points[0].t) / 1000;
    const latest = values[values.length - 1];
    const peak = Math.max(...values);
    const average = values.reduce((sum, value) => sum + value, 0) / values.length;
    const over = span >= 1 ? ` over ${formatDuration(span)}` : '';
    return `${series.label}${over}: latest ${seriesValue(series, latest)}, peak ${seriesValue(series, peak)}, average ${seriesValue(series, average)}`;
}

/** Latest value of a series, formatted, for the visible caption beside a graph. */
export function graphLatest(series) {
    const points = (series?.points || []).filter(point => finite(point.v));
    if (points.length === 0) return '';
    return seriesValue(series, points[points.length - 1].v);
}

/**
 * Extends a series with the latest point a live frame carried. A point with
 * the same timestamp as the last one replaces it; an older one is ignored.
 */
export function mergeLivePoint(series, point, intervalMs) {
    if (!point || !finite(point.t)) return series;
    const current = series || { intervalMs: intervalMs || 1000, points: [] };
    const points = [...(current.points || [])];
    const last = points[points.length - 1];
    if (last && point.t < last.t) return current;
    if (last && point.t === last.t) points[points.length - 1] = point;
    else points.push(point);
    let thinned = points;
    if (thinned.length > MAX_LIVE_POINTS) {
        // Keep the first point, where the Job started, and every other one after.
        thinned = [points[0], ...points.slice(1).filter((_, index) => index % 2 === 1)];
        if (thinned[thinned.length - 1] !== points[points.length - 1]) thinned.push(points[points.length - 1]);
    }
    return { ...current, intervalMs: intervalMs || current.intervalMs, points: thinned };
}

/**
 * Reconciles a fetched copy of a Job with the one already held. Progress ticks
 * do not move a Job's version, so a list or detail response that was issued
 * before the latest live frame can carry an older snapshot at the same
 * version; the held progress wins then. A response that carries no series
 * (a command's answer, a plain listing) keeps the series already drawn.
 */
export function mergeFetchedProgress(next, previous) {
    if (!next || !previous || next.id !== previous.id || next === previous) return next;
    const incoming = next.progress || {};
    const held = previous.progress || {};
    // A newer version is a newer execution or transition: its progress wins
    // whatever its timestamp says, since each process stamps its own clock.
    if (Number(next.version || 0) > Number(previous.version || 0)) {
        return incoming.series || !held.series ? next : { ...next, progress: { ...incoming, series: held.series } };
    }
    const incomingAt = Date.parse(incoming.updatedAt || '');
    const heldAt = Date.parse(held.updatedAt || '');
    if (Number.isFinite(incomingAt) && Number.isFinite(heldAt) && incomingAt < heldAt) {
        return { ...next, progress: held };
    }
    if (!incoming.series && held.series) return { ...next, progress: { ...incoming, series: held.series } };
    return next;
}

/**
 * Applies one job-progress frame to a listed Job. The frame replaces the
 * progress snapshot and extends the series the row already holds; it never
 * changes the Job's version or state, which only lifecycle events move.
 */
export function applyProgressFrame(job, frame) {
    if (!job || !frame || job.id !== frame.jobId) return job;
    if (Number(frame.version || 0) < Number(job.version || 0)) return job;
    const previous = job.progress || {};
    // The same version can still carry an older snapshot: the stream starts a
    // little in the past, so a frame can arrive after a fetch that was newer.
    const incomingAt = Date.parse(frame.progress?.updatedAt || '');
    const currentAt = Date.parse(previous.updatedAt || '');
    const sameVersion = Number(frame.version || 0) === Number(job.version || 0);
    if (sameVersion && Number.isFinite(incomingAt) && Number.isFinite(currentAt) && incomingAt < currentAt) return job;
    const series = frame.point
        ? mergeLivePoint(previous.series, frame.point, frame.intervalMs)
        : previous.series;
    return { ...job, progress: { ...frame.progress, series } };
}
