import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import {
    applyProgressFrame,
    formatAmount,
    mergeFetchedProgress,
    liveEtaText,
    liveRateText,
    formatMetric,
    formatRate,
    graphLatest,
    graphSeries,
    graphSummary,
    sparklinePath,
} from './jobProgress.js';

export const JOB_STATES = Object.freeze([
    'scheduled', 'queued', 'running', 'paused', 'blocked',
    'succeeded', 'failed', 'cancelled', 'interrupted',
]);
const ACTIVE_STATES = ['scheduled', 'queued', 'running', 'paused'];
const ATTENTION_STATES = ['blocked', 'failed', 'interrupted'];

export function advertisedCommands(job) {
    return Array.isArray(job?.commands) ? job.commands : [];
}

export function jobCommands(job) {
    return advertisedCommands(job).filter(command => {
        if (command?.key === 'pin') return job?.pinned !== true;
        if (command?.key === 'unpin') return job?.pinned === true;
        return true;
    });
}

export function advertisedOutputs(job) {
    return Array.isArray(job?.outputs) ? job.outputs : [];
}

export function warningEvents(events) {
    return (Array.isArray(events) ? events : []).filter(event =>
        event?.type === 'warning' || event?.type === 'events-truncated',
    );
}

export function commandLabel(command) {
    return command?.label || command?.key || 'Run command';
}

export function commandEndpoint(job, command) {
    return command?.endpoint || `/v1/jobs/${encodeURIComponent(job?.id || '')}/commands/${encodeURIComponent(command?.key || '')}`;
}

export function outputEndpoint(output) {
    return output?.url || output?.endpoint || output?.href || '';
}

function hasEntityCompanion(output, outputs) {
    return Array.isArray(outputs) && outputs.some(candidate =>
        candidate !== output && candidate?.key === 'entity' && candidate?.type === 'entity' && candidate?.availability === 'available',
    );
}

export function outputLinkURL(output, outputs = []) {
    if (output?.type === 'summary' && output.destinationUrl && !hasEntityCompanion(output, outputs)) {
        return output.destinationUrl;
    }
    return outputEndpoint(output);
}

export function outputJSONLinkURL(output, outputs = []) {
    if (output?.type === 'summary' && output.destinationUrl && !hasEntityCompanion(output, outputs)) {
        return outputEndpoint(output);
    }
    return '';
}

export function outputLinkLabel(output, outputs = []) {
    if (output?.type === 'entity') {
        const label = String(output.label || '').trim().toLowerCase();
        return `View ${label || 'entity'}`;
    }
    if (output?.type === 'summary') {
        if (hasEntityCompanion(output, outputs)) return 'View JSON result';
        if (output.destinationUrl) return 'View result';
    }
    if (output?.type === 'log') return 'Open log';
    return 'Open output';
}

export function outputLinkAccessibleLabel(output, outputs = []) {
    const visibleLabel = outputLinkLabel(output, outputs);
    if (output?.type === 'entity' || (output?.type === 'summary' && (output.destinationUrl || hasEntityCompanion(output, outputs)))) return visibleLabel;
    const name = String(output?.label || output?.key || '').trim();
    if (!name) return visibleLabel;
    return output?.type === 'log' ? `Open log ${name}` : `Open ${name}`;
}

function safeResultURL(value) {
    if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//') ||
        /[\\\u0000-\u001f\u007f]/.test(value)) return '';
    const origin = globalThis.location?.origin || 'http://localhost';
    try {
        const parsed = new URL(value, origin);
        if (parsed.origin !== origin || parsed.username || parsed.password || parsed.hash) return '';
        return value;
    } catch {
        return '';
    }
}

// The one link a finished job offers straight from a list: what it made. Any
// succeeded job's available entity output is that — the Resource a download
// created, the entity a plugin action returned. The endpoint it names redirects to
// the entity after checking the viewer may open it. Only a plugin action records
// a summary destination instead (historical result.redirect values), so that
// fallback stays with that kind.
export function resultOutput(job) {
    if (job?.state !== 'succeeded') return null;
    const outputs = advertisedOutputs(job);
    const entity = outputs.find(output => output?.type === 'entity' &&
        output.availability === 'available' &&
        safeResultURL(outputLinkURL(output, outputs)));
    if (entity) return entity;
    if (job.kind !== 'plugin-action') return null;
    return outputs.find(output => {
        if (output?.type !== 'summary' || output.availability !== 'available' || !output.destinationUrl) return false;
        const url = outputLinkURL(output, outputs);
        return url === output.destinationUrl && Boolean(safeResultURL(url));
    }) || null;
}

export function resultURL(job) {
    const output = resultOutput(job);
    return output ? safeResultURL(outputLinkURL(output, advertisedOutputs(job))) : '';
}

export function resultLinkLabel(job) {
    const output = resultOutput(job);
    return output ? outputLinkLabel(output, advertisedOutputs(job)) : '';
}

export function resultAccessibleLabel(job) {
    const output = resultOutput(job);
    if (!output) return '';
    const label = outputLinkAccessibleLabel(output, advertisedOutputs(job));
    const context = String(job?.title || job?.kind || job?.id || '').trim();
    return context ? `${label} for ${context}` : label;
}

export function stateOf(job) {
    return String(job?.state || 'unknown').toLowerCase();
}

// A succeeded job whose Kind recorded the `partial` phase stopped short of
// finished and may offer Continue; its state is still succeeded. The /jobs card
// (job_template_context.go jobStateLabel) says the same.
export function isPartialSuccess(job) {
    return stateOf(job) === 'succeeded' && job?.phase === 'partial';
}

export function stateLabel(job) {
    const state = stateOf(job);
    if (!state) return 'Unknown';
    if (isPartialSuccess(job)) return 'Partially completed';
    return state.charAt(0).toUpperCase() + state.slice(1).replaceAll('-', ' ');
}

// The phase shown beside the state, or nothing when the state label already
// says it: a partial success's label is its phase.
export function phaseText(job) {
    return isPartialSuccess(job) ? '' : String(job?.phase || '');
}

export function classifyJobState(jobOrState) {
    const state = typeof jobOrState === 'string' ? jobOrState.toLowerCase() : stateOf(jobOrState);
    if (ATTENTION_STATES.includes(state)) return 'attention';
    if (ACTIVE_STATES.includes(state)) return 'active';
    if (JOB_STATES.includes(state)) return 'finished';
    return 'other';
}

export function selectedBulkCommands(jobs, selectedIds) {
    const selected = new Set(selectedIds || []);
    const chosenJobs = (jobs || []).filter(job => selected.has(job.id));
    if (chosenJobs.length === 0 || chosenJobs.length !== selected.size) return [];
    const commandMaps = chosenJobs.map(job => new Map(
        advertisedCommands(job).filter(command => command.bulk).map(command => [command.key, command]),
    ));
    const hasPinned = chosenJobs.some(job => job.pinned === true);
    const hasUnpinned = chosenJobs.some(job => job.pinned !== true);
    return advertisedCommands(chosenJobs[0])
        .filter(command => command.bulk && commandMaps.every(commands => commands.has(command.key)))
        .filter(command => {
            if (command.key === 'pin' && hasPinned && !hasUnpinned) return false;
            if (command.key === 'unpin' && hasUnpinned && !hasPinned) return false;
            return true;
        });
}

function announcementFor(job, previous, replay) {
    if (replay || !previous || stateOf(job) === stateOf(previous)) return '';
    return `${job.title || job.kind || 'Job'} ${stateLabel(job).toLowerCase()}.`;
}

function eventJob(message) {
    if (message?.job && typeof message.job === 'object') return message.job;
    if (message?.snapshot && typeof message.snapshot === 'object') return message.snapshot;
    if (message?.id && message?.state) return message;
    return null;
}

export function reduceJobStreamEvent(jobs, message, lastSequence = 0, options = {}) {
    const parsedSequence = streamSequence(message?.deliverySequence ?? message?.delivery_sequence ?? message?.lastEventId ?? 0);
    const sequence = Number.isFinite(parsedSequence) ? parsedSequence : 0;
    if (sequence > 0 && sequence <= Number(lastSequence || 0)) {
        return { jobs, lastSequence: Number(lastSequence || 0), changed: false, announcement: '', previousClass: '', nextClass: '' };
    }

    const incoming = eventJob(message);
    const nextSequence = Math.max(Number(lastSequence || 0), sequence);
    if (!incoming?.id) {
        const id = message?.jobId || message?.jobID || '';
        return {
            jobs,
            lastSequence: nextSequence,
            changed: !!id,
            needsSnapshot: !!id,
            jobId: id,
            announcement: '',
            previousClass: '',
            nextClass: '',
        };
    }

    return reduceJobSnapshot(jobs, incoming, message?.replay === true, nextSequence, options);
}

export function reduceJobSnapshot(jobs, incoming, replay = false, lastSequence = 0, options = {}) {
    if (!incoming?.id) {
        return { jobs, lastSequence, changed: false, announcement: '', previousClass: '', nextClass: '' };
    }
    const index = jobs.findIndex(job => job.id === incoming.id);
    const current = index >= 0 ? jobs[index] : null;
    if (!current && options.allowInsert !== true) {
        return {
            jobs,
            lastSequence,
            changed: false,
            ignored: true,
            jobId: incoming.id,
            announcement: '',
            previousClass: '',
            nextClass: '',
        };
    }
    const incomingVersion = Number(incoming.version || 0);
    const currentVersion = Number(current?.version || 0);
    if (current && incomingVersion > 0 && incomingVersion < currentVersion) {
        return { jobs, lastSequence, changed: false, announcement: '', previousClass: '', nextClass: '' };
    }

    const merged = {
        ...(current || {}),
        ...incoming,
        uiExpanded: current?.uiExpanded ?? incoming.uiExpanded ?? false,
        uiSelected: current?.uiSelected ?? incoming.uiSelected ?? false,
    };
    const previousClass = current ? classifyJobState(current) : '';
    const nextClass = classifyJobState(merged);
    const nextJobs = [...jobs];
    if (index >= 0) nextJobs[index] = merged;
    else nextJobs.push(merged);
    nextJobs.sort((left, right) => String(right.acceptedAt || '').localeCompare(String(left.acceptedAt || '')) || String(right.id).localeCompare(String(left.id)));

    return {
        jobs: nextJobs,
        lastSequence,
        changed: true,
        announcement: announcementFor(merged, current, replay),
        previousClass,
        nextClass,
    };
}

function streamSequence(value) {
    const raw = String(value || '').replace(/^v2:/, '');
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? parsed : 0;
}

export function streamCursorSequence(value) {
    const cursor = String(value || '');
    if (!/^v2:\d+$/.test(cursor)) return null;
    const sequence = Number(cursor.slice(3));
    return Number.isFinite(sequence) ? sequence : null;
}

// A succeeded job is complete whatever its last progress row says. Producers
// publish progress while they work and none rewrites it on the way out, so a
// finished job keeps whatever was current when the work ended: a plugin action's
// last percent, or — for a download whose size was never known (no
// Content-Length, an HLS stream) — no total at all, which reads as indeterminate
// and would keep pulsing "In progress" on a download that finished long ago.
function progressSupersededBySuccess(job) {
    if (job?.state !== 'succeeded') return false;
    // A partial success always reads as partial, whatever its last row says.
    if (isPartialSuccess(job)) return true;
    const progress = job?.progress || {};
    const completed = progress.completed;
    const total = progress.total;
    return !(Number.isFinite(completed) && Number.isFinite(total) && total > 0 && completed >= total);
}

export function progressText(job) {
    if (isPartialSuccess(job)) {
        // The bar says what the badge says, keeping the run's last message:
        // it is usually what says how much is left.
        const message = job?.progress?.message;
        return message ? `Partially completed: ${message}` : 'Partially completed';
    }
    if (progressSupersededBySuccess(job)) return 'Completed';
    const progress = job?.progress || {};
    if (progress.message) return progress.message;
    const completed = progress.completed;
    const total = progress.total;
    if (completed !== null && completed !== undefined && total > 0) {
        return `${completed} / ${total}${progress.unit ? ` ${progress.unit}` : ''}`;
    }
    return progress.phase || (completed !== null && completed !== undefined ? String(completed) : 'Working');
}

export function progressValue(job) {
    if (progressSupersededBySuccess(job)) return 100;
    const progress = job?.progress || {};
    if (progress.completed === null || progress.completed === undefined || !(progress.total > 0)) return null;
    return Math.max(0, Math.min(100, Math.round((progress.completed / progress.total) * 100)));
}

// Whether the progress bar may animate and read "In progress". An unknown total
// on a job that has stopped — failed, cancelled, interrupted, blocked — stays
// unknown, but nothing is working on it any more.
export function progressIndeterminate(job) {
    return progressValue(job) === null && classifyJobState(job) === 'active';
}

export function progressAccessibleText(job) {
    const label = progressText(job);
    if (progressValue(job) !== null) return label;
    const progress = job?.progress || {};
    const completed = progress.completed;
    if (completed !== null && completed !== undefined) {
        const amount = `${completed}${progress.unit ? ` ${progress.unit}` : ''} processed`;
        return `${progress.message || progress.phase ? `${label}; ` : ''}${amount}; total unknown`;
    }
    return `${label}; total unknown`;
}

function safeJSON(response) {
    return response.json().catch(() => ({}));
}

function idempotencyKey() {
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `job-command-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

/**
 * The /job detail page. The /jobs list is server-rendered (listJobs.tpl) and has
 * its own small component in jobList.js; this one reads one Job, its timeline,
 * commands and outputs, and follows it on the canonical stream.
 */
export function jobCenter(options = {}) {
    return {
        detailId: options.detailId || '',
        jobs: [],
        details: {},
        timeline: [],
        timelineError: '',
        detail: null,
        loading: true,
        error: '',
        notice: '',
        connectionStatus: 'disconnected',
        eventSource: null,
        lastSequence: 0,
        streamCaughtUp: false,
        now: Date.now(),
        _clockTimer: null,
        _liveRegion: null,

        init() {
            if (!this.detailId) {
                this.detailId = new URLSearchParams(globalThis.location?.search || '').get('id') || '';
            }
            this._liveRegion = createLiveRegion();
            // Keeps "about 14 s left" counting down between progress frames.
            this._clockTimer = setInterval(() => { this.now = Date.now(); }, 1000);
            this.connect();
            this.load();
        },

        destroy() {
            if (this._clockTimer) clearInterval(this._clockTimer);
            this.eventSource?.close();
            this._liveRegion?.destroy();
        },

        async fetchJSON(url, init = {}) {
            const response = await fetch(url, {
                ...init,
                headers: { Accept: 'application/json', ...(init.headers || {}) },
            });
            const payload = await safeJSON(response);
            if (!response.ok) {
                const error = new Error(payload.error || `Request failed (${response.status})`);
                error.status = response.status;
                error.payload = payload;
                throw error;
            }
            return payload;
        },

        async load() {
            this.loading = true;
            this.error = '';
            try {
                await this.loadDetail(this.detailId);
            } catch (error) {
                this.error = error.message || 'Could not load this job.';
            } finally {
                this.loading = false;
            }
        },

        async loadDetail(id) {
            if (!id) throw new Error('A job ID is required.');
            const payload = await this.fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            this.detail = payload.job || payload;
            this.timelineError = '';
            this.timeline = [];
            if (this.detail?.id) {
                try {
                    const timelinePayload = await this.fetchJSON(`/v1/jobs/${encodeURIComponent(id)}/events?limit=100`);
                    this.timeline = timelinePayload.events || [];
                } catch (error) {
                    this.timelineError = error.message || 'Timeline is unavailable.';
                }
            }
            this.details[id] = this.detail;
            this.jobs = this.detail ? [this.detail] : [];
            return this.detail;
        },

        async refreshJobPreference(id) {
            const payload = await this.fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            const freshJob = payload.job || payload;
            if (freshJob?.id) this.updateJob(freshJob);
            return freshJob;
        },

        detailURL(job) {
            return `/job?id=${encodeURIComponent(job?.id || '')}`;
        },

        async runCommand(job, command) {
            const confirmation = commandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(confirmation, {
                    title: commandLabel(command),
                    confirmLabel: commandLabel(command),
                });
                if (!accepted) return null;
            }
            const key = idempotencyKey();
            this.notice = '';
            try {
                const payload = await this.fetchJSON(commandEndpoint(job, command), {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                    body: JSON.stringify({ expectedVersion: command.jobVersion ?? job.version, idempotencyKey: key }),
                });
                const outcome = payload.result || payload;
                const freshJob = outcome.job || payload.job;
                if (freshJob?.id) this.updateJob(freshJob);
                let preferenceRefreshFailed = false;
                if (command?.key === 'pin' || command?.key === 'unpin') {
                    try {
                        await this.refreshJobPreference(job.id);
                    } catch {
                        preferenceRefreshFailed = true;
                    }
                }
                const successorId = outcome.successorId || outcome.successorID || payload.successorId || payload.successorID;
                if (successorId) globalThis.location?.assign?.(`/job?id=${encodeURIComponent(successorId)}`);
                this.notice = preferenceRefreshFailed
                    ? `${commandLabel(command)} completed. Reload this job to see its current pin status.`
                    : outcome.message || payload.message || `${commandLabel(command)} requested.`;
                this._liveRegion?.announce(this.notice);
                return outcome;
            } catch (error) {
                if (error.status === 409) {
                    const fresh = error.payload?.job || error.payload?.snapshot;
                    if (fresh?.id) this.updateJob(fresh);
                    this.notice = error.message || 'This job changed. The latest details are shown.';
                } else this.notice = error.message || 'The command could not be completed.';
                this._liveRegion?.announce(this.notice);
                return null;
            }
        },

        connect() {
            if (this.eventSource || typeof EventSource === 'undefined') return;
            this.connectionStatus = 'connecting';
            this.eventSource = new EventSource('/v1/jobs/events?version=2');
            this.eventSource.addEventListener('open', () => { this.connectionStatus = 'connected'; });
            this.eventSource.addEventListener('error', () => {
                this.connectionStatus = 'reconnecting';
                this.streamCaughtUp = false;
            });
            this.eventSource.addEventListener('job-caught-up', event => this.markStreamCaughtUp(event));
            this.eventSource.addEventListener('job-progress', event => this.handleProgressFrame(event));
            for (const eventName of ['message', 'job']) {
                this.eventSource.addEventListener(eventName, event => this.handleStreamMessage(event));
            }
        },

        // Live progress for the Job on this page: the snapshot is replaced and
        // the graph extended in place. It is never announced; lifecycle events
        // are what the live region is for.
        handleProgressFrame(event) {
            let frame;
            try { frame = JSON.parse(event.data); }
            catch { return; }
            if (!frame?.jobId || this.detail?.id !== frame.jobId) return;
            const next = applyProgressFrame(this.detail, frame);
            if (next === this.detail) return;
            this.detail = next;
            this.details[next.id] = next;
            this.jobs = this.jobs.map(job => job.id === next.id ? next : job);
        },

        markStreamCaughtUp(event) {
            let boundary;
            try { boundary = JSON.parse(event.data); }
            catch { return; }
            const sequence = streamCursorSequence(boundary?.cursor);
            if (sequence === null) return;
            this.lastSequence = Math.max(this.lastSequence, sequence);
            this.streamCaughtUp = true;
        },

        handleStreamMessage(event) {
            let message;
            try { message = JSON.parse(event.data); }
            catch { return; }
            if (!message.deliverySequence && event.lastEventId) message.lastEventId = event.lastEventId;
            message.replay = message.replay === true || !this.streamCaughtUp;
            const announceSnapshot = !message.replay;
            const result = reduceJobStreamEvent(this.jobs, message, this.lastSequence);
            this.lastSequence = result.lastSequence;
            if (!result.changed) {
                return;
            }
            if (result.needsSnapshot) {
                if (!this.jobs.some(job => job.id === result.jobId)) return;
                this.fetchJSON(`/v1/jobs/${encodeURIComponent(result.jobId)}`)
                    .then(payload => {
                        const snapshot = payload.job || payload;
                        if (this.jobs.some(job => job.id === snapshot.id)) this.applyStreamSnapshot(snapshot, null, announceSnapshot);
                    })
                    .catch(() => {});
                return;
            }
            this.jobs = result.jobs;
            if (result.announcement) this._liveRegion?.announce(result.announcement);
        },

        updateJob(job) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence);
            this.applyStreamSnapshot(job, result);
        },

        applyStreamSnapshot(job, previousResult = null, announce = false) {
            if (!job?.id) return;
            const result = previousResult || reduceJobStreamEvent(this.jobs, { job }, this.lastSequence);
            const held = new Map(this.jobs.map(current => [current.id, current]));
            if (result.changed) this.jobs = result.jobs.map(next => mergeFetchedProgress(next, held.get(next.id)));
            this.details[job.id] = mergeFetchedProgress({ ...(this.details[job.id] || {}), ...job }, this.details[job.id]);
            if (this.detail?.id === job.id) this.detail = mergeFetchedProgress({ ...this.detail, ...job }, this.detail);
            if (announce && result.announcement) this._liveRegion?.announce(result.announcement);
        },

        progressText(job) { return progressText(job); },
        amountText(job) { return formatAmount(job?.progress); },
        rateText(job) {
            const progress = job?.progress || {};
            if (job?.state === 'running') return liveRateText(progress, this.now);
            const average = formatRate(progress.averageRate, progress.unit);
            return average ? `average ${average}` : '';
        },
        etaText(job) { return job?.state === 'running' ? liveEtaText(job?.progress, this.now) : ''; },
        statsText(job) {
            return [this.amountText(job), this.rateText(job), this.etaText(job)].filter(Boolean).join(' · ');
        },
        metricsFor(job) { return job?.progress?.metrics || []; },
        metricText(metric) { return formatMetric(metric); },
        graphsFor(job) { return graphSeries(job); },
        sparkline(series) { return sparklinePath(series.points, 480, 80); },
        graphLabel(series) { return graphSummary(series); },
        graphLatest(series) { return graphLatest(series); },
        progressValue(job) { return progressValue(job); },
        progressAccessibleText(job) { return progressAccessibleText(job); },
        progressIndeterminate(job) { return progressIndeterminate(job); },
        stateLabel(job) { return stateLabel(job); },
        phaseText(job) { return phaseText(job); },
        stateClass(job) { return classifyJobState(job); },
        commandLabel(command) { return commandLabel(command); },
        advertisedCommands(job) { return advertisedCommands(this.details[job.id] || job); },
        commandsFor(job) { return jobCommands(this.details[job.id] || job); },
        advertisedOutputs(job) { return advertisedOutputs(job); },
        warningEvents() { return warningEvents(this.timeline); },
        outputEndpoint(output) { return outputEndpoint(output); },
        outputLinkURL(output, outputs) { return outputLinkURL(output, outputs); },
        outputJSONLinkURL(output, outputs) { return outputJSONLinkURL(output, outputs); },
        outputLinkLabel(output, outputs) { return outputLinkLabel(output, outputs); },
        outputLinkAccessibleLabel(output, outputs) { return outputLinkAccessibleLabel(output, outputs); },
    };
}

export function commandConfirmation(command) {
    if (command?.key === 'pin') {
        return "Pin this job's metadata and event history against ordinary retention. Linked jobs and artifacts keep their own retention.";
    }
    if (command?.key === 'pin-lineage') {
        return 'Pin this job and each related job you can see against ordinary retention. Artifacts keep their own retention.';
    }
    if (command?.key === 'forget') {
        return 'Forget this job’s saved replay input. Retry, Continue and Repeat will no longer be possible. Its sanitized history remains, and its outputs and artifacts are not affected. This cannot be undone.';
    }
    if (command?.confirmation) return command.confirmation;
    if (command?.destructive) return `Run ${commandLabel(command)}?`;
    return '';
}
