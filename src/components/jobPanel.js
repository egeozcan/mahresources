import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { captureTrigger, focusedElement, focusFirstIn, focusOn, restoreFocus } from '../utils/focus.js';
import { blockingModal, isRendered } from '../utils/modality.js';
import {
    advertisedCommands,
    classifyJobState,
    commandEndpoint,
    commandLabel,
    jobCommands,
    advertisedOutputs,
    reduceJobStreamEvent,
    resultAccessibleLabel,
    resultLinkLabel,
    resultOutput,
    resultURL,
    stateLabel,
    streamCursorSequence,
} from './jobCenter.js';

const PANEL_LIMIT = 5;
const PANEL_REFRESH_MAX_WAIT_MS = 500;
// One list page is one bulk dismiss: the server's MaxPageSize and MaxBulkCommandJobs are both 200.
const FINISHED_PAGE_LIMIT = 200;
const PANEL_STATE_FILTERS = [
    ['blocked', 'failed', 'interrupted'],
    ['scheduled', 'queued', 'running', 'paused'],
    ['succeeded', 'cancelled'],
];

export function panelCounts(jobs) {
    return (jobs || []).reduce((counts, job) => {
        const classification = classifyJobState(job);
        if (classification === 'active') counts.active += 1;
        if (classification === 'attention') counts.attention += 1;
        return counts;
    }, { active: 0, attention: 0 });
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
        return 'Forget this job’s saved replay input. Retry, Continue and Repeat will no longer be possible. Its sanitized history remains, and its outputs and artifacts are not affected. This cannot be undone.';
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
        eventSource: null,
        lastSequence: 0,
        streamCaughtUp: false,
        connectionStatus: 'disconnected',
        error: '',
        notice: '',
        _pendingLiveJobUpdates: new Map(),
        _resourceRefreshNotified: new Set(),
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
            // The expression is the Dismiss finished button's own x-show.
            this.$watch?.('finishedCount > 0 || busy', shown => {
                if (!shown) this.$nextTick?.(() => this.keepFocusWhenDismissHides());
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

        get counts() { return panelCounts(this.jobs); },
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
            const pendingLiveUpdates = new Map([...this._pendingLiveJobUpdates.entries()].map(([jobId, update]) => [
                jobId,
                { generation: update.generation, previous: update.previous },
            ]));
            try {
                const pages = await Promise.all(
                    PANEL_STATE_FILTERS.map(states => this.requestJSON(buildPanelListURL(states))),
                );
                if (generation !== this._refreshGeneration) return;
                const byId = new Map();
                for (const payload of pages) {
                    for (const job of payload.jobs || []) if (!byId.has(job.id)) byId.set(job.id, job);
                }
                const nextJobs = boundedPanelJobs([...byId.values()]);
                this.announceRefreshedLiveTransitions(nextJobs, pendingLiveUpdates);
                nextJobs.forEach(job => this.trackResourceCompletion(job));
                this.jobs = nextJobs;
                await Promise.all(this.jobs.map(job => this.loadAdvertisedCommands(job, generation).catch(() => null)));
            } catch (error) {
                if (generation === this._refreshGeneration) this.error = error.message || 'Could not load jobs.';
            }
        },

        queueLiveJobUpdate(jobId) {
            const previous = this.jobs.find(job => job.id === jobId);
            if (!previous) return;
            const pending = this._pendingLiveJobUpdates.get(jobId);
            if (pending) pending.generation += 1;
            else this._pendingLiveJobUpdates.set(jobId, { generation: 1, previous });
        },

        announceRefreshedLiveTransitions(nextJobs, capturedUpdates) {
            const refreshed = new Map(nextJobs.map(job => [job.id, job]));
            for (const [jobId, captured] of capturedUpdates) {
                const currentPending = this._pendingLiveJobUpdates.get(jobId);
                if (!currentPending) continue;
                const next = refreshed.get(jobId);
                if (next) {
                    const result = reduceJobStreamEvent([captured.previous], { job: next }, this.lastSequence, { allowInsert: true });
                    if (result.announcement) this.announce(result.announcement);
                }
                if (currentPending.generation === captured.generation) this._pendingLiveJobUpdates.delete(jobId);
                else if (next) currentPending.previous = next;
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
            if (generation !== this._refreshGeneration || streamGeneration !== this._streamGeneration ||
                !this.jobs.some(current => current.id === job.id)) return null;
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
                this._pendingLiveJobUpdates.clear();
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
            this._pendingLiveJobUpdates.clear();
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
            if (!result.changed) return;
            const jobId = result.jobId || message.job?.id || message.snapshot?.id ||
                (message.id && message.state ? message.id : '') || message.jobId || message.jobID || '';
            if (result.needsSnapshot) {
                if (!message.replay) this.queueLiveJobUpdate(result.jobId);
                return;
            }
            this._pendingLiveJobUpdates.delete(jobId);
            this.jobs = boundedPanelJobs(result.jobs);
            this.jobs.forEach(job => this.trackResourceCompletion(job));
            if (result.announcement) this.announce(result.announcement);
        },

        trackResourceCompletion(job) {
            if (!job?.id || job.state !== 'succeeded' ||
                (job.kind !== 'remote-download' && job.kind !== 'deferred-download') ||
                this._resourceRefreshNotified.has(job.id)) return;
            this._resourceRefreshNotified.add(job.id);
            if (this._resourceRefreshNotified.size > 256) {
                this._resourceRefreshNotified.delete(this._resourceRefreshNotified.values().next().value);
            }
            if (this.streamCaughtUp && globalThis.window?.dispatchEvent && globalThis.CustomEvent) {
                globalThis.window.dispatchEvent(new CustomEvent('download-completed', { detail: { jobId: job.id } }));
            }
        },

        // Every caller is a command or preference answer about a row the panel
        // already had. A row gone by the time the answer lands was removed on
        // purpose — dismissed, or refreshed out — so the answer updates rows
        // and never resurrects one.
        applyStreamSnapshot(job, announce = false, allowInsert = false) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence, { allowInsert });
            if (!result.changed) return;
            this.jobs = boundedPanelJobs(result.jobs);
            this.trackResourceCompletion(job);
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

        // The list row carries the live state; the detail carries the outputs.
        resultSource(job) {
            const detail = this.details[job?.id] || job;
            return { ...detail, ...job, outputs: advertisedOutputs(detail) };
        },

        resultOutput(job) { return resultOutput(this.resultSource(job)); },
        resultURL(job) { return resultURL(this.resultSource(job)); },
        resultLinkLabel(job) { return resultLinkLabel(this.resultSource(job)); },
        resultAccessibleLabel(job) { return resultAccessibleLabel(this.resultSource(job)); },

        commandsFor(job) {
            return jobCommands(this.details[job.id] || job);
        },

        async refreshJobPreference(id) {
            const payload = await this.requestJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            const freshJob = payload.job || payload;
            if (freshJob?.id) {
                this.details[id] = freshJob;
                this.applyStreamSnapshot(freshJob);
            }
            return freshJob;
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
                let preferenceRefreshFailed = false;
                if (command?.key === 'pin' || command?.key === 'unpin') {
                    try { await this.refreshJobPreference(job.id); }
                    catch { preferenceRefreshFailed = true; }
                }
                const successorId = outcome.successorId || outcome.successorID || result.successorId || result.successorID;
                if (successorId) globalThis.location?.assign?.(`/job?id=${encodeURIComponent(successorId)}`);
                this.notice = preferenceRefreshFailed
                    ? `${commandLabel(command)} completed. Reload this job to see its current pin status.`
                    : outcome.message || `${commandLabel(command)} requested.`;
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

        // Dismisses every finished job this viewer has not dismissed, not only the
        // few the panel shows. The list is walked by keyset cursor, so rows leaving
        // the undismissed filter while we page do not shift it, and each page fits
        // one bulk request. The server answers per job, so it alone decides which
        // jobs may be dismissed. Only counts are kept: a backlog can be far larger
        // than anything worth holding in the page.
        async dismissFinished() {
            if (this.busy || this.finishedCount === 0) return { dismissed: 0, total: 0 };
            this.busy = true;
            let dismissed = 0;
            let total = 0;
            let refusal = '';
            try {
                let cursor = '';
                do {
                    const page = await this.requestJSON(buildFinishedPageURL(cursor));
                    const jobIds = (page.jobs || []).map(job => job.id);
                    if (jobIds.length) {
                        const key = commandKey();
                        const payload = await this.requestJSON('/v1/jobs/commands/dismiss', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                            body: JSON.stringify({ jobIds, idempotencyKey: key }),
                        });
                        // A refresh issued before this dismissal committed
                        // would put its rows back; the generation fence
                        // discards it. One issued from here on already sees
                        // the dismissal.
                        this._refreshGeneration += 1;
                        const confirmed = new Set();
                        for (const result of payload.results || []) {
                            total += 1;
                            if (result.status === 'succeeded' || result.code === 'applied') {
                                dismissed += 1;
                                confirmed.add(result.jobId);
                            } else if (!refusal) {
                                refusal = result.message || result.code || 'refused';
                            }
                        }
                        // Rows the server confirmed dismissed leave the panel
                        // now rather than waiting on a final refresh that may
                        // fail. Only this page's ids are held.
                        if (confirmed.size) this.jobs = this.jobs.filter(job => !confirmed.has(job.id));
                    }
                    cursor = page.nextCursor || '';
                } while (cursor);
                const plural = total === 1 ? '' : 's';
                this.notice = dismissed === total
                    ? `${dismissed} finished job${plural} dismissed.`
                    : `${dismissed} of ${total} finished job${plural} dismissed. Not dismissed: ${refusal}`;
                this.announce(this.notice);
            } catch (error) {
                const reason = error.message || 'Could not dismiss finished jobs.';
                this.notice = dismissed > 0
                    ? `${dismissed} finished job${dismissed === 1 ? '' : 's'} dismissed before an error: ${reason}`
                    : reason;
                if (refusal) this.notice = `${this.notice.replace(/\.$/, '')}. Not dismissed: ${refusal}`;
                this.announce(this.notice);
            }
            // Busy lasts through the refresh, which brings in whatever the
            // cleared rows made room for.
            try {
                await this.refresh();
            } finally {
                this.busy = false;
            }
            return { dismissed, total };
        },

        // "Dismiss finished" hides once nothing finished is shown, and it is
        // usually the control holding focus. Whichever refresh hides it — this
        // one, or a stream-driven one that lands later — hand focus to the
        // footer's next control rather than letting it fall behind the dialog.
        // Focus the reader already moved elsewhere is left alone. The decision
        // reads the state x-show reads rather than the button's visibility,
        // because x-show applies a hide a frame later than the state changes.
        keepFocusWhenDismissHides() {
            if (typeof document === 'undefined' || this.finishedCount > 0 || this.busy) return;
            const button = document.querySelector('#job-center-panel [data-job-panel-dismiss-finished]');
            if (!button) return;
            const active = document.activeElement;
            if (active && active !== button && active !== document.body) return;
            focusOn(document.querySelector('#job-center-panel [data-job-panel-all-jobs]'));
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

function buildFinishedPageURL(cursor) {
    const params = new URLSearchParams();
    PANEL_STATE_FILTERS[2].forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    params.set('limit', String(FINISHED_PAGE_LIMIT));
    if (cursor) params.set('cursor', cursor);
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
