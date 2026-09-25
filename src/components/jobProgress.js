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
    if (unit !== 'percent' && points.some(point => finite(point.r))) {
        out.push({
            key: 'rate',
            label: 'Speed',
            unit,
            rate: true,
            points: points.filter(point => finite(point.r)).map(point => ({ t: point.t, v: point.r })),
        });
    }
    for (const metric of progress.metrics || []) {
        if (!metric.graph) continue;
        const values = points.filter(point => finite(point?.v?.[metric.key])).map(point => ({ t: point.t, v: point.v[metric.key] }));
        if (values.length === 0) continue;
        out.push({ key: metric.key, label: metric.label || metric.key, unit: metric.unit || '', rate: false, points: values });
    }
    return out;
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
    const usable = (points || []).filter(point => finite(point?.t) && finite(point?.v));
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
    return usable.map((point, index) => `${index === 0 ? 'M' : 'L'}${x(point.t).toFixed(1)} ${y(point.v).toFixed(1)}`).join(' ');
}

/** The accessible summary of one graph: its span, latest, peak and average. */
export function graphSummary(series) {
    const values = (series?.points || []).map(point => point.v).filter(finite);
    if (values.length === 0) return `${series?.label || 'Graph'}: no samples yet`;
    const points = series.points;
    const span = (points[points.length - 1].t - points[0].t) / 1000;
    const latest = values[values.length - 1];
    const peak = Math.max(...values);
    const average = values.reduce((sum, value) => sum + value, 0) / values.length;
    const over = span >= 1 ? ` over ${formatDuration(span)}` : '';
    return `${series.label}${over}: latest ${seriesValue(series, latest)}, peak ${seriesValue(series, peak)}, average ${seriesValue(series, average)}`;
}

/** Latest value of a series, formatted, for the visible caption beside a graph. */
export function graphLatest(series) {
    const points = series?.points || [];
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
 * Applies one job-progress frame to a listed Job. The frame replaces the
 * progress snapshot and extends the series the row already holds; it never
 * changes the Job's version or state, which only lifecycle events move.
 */
export function applyProgressFrame(job, frame) {
    if (!job || !frame || job.id !== frame.jobId) return job;
    if (Number(frame.version || 0) < Number(job.version || 0)) return job;
    const previous = job.progress || {};
    const series = frame.point
        ? mergeLivePoint(previous.series, frame.point, frame.intervalMs)
        : previous.series;
    return { ...job, progress: { ...frame.progress, series } };
}
