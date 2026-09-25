import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { captureTrigger, focusedElement, focusFirstIn, focusOn, restoreFocus } from '../utils/focus.js';
import { blockingModal, isRendered } from '../utils/modality.js';
import {
    advertisedCommands,
    classifyJobState,
    commandEndpoint,
    commandLabel,
    failureText,
    jobCommands,
    advertisedOutputs,
    reduceJobStreamEvent,
    resultAccessibleLabel,
    resultLinkLabel,
    resultOutput,
    resultURL,
    stateLabel,
    streamCursorSequence,
    progressAccessibleText,
    progressIndeterminate,
    progressText,
    progressValue,
    phaseText,
} from './jobCenter.js';
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

// Work that is running, waiting or needs a person is listed up to this many per
// group, which is as good as uncapped for a drawer; each row also costs one
// detail fetch for its commands, so it is not literally unbounded.
const OPEN_WORK_LIMIT = 50;
// Finished rows follow the deployment's download_cockpit_limit, published on the
// page as a meta tag; this is the fallback when the tag is missing.
const DEFAULT_FINISHED_LIMIT = 10;
// The list API's page ceiling. The boot flag is not bounded like the runtime
// setting, and a page over the ceiling is refused, which would blank the drawer.
const MAX_FINISHED_LIMIT = 200;
const PANEL_REFRESH_MAX_WAIT_MS = 500;
// One list page is one bulk dismiss: the server's MaxPageSize and MaxBulkCommandJobs are both 200.
const FINISHED_PAGE_LIMIT = 200;
const FINISHED_STATES = ['succeeded', 'cancelled'];
// Each group is its own bounded page. Only open work asks for the progress
// series: it is up to 120 points per Job, and a finished row shows no graph.
function panelGroups(finishedLimit) {
    return [
        { key: 'attention', states: ['blocked', 'failed', 'interrupted'], limit: OPEN_WORK_LIMIT, series: false },
        { key: 'active', states: ['scheduled', 'queued', 'running', 'paused'], limit: OPEN_WORK_LIMIT, series: true },
        { key: 'finished', states: FINISHED_STATES, limit: finishedLimit, series: false },
    ];
}

export function panelFinishedLimit(doc = globalThis.document) {
    const raw = doc?.querySelector?.('meta[name="x-jobs-panel-finished-limit"]')?.getAttribute('content');
    const parsed = Number.parseInt(raw || '', 10);
    return Number.isFinite(parsed) && parsed > 0 ? Math.min(parsed, MAX_FINISHED_LIMIT) : DEFAULT_FINISHED_LIMIT;
}

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
        finishedLimit: DEFAULT_FINISHED_LIMIT,
        finishedHasMore: false,
        // Bumped once a second while the drawer is open, so "about 14 s left"
        // counts down between progress frames.
        now: Date.now(),
        _clockTimer: null,
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
            this.finishedLimit = panelFinishedLimit();
            this._liveRegion = createLiveRegion();
            this._trigger = this.$el?.querySelector?.('.job-panel-trigger') || null;
            this._root = this.$el || null;
            this._keydownHandler = event => this.handleShortcut(event);
            this._panelOpenHandler = event => this.openFromEvent(event.detail);
            document.addEventListener('keydown', this._keydownHandler);
            window.addEventListener('jobs-panel-open', this._panelOpenHandler);
            this.$watch?.('isOpen', open => {
                if (open) {
                    this.startClock();
                    this.$nextTick?.(() => focusFirstIn(this.$refs?.panel));
                } else {
                    this.stopClock();
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
            this.stopClock();
            this._refreshGeneration += 1;
            this._streamGeneration += 1;
            this._panelRefreshRequested = false;
            this.eventSource?.close();
            this._liveRegion?.destroy();
        },

        startClock() {
            if (this._clockTimer) return;
            this.now = Date.now();
            this._clockTimer = setInterval(() => { this.now = Date.now(); }, 1000);
        },

        stopClock() {
            if (this._clockTimer) clearInterval(this._clockTimer);
            this._clockTimer = null;
        },

        get counts() { return panelCounts(this.jobs); },
        get attentionJobs() { return this.jobs.filter(job => classifyJobState(job) === 'attention'); },
        get activeJobs() { return this.jobs.filter(job => classifyJobState(job) === 'active'); },
        get finishedJobs() { return this.jobs.filter(job => classifyJobState(job) === 'finished'); },
        // The drawer's sections, in the order a person acts on them. An empty
        // section is left out rather than drawn with nothing under it.
        get groups() {
            return [
                { key: 'attention', title: 'Needs attention', jobs: this.attentionJobs },
                { key: 'active', title: 'Active and scheduled', jobs: this.activeJobs },
                { key: 'finished', title: 'Finished', jobs: this.finishedJobs },
            ].filter(group => group.jobs.length > 0);
        },
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
                const groups = panelGroups(this.finishedLimit);
                const pages = await Promise.all(groups.map(group => this.requestJSON(buildPanelListURL(group))));
                if (generation !== this._refreshGeneration) return;
                const byId = new Map();
                pages.forEach((payload, index) => {
                    if (groups[index].key === 'finished') this.finishedHasMore = !!payload.nextCursor;
                    for (const job of payload.jobs || []) if (!byId.has(job.id)) byId.set(job.id, job);
                });
                const nextJobs = this.bounded([...byId.values()]);
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
            this.jobs = this.bounded(this.jobs.map(current => current.id === job.id ? { ...current, ...detail } : current));
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
            this.eventSource.addEventListener('job-progress', event => this.handleProgressFrame(event));
            for (const eventName of ['message', 'job']) {
                this.eventSource.addEventListener(eventName, event => this.handleStreamMessage(event));
            }
        },

        // A live progress frame updates the row it names in place. It is not a
        // lifecycle event: it never refetches the list, never inserts a row the
        // list did not return, and is never announced — a screen reader told
        // every second that a download moved would hear nothing else.
        handleProgressFrame(event) {
            let frame;
            try { frame = JSON.parse(event.data); }
            catch { return; }
            if (!frame?.jobId) return;
            const index = this.jobs.findIndex(job => job.id === frame.jobId);
            if (index < 0) return;
            const next = applyProgressFrame(this.jobs[index], frame);
            if (next === this.jobs[index]) return;
            const jobs = [...this.jobs];
            jobs[index] = next;
            this.jobs = jobs;
        },

        // Every list assignment goes through here, so a fetched or command-
        // answered copy of a row never rolls back the live progress it holds.
        bounded(jobs) {
            const held = new Map(this.jobs.map(job => [job.id, job]));
            return boundedPanelJobs((jobs || []).map(job => mergeFetchedProgress(job, held.get(job?.id))), this.finishedLimit);
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
            this.jobs = this.bounded(result.jobs);
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
            this.jobs = this.bounded(result.jobs);
            this.trackResourceCompletion(job);
            this.details[job.id] = { ...(this.details[job.id] || {}), ...job };
            if (announce && result.announcement) this.announce(result.announcement);
        },

        upsert(job) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence, { allowInsert: true });
            this.jobs = this.bounded(result.jobs);
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
        failureText(job) { return failureText(job); },
        commandLabel(command) { return commandLabel(command); },
        phaseText(job) { return phaseText(job); },
        progressText(job) { return progressText(job); },
        progressValue(job) { return progressValue(job); },
        // Only running work pulses: a paused or queued Job with no total is
        // waiting, not working.
        progressIndeterminate(job) { return progressIndeterminate(job) && job?.state === 'running'; },
        progressAccessibleText(job) { return progressAccessibleText(job); },
        showsProgress(job) {
            const progress = job?.progress || {};
            return classifyJobState(job) === 'active' && (
                Number.isFinite(progress.completed) || !!progress.message || job.state === 'running');
        },
        // The line above the bar: what the executor says it is doing, else its
        // phase. The counts are in statsText, formatted, rather than here raw.
        progressLabel(job) {
            return job?.progress?.message || phaseText(job) || stateLabel(job);
        },
        // Everything the bar shows, for a reader who cannot see it.
        progressValueText(job) {
            const value = progressValue(job);
            const parts = [value === null ? '' : `${value}%`, this.amountText(job), this.rateText(job), this.etaText(job)].filter(Boolean);
            return parts.length ? parts.join(', ') : progressAccessibleText(job);
        },
        amountText(job) { return formatAmount(job?.progress); },
        rateText(job) {
            const progress = job?.progress || {};
            if (job?.state === 'running') return liveRateText(progress, this.now);
            if (classifyJobState(job) === 'finished') {
                const average = formatRate(progress.averageRate, progress.unit);
                return average ? `average ${average}` : '';
            }
            return '';
        },
        etaText(job) { return job?.state === 'running' ? liveEtaText(job?.progress, this.now) : ''; },
        statsText(job) {
            return [this.amountText(job), this.rateText(job), this.etaText(job)].filter(Boolean).join(' · ');
        },
        metricsFor(job) { return job?.progress?.metrics || []; },
        metricText(metric) { return formatMetric(metric); },
        graphsFor(job) { return classifyJobState(job) === 'active' ? graphSeries(job) : []; },
        sparkline(series) { return sparklinePath(series.points, 120, 28); },
        graphLabel(series) { return graphSummary(series); },
        graphLatest(series) { return graphLatest(series); },
    };
}

function buildPanelListURL(group) {
    const params = new URLSearchParams();
    group.states.forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    params.set('limit', String(group.limit));
    if (group.series) params.set('include', 'progressSeries');
    return `/v1/jobs?${params}`;
}

function buildFinishedPageURL(cursor) {
    const params = new URLSearchParams();
    FINISHED_STATES.forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    params.set('limit', String(FINISHED_PAGE_LIMIT));
    if (cursor) params.set('cursor', cursor);
    return `/v1/jobs?${params}`;
}

// Newest first, each group held to its own limit: a burst of new running work
// must not push the failures a person has to act on out of the drawer.
function boundedPanelJobs(jobs, finishedLimit = DEFAULT_FINISHED_LIMIT) {
    const unique = new Map();
    for (const job of jobs || []) {
        if (!unique.has(job.id)) unique.set(job.id, job);
    }
    const limits = { attention: OPEN_WORK_LIMIT, active: OPEN_WORK_LIMIT, finished: finishedLimit };
    const kept = { attention: 0, active: 0, finished: 0, other: 0 };
    return [...unique.values()]
        .sort((a, b) => String(b.acceptedAt || '').localeCompare(String(a.acceptedAt || '')) || String(b.id).localeCompare(String(a.id)))
        .filter(job => {
            const group = classifyJobState(job);
            if (!(group in limits)) return false;
            kept[group] += 1;
            return kept[group] <= limits[group];
        });
}
