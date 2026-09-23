import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { captureTrigger, focusedElement, focusFirstIn, restoreFocus } from '../utils/focus.js';
import { blockingModal, isRendered } from '../utils/modality.js';
import {
    advertisedCommands,
    classifyJobState,
    commandEndpoint,
    commandLabel,
    reduceJobStreamEvent,
    stateLabel,
    streamCursorSequence,
} from './jobCenter.js';

const PANEL_LIMIT = 5;
const PANEL_REFRESH_MAX_WAIT_MS = 500;
const PANEL_STATE_FILTERS = [
    ['blocked', 'failed', 'interrupted'],
    ['scheduled', 'queued', 'running', 'paused'],
    ['succeeded', 'cancelled'],
];

export function panelCounts(summary) {
    const byState = summary?.byState || {};
    return {
        active: ['scheduled', 'queued', 'running', 'paused'].reduce((count, state) => count + Number(byState[state] || 0), 0),
        attention: ['blocked', 'failed', 'interrupted'].reduce((count, state) => count + Number(byState[state] || 0), 0),
    };
}

export function panelCommandConfirmation(command) {
    if (command?.key === 'dismiss') {
        return 'Dismiss this job from your default list. Its history and outputs remain available, and any artifacts keep their own retention.';
    }
    if (command?.key === 'pin') {
        return "Pin this job's metadata and event history against ordinary retention. Linked jobs and artifacts keep their own retention.";
    }
    if (command?.key === 'pin-lineage') {
        return 'Pin this job and each related job you can see against ordinary retention. Artifacts keep their own retention.';
    }
    if (command?.key === 'forget') {
        return 'Forget this job’s saved replay input. Its sanitized history remains, and its outputs and artifacts are not affected. This cannot be undone.';
    }
    if (command?.confirmation) return command.confirmation;
    if (command?.destructive) return `Run ${commandLabel(command)}? ${command.confirmation || ''}`.trim();
    return '';
}

function commandKey() {
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `job-panel-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function jobPanel() {
    return {
        isOpen: false,
        jobs: [],
        details: {},
        summary: null,
        eventSource: null,
        lastSequence: 0,
        streamCaughtUp: false,
        connectionStatus: 'disconnected',
        error: '',
        notice: '',
        outcomes: [],
        busy: false,
        _liveRegion: null,
        _trigger: null,
        _lastTrigger: null,
        _root: null,
        _keydownHandler: null,
        _panelOpenHandler: null,
        _panelRefreshTimer: null,
        _panelRefreshMaxTimer: null,
        _panelRefreshRequested: false,
        _panelRefreshPromise: null,
        _refreshGeneration: 0,
        _streamGeneration: 0,

        init() {
            this._liveRegion = createLiveRegion();
            this._trigger = this.$el?.querySelector?.('.job-panel-trigger') || null;
            this._root = this.$el || null;
            this._keydownHandler = event => this.handleShortcut(event);
            this._panelOpenHandler = event => this.openFromEvent(event.detail);
            document.addEventListener('keydown', this._keydownHandler);
            window.addEventListener('jobs-panel-open', this._panelOpenHandler);
            this.$watch?.('isOpen', open => {
                if (open) this.$nextTick?.(() => focusFirstIn(this.$refs?.panel));
                else {
                    restoreFocus(this._lastTrigger, this._trigger);
                    this._lastTrigger = null;
                }
            });
            this.connect();
            this.refresh();
        },

        destroy() {
            if (this._keydownHandler) document.removeEventListener('keydown', this._keydownHandler);
            if (this._panelOpenHandler) window.removeEventListener('jobs-panel-open', this._panelOpenHandler);
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this._refreshGeneration += 1;
            this._streamGeneration += 1;
            this._panelRefreshRequested = false;
            this.eventSource?.close();
            this._liveRegion?.destroy();
        },

        get counts() { return panelCounts(this.summary); },
        get activeCount() { return this.counts.active; },
        get attentionCount() { return this.counts.attention; },
        get finishedCount() {
            return this.jobs.filter(job => classifyJobState(job) === 'finished' &&
                this.commandsFor(job).some(command => command.key === 'dismiss')).length;
        },

        handleShortcut(event) {
            if (!(event.metaKey || event.ctrlKey) || !event.shiftKey || String(event.key).toLowerCase() !== 'd') return;
            event.preventDefault();
            this.toggle(event);
        },

        blockingModal() {
            if (typeof document === 'undefined') return null;
            return blockingModal([this._root, this.$refs?.panel]);
        },

        openFromEvent(detail = null) {
            if (!this.isOpen) {
                if (this.blockingModal()) {
                    this.announce('A dialog is open. Close it before opening Jobs.');
                    return;
                }
                const requested = detail?.returnFocusTo;
                this._lastTrigger = (isRendered(requested) ? requested : null) ?? focusedElement() ?? this._trigger;
            }
            this.isOpen = true;
        },

        toggle(event = null) {
            if (!this.isOpen && this.blockingModal()) {
                this.announce('A dialog is open. Close it before opening Jobs.');
                return;
            }
            if (!this.isOpen) this._lastTrigger = captureTrigger(event) ?? focusedElement() ?? this._trigger;
            this.isOpen = !this.isOpen;
        },

        close() {
            this.isOpen = false;
            restoreFocus(this._lastTrigger, this._trigger);
            this._lastTrigger = null;
        },

        announce(message) {
            this._liveRegion?.announce(message);
        },

        async requestJSON(url, init = {}) {
            const response = await fetch(url, {
                ...init,
                headers: { Accept: 'application/json', ...(init.headers || {}) },
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
                const error = new Error(payload.error || `Request failed (${response.status})`);
                error.status = response.status;
                error.payload = payload;
                throw error;
            }
            return payload;
        },

        async refresh() {
            const generation = ++this._refreshGeneration;
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this._panelRefreshRequested = false;
            return this.refreshAtGeneration(generation);
        },

        async refreshAtGeneration(generation) {
            this.error = '';
            try {
                const [summary, ...pages] = await Promise.all([
                    this.requestJSON('/v1/jobs/summary'),
                    ...PANEL_STATE_FILTERS.map(states => this.requestJSON(buildPanelListURL(states))),
                ]);
                if (generation !== this._refreshGeneration) return;
                this.summary = summary;
                const byId = new Map();
                for (const payload of pages) {
                    for (const job of payload.jobs || []) if (!byId.has(job.id)) byId.set(job.id, job);
                }
                this.jobs = boundedPanelJobs([...byId.values()]);
                await Promise.all(this.jobs.map(job => this.loadAdvertisedCommands(job, generation).catch(() => null)));
            } catch (error) {
                if (generation === this._refreshGeneration) this.error = error.message || 'Could not load jobs.';
            }
        },

        schedulePanelRefresh() {
            this._panelRefreshRequested = true;
            if (this._panelRefreshPromise) {
                if (!this._panelRefreshMaxTimer) {
                    this._panelRefreshMaxTimer = setTimeout(() => this.startScheduledPanelRefresh(), PANEL_REFRESH_MAX_WAIT_MS);
                }
                return;
            }
            if (!this._panelRefreshMaxTimer) {
                this._panelRefreshMaxTimer = setTimeout(() => this.startScheduledPanelRefresh(), PANEL_REFRESH_MAX_WAIT_MS);
            }
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            this._panelRefreshTimer = setTimeout(() => {
                this.startScheduledPanelRefresh();
            }, 150);
        },

        startScheduledPanelRefresh() {
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this._panelRefreshTimer = null;
            this._panelRefreshMaxTimer = null;
            if (this._panelRefreshPromise) {
                this._panelRefreshRequested = true;
                return;
            }
            this._panelRefreshRequested = false;
            const generation = ++this._refreshGeneration;
            this._panelRefreshPromise = this.refreshAtGeneration(generation)
                .catch(() => {})
                .finally(() => {
                    this._panelRefreshPromise = null;
                    if (this._panelRefreshRequested) this.startScheduledPanelRefresh();
                });
        },

        async loadAdvertisedCommands(job, generation = this._refreshGeneration) {
            if (advertisedCommands(job).length) {
                this.details[job.id] = job;
                return job;
            }
            const streamGeneration = this._streamGeneration;
            const detail = await this.requestJSON(`/v1/jobs/${encodeURIComponent(job.id)}`);
            if (generation !== this._refreshGeneration || streamGeneration !== this._streamGeneration || !this.jobs.some(current => current.id === job.id)) return null;
            const current = this.jobs.find(currentJob => currentJob.id === job.id);
            if (Number(detail.version || 0) < Number(current?.version || 0)) return null;
            this.details[job.id] = detail;
            this.jobs = boundedPanelJobs(this.jobs.map(current => current.id === job.id ? { ...current, ...detail } : current));
            return detail;
        },

        connect() {
            if (this.eventSource || typeof EventSource === 'undefined') return;
            this.connectionStatus = 'connecting';
            this.eventSource = new EventSource('/v1/jobs/events?version=2');
            this.eventSource.addEventListener('open', () => { this.connectionStatus = 'connected'; });
            this.eventSource.addEventListener('error', () => {
                this.connectionStatus = 'reconnecting';
                this.streamCaughtUp = false;
                this._refreshGeneration += 1;
                this._streamGeneration += 1;
                if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
                if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
                this._panelRefreshTimer = null;
                this._panelRefreshMaxTimer = null;
                this._panelRefreshRequested = false;
            });
            this.eventSource.addEventListener('job-caught-up', event => this.markStreamCaughtUp(event));
            for (const eventName of ['message', 'job']) {
                this.eventSource.addEventListener(eventName, event => this.handleStreamMessage(event));
            }
        },

        markStreamCaughtUp(event) {
            let boundary;
            try { boundary = JSON.parse(event.data); }
            catch { return; }
            const sequence = streamCursorSequence(boundary?.cursor);
            if (sequence === null) return;
            const wasCaughtUp = this.streamCaughtUp;
            this.lastSequence = Math.max(this.lastSequence, sequence);
            this.streamCaughtUp = true;
            this._streamGeneration += 1;
            if (!wasCaughtUp) this.schedulePanelRefresh();
        },

        async handleStreamMessage(event) {
            let message;
            try { message = JSON.parse(event.data); }
            catch { return; }
            message.lastEventId = event.lastEventId;
            message.replay = message.replay === true || !this.streamCaughtUp;
            const announceSnapshot = !message.replay;
            const previousSequence = this.lastSequence;
            const result = reduceJobStreamEvent(this.jobs, message, this.lastSequence, { allowInsert: false });
            this.lastSequence = result.lastSequence;
            if (this.lastSequence > previousSequence && this.streamCaughtUp) this.schedulePanelRefresh();
            if (result.changed || this.lastSequence > previousSequence) this._streamGeneration += 1;
            if (!result.changed) return;
            if (result.needsSnapshot) {
                if (!this.jobs.some(job => job.id === result.jobId)) return;
                const streamGeneration = this._streamGeneration;
                const refreshGeneration = this._refreshGeneration;
                try {
                    const detail = await this.requestJSON(`/v1/jobs/${encodeURIComponent(result.jobId)}`);
                    if (streamGeneration !== this._streamGeneration || refreshGeneration !== this._refreshGeneration || !this.jobs.some(job => job.id === detail.id)) return;
                    this.applyStreamSnapshot(detail, announceSnapshot, false);
                } catch { /* A hidden or expired Job stays absent from the panel. */ }
                return;
            }
            this.jobs = boundedPanelJobs(result.jobs);
            if (result.announcement) this.announce(result.announcement);
        },

        applyStreamSnapshot(job, announce = false, allowInsert = true) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence, { allowInsert });
            if (!result.changed) return;
            this.jobs = boundedPanelJobs(result.jobs);
            this.details[job.id] = { ...(this.details[job.id] || {}), ...job };
            if (announce && result.announcement) this.announce(result.announcement);
        },

        upsert(job) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence, { allowInsert: true });
            this.jobs = boundedPanelJobs(result.jobs);
        },

        detailURL(job) {
            return `/job?id=${encodeURIComponent(job?.id || '')}`;
        },

        commandsFor(job) {
            return advertisedCommands(this.details[job.id] || job);
        },

        async runCommand(job, command) {
            const confirmation = panelCommandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(confirmation, {
                    title: commandLabel(command), confirmLabel: commandLabel(command),
                });
                if (!accepted) return null;
            }
            const key = commandKey();
            try {
                const result = await this.requestJSON(commandEndpoint(job, command), {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                    body: JSON.stringify({ expectedVersion: command.jobVersion ?? job.version, idempotencyKey: key }),
                });
                const outcome = result.result || result;
                const freshJob = outcome.job || result.job;
                if (freshJob?.id) this.applyStreamSnapshot(freshJob);
                const successorId = outcome.successorId || outcome.successorID || result.successorId || result.successorID;
                if (successorId) globalThis.location?.assign?.(`/job?id=${encodeURIComponent(successorId)}`);
                this.notice = outcome.message || `${commandLabel(command)} requested.`;
                this.announce(this.notice);
                return outcome;
            } catch (error) {
                const freshJob = error.payload?.job;
                if (error.status === 409 && freshJob?.id) {
                    this.applyStreamSnapshot(freshJob);
                    this.notice = 'This job changed. The latest details are shown.';
                    this.announce(this.notice);
                    return null;
                }
                this.notice = error.message || 'The command could not be completed.';
                this.announce(this.notice);
                return null;
            }
        },

        async dismissFinished() {
            const finished = this.jobs.filter(job => classifyJobState(job) === 'finished');
            const eligible = finished.map(job => ({ job, command: this.commandsFor(job).find(command => command.key === 'dismiss') }))
                .filter(item => item.command);
            if (!eligible.length || this.busy) return [];
            const unavailable = finished.filter(job => !eligible.some(item => item.job.id === job.id))
                .map(job => ({ jobId: job.id, key: 'dismiss', status: 'failed', code: 'not-advertised', message: 'Dismiss is not available for this job.' }));
            const bulk = eligible.every(item => item.command.bulk);
            this.busy = true;
            this.outcomes = [];
            try {
                if (bulk) {
                    const key = commandKey();
                    const payload = await this.requestJSON(`/v1/jobs/commands/${encodeURIComponent(eligible[0].command.key)}`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                        body: JSON.stringify({ jobIds: eligible.map(item => item.job.id), idempotencyKey: key }),
                    });
                    this.outcomes = [...(payload.results || []), ...unavailable];
                } else {
                    for (const item of eligible) {
                        const outcome = await this.runCommand(item.job, item.command);
                        this.outcomes.push({ jobId: item.job.id, ...(outcome || { status: 'failed', message: this.notice }) });
                    }
                    this.outcomes.push(...unavailable);
                }
                const done = this.outcomes.filter(outcome => outcome.status === 'succeeded' || outcome.code === 'applied').length;
                this.notice = `${done} of ${finished.length} finished job${finished.length === 1 ? '' : 's'} dismissed.`;
                this.announce(this.notice);
                await this.refresh();
                return this.outcomes;
            } catch (error) {
                this.notice = error.message || 'Could not dismiss finished jobs.';
                this.announce(this.notice);
                return this.outcomes;
            } finally {
                this.busy = false;
            }
        },

        stateLabel(job) { return stateLabel(job); },
        commandLabel(command) { return commandLabel(command); },
    };
}

function buildPanelListURL(states) {
    const params = new URLSearchParams();
    states.forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    params.set('limit', String(PANEL_LIMIT));
    return `/v1/jobs?${params}`;
}

function boundedPanelJobs(jobs) {
    const unique = new Map();
    for (const job of jobs || []) {
        if (!unique.has(job.id)) unique.set(job.id, job);
    }
    return [...unique.values()]
        .sort((a, b) => String(b.acceptedAt || '').localeCompare(String(a.acceptedAt || '')) || String(b.id).localeCompare(String(a.id)))
        .slice(0, PANEL_LIMIT * PANEL_STATE_FILTERS.length);
}
