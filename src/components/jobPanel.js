import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { captureTrigger, focusedElement, focusFirstIn, focusOn, restoreFocus } from '../utils/focus.js';
import { blockingModal, isRendered } from '../utils/modality.js';
import {
    advertisedCommands,
    classifyJobState,
    commandEndpoint,
    eventJob,
    isPartialSuccess,
    lifecycleAnnouncement,
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

// Commands that keep a job's record rather than act on its work. They sit
// behind a row's "More" disclosure so the inline controls stay the ones a
// person reaches for: Retry, Cancel, Pause, Dismiss.
const RECORD_COMMANDS = new Set(['pin', 'unpin', 'pin-lineage', 'forget']);

export function panelCommandSplit(commands) {
    const primary = [];
    const more = [];
    for (const command of commands || []) (RECORD_COMMANDS.has(command?.key) ? more : primary).push(command);
    return { primary, more };
}

// One word per state for the row's icon and pill colour. The pill's text is
// the state label, so the colour never carries the meaning alone.
export function panelStateTone(job) {
    switch (job?.state) {
    case 'running': return 'working';
    case 'queued':
    case 'scheduled': return 'waiting';
    case 'paused': return 'paused';
    case 'succeeded': return isPartialSuccess(job) ? 'warning' : 'done';
    case 'blocked': return 'warning';
    case 'failed':
    case 'interrupted': return 'failed';
    default: return 'neutral';
    }
}

// The trigger's accessible description: its badges are hidden from assistive
// technology because the button's aria-label replaces them as its name.
export function panelCountsText({ active = 0, attention = 0 } = {}) {
    // "Showing": each group is capped, so these count rows shown, not every job.
    const activeText = `${active} active or scheduled job${active === 1 ? '' : 's'}`;
    return `Showing ${activeText} and ${attention} needing attention`;
}

// How many jobs the announcement ledger remembers. A job older than this that
// changes is recorded again without being said, which is the safe side.
const HEARD_LIMIT = 1000;
// Event types a Job's state transitions are recorded under: the lifecycle
// constants in jobs/types.go, plus the one type an executor passes of its own
// (a plugin action cancelled before it started). Delivered live, after
// catch-up, one is proof that the transition at its version happened live.
// jobPanel.test.ts reads the Go sources to keep this complete.
export const panelLifecycleEvents = new Set([
    'accepted', 'scheduled', 'queued', 'started', 'resumed', 'paused', 'blocked',
    'succeeded', 'failed', 'cancelled', 'interrupted', 'not-started',
]);
// Both live regions replace a message that has not landed within 50 ms. News
// made that close together is said together rather than cancelled.
const NEWS_COALESCE_MS = 50;

// Where focus goes when the command control that had it leaves the row: its
// counterpart first, since Pin becomes Unpin, then the same command re-rendered.
const COMMAND_COUNTERPARTS = { pin: 'unpin', unpin: 'pin', pause: 'resume', resume: 'pause' };

export function panelFocusSuccessorKeys(key) {
    return [COMMAND_COUNTERPARTS[key], key].filter(Boolean);
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
        _resourceRefreshNotified: new Set(),
        busy: false,
        finishedLimit: DEFAULT_FINISHED_LIMIT,
        finishedHasMore: false,
        // Bumped once a second while the drawer is open, so "about 14 s left"
        // counts down between progress frames.
        now: Date.now(),
        _clockTimer: null,
        _liveRegion: null,
        _drawerAnnounceTimer: null,
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
        // What the reader has been told about each job: its state and version,
        // and the stream generation that was current when it was recorded. See
        // hearJob.
        _heard: new Map(),
        // Versions a live lifecycle event proved news before any read saw them.
        _liveVersions: new Map(),
        // What was said within the last NEWS_COALESCE_MS, which may not have
        // landed: news, and at most one notice (the newest notice wins).
        _recentNews: [],
        _recentNotice: '',
        _newsAt: 0,
        // Which rows a stream snapshot changed, and when, by a counter a refresh
        // reads at its start: a row changed after that may be missing from the
        // group lists, which are read one after another.
        _streamTouchSeq: 0,
        _streamTouched: new Map(),

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
                    this.$nextTick?.(() => {
                        focusFirstIn(this.$refs?.panel);
                        this.adoptPendingAnnouncement();
                    });
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
            clearTimeout(this._drawerAnnounceTimer);
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
        get countsText() { return panelCountsText(this.counts); },
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
                    this.announceNotice('A dialog is open. Close it before opening Jobs.');
                    return;
                }
                const requested = detail?.returnFocusTo;
                this._lastTrigger = (isRendered(requested) ? requested : null) ?? focusedElement() ?? this._trigger;
            }
            this.isOpen = true;
        },

        toggle(event = null) {
            if (!this.isOpen && this.blockingModal()) {
                this.announceNotice('A dialog is open. Close it before opening Jobs.');
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

        // The open drawer is aria-modal, and a screen reader may ignore a live
        // region outside a modal dialog: the one on <body> goes unheard exactly
        // when someone is watching the drawer. While it is open, its own status
        // region speaks instead. That one lands a tick later (cleared first, so
        // the same message said twice is heard twice), and where it lands is
        // decided then: the drawer's region if the drawer is still open, the
        // page's if it closed meanwhile and took its region with it.
        //
        // Two regions means two pending messages, and the newest must win across
        // both, as it does within one: any announcement cancels the drawer's
        // pending one, and one made inside the drawer withdraws the page's.
        announce(message) {
            clearTimeout(this._drawerAnnounceTimer);
            this._drawerAnnounceTimer = null;
            const inside = this._drawerAnnouncer();
            if (!inside) {
                this._liveRegion?.announce(message);
                return;
            }
            this._liveRegion?.cancel?.();
            inside.textContent = '';
            this._drawerAnnounceTimer = setTimeout(() => {
                this._drawerAnnounceTimer = null;
                const region = this._drawerAnnouncer();
                if (region) region.textContent = message;
                else this._liveRegion?.announce(message);
            }, 50);
        },

        // A message the page had not yet spoken when the drawer opened would land
        // behind the dialog: it is said inside the drawer instead.
        adoptPendingAnnouncement() {
            const pending = this._liveRegion?.cancel?.();
            if (pending) this.announce(pending);
        },

        _drawerAnnouncer() {
            if (!this.isOpen) return null;
            const region = document.querySelector?.('#job-center-panel [data-job-panel-announcer]');
            return region?.isConnected ? region : null;
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
            const streamGeneration = this._streamGeneration;
            const touchedFrom = this._streamTouchSeq;
            try {
                const groups = panelGroups(this.finishedLimit);
                const pages = await Promise.all(groups.map(group => this.requestJSON(buildPanelListURL(group))));
                if (generation !== this._refreshGeneration) return;
                const byId = new Map();
                pages.forEach((payload, index) => {
                    if (groups[index].key === 'finished') this.finishedHasMore = !!payload.nextCursor;
                    for (const job of payload.jobs || []) if (!byId.has(job.id)) byId.set(job.id, job);
                });
                // A list read before a stream snapshot the panel has since
                // applied is older than the row it would replace; the newer row
                // stays, rather than the row rolling back.
                const shown = new Map(this.jobs.map(job => [job.id, job]));
                const listed = [...byId.values()].map(job => {
                    const held = shown.get(job.id);
                    return held && Number(held.version || 0) > Number(job.version || 0) ? held : job;
                });
                // A row the stream changed during the reads can fall between two
                // groups' lists, read before and after its move. It stays; a job
                // that is really gone leaves on the next refresh, which that
                // stream change scheduled.
                for (const [jobId, touchedAt] of this._streamTouched) {
                    if (touchedAt > touchedFrom && !byId.has(jobId) && shown.has(jobId)) listed.push(shown.get(jobId));
                    if (touchedAt <= touchedFrom) this._streamTouched.delete(jobId);
                }
                // One announcement for everything this refresh finds, its detail
                // reads included: two made in a row would each cancel the one
                // before it. It is said even if a newer refresh supersedes this
                // one, since what it heard is already recorded and nothing else
                // would say it.
                const spoken = [];
                for (const job of listed) this.hearFromRead(job, streamGeneration, spoken);
                const nextJobs = this.bounded(listed);
                nextJobs.forEach(job => this.trackResourceCompletion(job));
                this.jobs = nextJobs;
                try {
                    await Promise.all(this.jobs.map(job => this.loadAdvertisedCommands(job, generation, spoken).catch(() => null)));
                } finally {
                    this.announceHeld(spoken, streamGeneration);
                }
            } catch (error) {
                if (generation === this._refreshGeneration) this.error = error.message || 'Could not load jobs.';
            }
        },

        // Announcements are made against what the reader has been told, not
        // against the rows on screen. Rows are dropped, rolled back and re-read
        // (three group lists read one after another, detail reads after them,
        // stream messages in between), so a change measured against the rows can
        // be applied silently by one path and then found unchanged by the path
        // that should have said it. The ledger records each job's last heard
        // state and version whichever path heard it:
        //   - an older version than the one recorded is old news, and ignored;
        //   - a live stream message says any change of state;
        //   - a list or detail read says a change only against an entry recorded
        //     on the same stream generation, so what changed while disconnected
        //     is recorded, not announced, as the stream's replay is not either;
        //   - replays and command answers are recorded without being said.
        // A read cannot tell a change made while disconnected from one made just
        // after catch-up; the stream can, since only the latter arrives as a live
        // lifecycle event. So reads defer to it both ways: a live event first
        // marks its version as news for the read that follows (_liveVersions),
        // and a read first records the change it withheld, for the live event
        // that follows to say (withheld).
        // The boundary is exact to within one publish tick: catch-up replays
        // only events already given a delivery sequence, and the runtime assigns
        // those on its tick (application_context/job_runtime.go, 2 s). A change
        // committed within that tick before catch-up arrives after it, and is
        // said. That is a real change the reader had not heard; the silence for
        // history exists to spare a reader a flood after a long disconnect,
        // which one tick's worth cannot be.
        // A proof anywhere in a withheld change's range releases it, even for an
        // intermediate state: what is said is the state the read found, which
        // the job is still in (currentNews), said once. The intermediate state
        // was superseded before anyone could hear it.
        // A job with no entry is recorded without being said: the first sight
        // of a job is not news. Every path that puts a row on screen hears it;
        // a row set on screen some other way counts as heard in its shown state
        // on the first stream generation, before any stream was connected.
        // A read is heard under the generation it began on, even when it answers
        // after a catch-up: what it saw is history to the stream that followed.
        // `proofOnly` is for a command's answer: the reader asked for the change
        // and hears the command's notice, so it speaks only for a change a live
        // event already proved happened on its own.
        hearJob(job, { live = false, sameGenerationOnly = false, generation = this._streamGeneration, proofOnly = false } = {}) {
            if (!job?.id || !job.state) return '';
            const version = Number(job.version || 0);
            const shown = this._heard.has(job.id) ? null : this.jobs.find(row => row.id === job.id);
            const entry = this._heard.get(job.id) ||
                (shown ? { state: shown.state, version: Number(shown.version || 0), stateSince: Number(shown.version || 0), generation: 0 } : null);
            if (entry && version > 0 && version < entry.version) return '';
            const changed = !!entry && entry.state !== job.state;
            const liveFrom = this._liveVersions.get(job.id);
            const provenLive = liveFrom !== undefined && version >= liveFrom;
            // Only an observation that could speak uses the proof up; a stale
            // read must leave it for the live one that follows.
            if (provenLive && (live || proofOnly)) this._liveVersions.delete(job.id);
            let said = this.streamCaughtUp && changed && (
                proofOnly ? provenLive
                    : live && (!sameGenerationOnly || entry.generation === generation || provenLive)
            ) ? lifecycleAnnouncement(job) : '';
            // A withheld change happened somewhere after the version heard
            // before it (withheldFrom) and at or before the version the read saw
            // (withheldVersion). A live message or proof inside that range says
            // it now.
            const withheldIn = entry?.withheld && entry.state === job.state ? entry : null;
            const inRange = at => withheldIn && at > withheldIn.withheldFrom && at <= withheldIn.withheldVersion;
            if (!said && this.streamCaughtUp && withheldIn &&
                ((live && !sameGenerationOnly && inRange(version)) || (inRange(liveFrom) && (live || proofOnly)))) {
                said = withheldIn.withheld;
            }
            // It stays while the job is still in that state, whatever versions a
            // same-state change (a control request) adds; saying it, or a change
            // of state, retires it.
            let withheld = !said && withheldIn
                ? { text: withheldIn.withheld, from: withheldIn.withheldFrom, to: withheldIn.withheldVersion } : null;
            if (sameGenerationOnly && changed && !said) withheld = { text: lifecycleAnnouncement(job), from: entry.version, to: version };
            this._heard.delete(job.id);
            this._heard.set(job.id, {
                state: job.state, version: Math.max(version, entry?.version || 0), generation,
                // The version the current state began at: news about it stays
                // true through same-state versions (a control request).
                stateSince: entry && entry.state === job.state ? entry.stateSince : version,
                withheld: withheld?.text || '', withheldFrom: withheld?.from, withheldVersion: withheld?.to,
            });
            if (this._heard.size > HEARD_LIMIT) this._heard.delete(this._heard.keys().next().value);
            return said;
        },

        // A live lifecycle event with no snapshot: says a change a read withheld
        // at exactly its version, or marks its version as news for the next read.
        hearLiveEvent(message) {
            const jobId = message?.jobId || message?.jobID;
            const version = Number(message?.jobVersion || 0);
            if (!jobId || !version || message.replay || !this.streamCaughtUp || !panelLifecycleEvents.has(message.type)) return '';
            const entry = this._heard.get(jobId);
            if (entry?.withheld && version > entry.withheldFrom && version <= entry.withheldVersion) {
                const said = entry.withheld;
                entry.withheld = '';
                entry.withheldFrom = undefined;
                entry.withheldVersion = undefined;
                return said;
            }
            if (entry && version < entry.version) return '';
            if (entry && entry.version === version) return '';
            this._liveVersions.delete(jobId);
            this._liveVersions.set(jobId, Math.max(version, this._liveVersions.get(jobId) || 0));
            if (this._liveVersions.size > HEARD_LIMIT) this._liveVersions.delete(this._liveVersions.keys().next().value);
            return '';
        },

        // A list or detail read, which may speak only if no reconnect happened
        // since the read began; what it says is held for the refresh to say.
        hearFromRead(job, streamGeneration, spoken) {
            const said = this.hearJob(job, {
                live: streamGeneration === this._streamGeneration, sameGenerationOnly: true, generation: streamGeneration,
            });
            if (said) spoken.push(this.newsEntry(job.id, said));
        },

        // Says what a refresh held back while its detail reads ran, less
        // anything the ledger has since moved past: the stream will have said
        // the newer news already, and older news must not be the last word, even
        // when the job has come back to the same state. Nothing is said once the
        // stream has dropped since the refresh began: that is no longer live.
        announceHeld(spoken, streamGeneration) {
            if (streamGeneration !== this._streamGeneration) return;
            this.announceNews(spoken);
        },

        // A piece of news about a job, as the ledger holds it now.
        newsEntry(jobId, text) {
            const heard = this._heard.get(jobId);
            return { jobId, state: heard?.state, version: heard?.version, text };
        },

        // The news still true: the job is still in the state it announced, that
        // state has not been left and re-entered since (stateSince), and no live
        // lifecycle event has marked a later transition (it records no state,
        // but supersedes all the same). Same-state versions, such as a control
        // request, leave it true. One entry per job, the latest.
        currentNews(entries) {
            const byJob = new Map();
            for (const entry of entries) {
                const heard = this._heard.get(entry.jobId);
                const newer = this._liveVersions.get(entry.jobId);
                if (heard?.state !== entry.state || !(heard.stateSince <= entry.version)) continue;
                if (newer !== undefined && newer > entry.version) continue;
                byJob.delete(entry.jobId);
                byJob.set(entry.jobId, entry);
            }
            return [...byJob.values()];
        },

        // News said so recently it may not have landed, still true.
        pendingNews() {
            return Date.now() - this._newsAt < NEWS_COALESCE_MS ? this.currentNews(this._recentNews) : [];
        },

        pendingNotice() {
            return Date.now() - this._newsAt < NEWS_COALESCE_MS ? this._recentNotice : '';
        },

        // Every panel message goes through here: news, a notice, or both. What
        // was said within the window and may not have landed is said again with
        // it, since the region would otherwise replace it. A new notice replaces
        // a pending one; news accumulates, less anything superseded.
        say(entries = [], notice = '') {
            const news = this.currentNews([...this.pendingNews(), ...entries]);
            const text = notice || this.pendingNotice();
            this._recentNews = news;
            this._recentNotice = text;
            this._newsAt = Date.now();
            const message = [...news.map(entry => entry.text), text].filter(Boolean).join(' ');
            if (message) this.announce(message);
        },

        announceNews(entries) {
            if (this.currentNews(entries).length) this.say(entries);
        },

        announceNotice(notice, news = []) {
            this.say(news, notice);
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

        // A detail read after the list can find the job already moved on; it is
        // heard like the list, and what it says joins the refresh's `spoken`.
        async loadAdvertisedCommands(job, generation = this._refreshGeneration, spoken = null) {
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
            if (spoken) this.hearFromRead({ ...current, ...detail }, streamGeneration, spoken);
            else this.hearJob({ ...current, ...detail });
            this.jobs = this.bounded(this.jobs.map(current => current.id === job.id ? { ...current, ...detail } : current));
            return detail;
        },

        connect() {
            if (this.eventSource || typeof EventSource === 'undefined') return;
            this.connectionStatus = 'connecting';
            this.eventSource = new EventSource('/v1/jobs/events?version=2');
            this.eventSource.addEventListener('open', () => { this.connectionStatus = 'connected'; });
            this.eventSource.addEventListener('error', () => this.dropStream());
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

        // The stream dropped. What it proved live does not carry across: what
        // happens before it catches up again arrives as replay.
        dropStream() {
            this.connectionStatus = 'reconnecting';
            this.streamCaughtUp = false;
            this._refreshGeneration += 1;
            this._streamGeneration += 1;
            this._liveVersions.clear();
            // News said just before the drop must not be said again with the
            // next message as if it were new.
            this._recentNews = [];
            this._recentNotice = '';
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this._panelRefreshTimer = null;
            this._panelRefreshMaxTimer = null;
            this._panelRefreshRequested = false;
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
            const previousSequence = this.lastSequence;
            const result = reduceJobStreamEvent(this.jobs, message, this.lastSequence, { allowInsert: false });
            this.lastSequence = result.lastSequence;
            if (this.lastSequence > previousSequence && this.streamCaughtUp) this.schedulePanelRefresh();
            // Heard whether or not the row is on screen: a row a refresh dropped
            // between two group reads still has news the reader must get. An
            // event with no snapshot says nothing; the refresh it scheduled
            // reads the job and hears it.
            // A redelivered message carries no newer version, so hearing it again
            // says nothing.
            const incoming = eventJob(message);
            const said = incoming
                ? this.hearJob({ ...this.jobs.find(job => job.id === incoming.id), ...incoming }, { live: !message.replay })
                : this.hearLiveEvent(message);
            const saidAbout = incoming?.id || message.jobId || message.jobID;
            if (result.changed && !result.needsSnapshot) {
                const jobId = result.jobId || incoming?.id || '';
                this.jobs = this.bounded(result.jobs);
                this.touchFromStream(jobId);
                this.jobs.forEach(job => this.trackResourceCompletion(job));
            }
            if (said) this.announceNews([this.newsEntry(saidAbout, said)]);
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
        // A proved change goes to `spoken` when the caller will say it with its
        // own notice: two messages in a row would cancel the first.
        // `asRead` is for an answer that reports someone else's change, such as a
        // 409: it is heard as a read would be, said on the same live stream and
        // withheld for the job's live event otherwise.
        applyStreamSnapshot(job, announce = false, allowInsert = false, spoken = null, { asRead = false } = {}) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence, { allowInsert });
            if (!result.changed) return;
            // The reader asked for this change and is told by the command's own
            // notice; it is recorded so no later read says it again, unless a
            // live event proved it happened on its own.
            const heard = { ...this.jobs.find(row => row.id === job.id), ...job };
            const news = [];
            if (asRead) this.hearFromRead(heard, this._streamGeneration, news);
            else {
                const said = this.hearJob(heard, { proofOnly: true });
                if (said) news.push(this.newsEntry(job.id, said));
            }
            if (news.length && spoken) spoken.push(...news);
            else if (news.length) this.announceNews(news);
            this.jobs = this.bounded(result.jobs);
            this.trackResourceCompletion(job);
            this.details[job.id] = { ...(this.details[job.id] || {}), ...job };
            if (announce && result.announcement) this.announceNews([this.newsEntry(job.id, result.announcement)]);
        },

        // Only stream messages mark a row, never a command's answer: every
        // stream message schedules the refresh that settles the row, while a
        // command such as Dismiss may schedule none, and a row kept for it would
        // outlive its dismissal.
        touchFromStream(jobId) {
            if (jobId) this._streamTouched.set(jobId, ++this._streamTouchSeq);
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
        primaryCommandsFor(job) { return panelCommandSplit(this.commandsFor(job)).primary; },
        moreCommandsFor(job) { return panelCommandSplit(this.commandsFor(job)).more; },
        stateTone(job) { return panelStateTone(job); },

        async refreshJobPreference(id, spoken = null) {
            const payload = await this.requestJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            const freshJob = payload.job || payload;
            if (freshJob?.id) {
                this.details[id] = freshJob;
                // A read: any change of state in it is someone else's.
                this.applyStreamSnapshot(freshJob, false, false, spoken, { asRead: true });
            }
            return freshJob;
        },

        async runCommand(job, command) {
            const watch = this.watchReaderFocus();
            try {
                return await this.runCommandUnfocused(job, command);
            } finally {
                this.keepFocusOnRow(job.id, command, watch);
            }
        },

        // Records whether the reader moved focus inside the drawer while a
        // command ran. A move the reader makes, by key, click or script, comes
        // from the element losing focus, so it has a relatedTarget. The trap's
        // rescue after the focused control is removed comes from nowhere, and
        // has none. That holds even if the row re-rendered the control first,
        // which a check on the opener still being attached did not survive.
        watchReaderFocus() {
            if (typeof document === 'undefined') return null;
            const opener = focusedElement();
            if (!opener) return null;
            const watch = { opener, movedByReader: false, stop: () => {} };
            const onFocusIn = event => {
                if (event.target === opener || !event.relatedTarget) return;
                if (event.target?.closest?.('#job-center-panel')) watch.movedByReader = true;
            };
            document.addEventListener('focusin', onFocusIn, true);
            watch.stop = () => document.removeEventListener('focusin', onFocusIn, true);
            return watch;
        },

        // A command that changes the row replaces the control that ran it: the
        // pinned snapshot swaps Pin for Unpin, and the focus trap then drops focus
        // on the drawer's first control, the Close button. Focus is moved to the
        // nearest control on the same row instead, once the trap has acted. It is
        // left alone when the control survived, when the reader moved focus
        // themselves while the command ran, or when focus left the drawer.
        keepFocusOnRow(jobId, command, watch) {
            if (!watch) return;
            this.$nextTick?.(() => setTimeout(() => {
                watch.stop();
                if (watch.opener.isConnected || watch.movedByReader || !this.isOpen) return;
                const panel = document.querySelector('#job-center-panel');
                const active = document.activeElement;
                if (active && active !== document.body && !panel?.contains(active)) return;
                const row = panel?.querySelector(`article[data-job-id="${CSS.escape(String(jobId))}"]`);
                if (!row) return;
                const candidates = [
                    ...panelFocusSuccessorKeys(command?.key).map(key => row.querySelector(`button[data-command-key="${CSS.escape(key)}"]`)),
                    row.querySelector('details[open] summary'),
                    row.querySelector('[role="group"] button, [role="group"] summary'),
                    row.querySelector('a[href]'),
                ];
                for (const candidate of candidates) {
                    if (candidate && isRendered(candidate) && focusOn(candidate)) return;
                }
            }, 0));
        },

        async runCommandUnfocused(job, command) {
            const confirmation = panelCommandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(confirmation, {
                    title: commandLabel(command), confirmLabel: commandLabel(command),
                });
                if (!accepted) return null;
            }
            const key = commandKey();
            // Changes a live event proved, said with the command's notice.
            const proved = [];
            // The notice is said with the proved changes, less any a newer
            // change has superseded while the command ran.
            const sayNotice = () => this.announceNotice(this.notice, proved);
            try {
                const result = await this.requestJSON(commandEndpoint(job, command), {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                    body: JSON.stringify({ expectedVersion: command.jobVersion ?? job.version, idempotencyKey: key }),
                });
                const outcome = result.result || result;
                const freshJob = outcome.job || result.job;
                // A command that keeps the record (pin, forget, dismiss) changes no
                // state, so a change of state in its answer is someone else's and
                // is heard as a read; a lifecycle command's answer is the reader's.
                const keepsRecord = RECORD_COMMANDS.has(command?.key) || command?.key === 'dismiss';
                if (freshJob?.id) this.applyStreamSnapshot(freshJob, false, false, proved, { asRead: keepsRecord });
                let preferenceRefreshFailed = false;
                if (command?.key === 'pin' || command?.key === 'unpin') {
                    try { await this.refreshJobPreference(job.id, proved); }
                    catch { preferenceRefreshFailed = true; }
                }
                const successorId = outcome.successorId || outcome.successorID || result.successorId || result.successorID;
                if (successorId) globalThis.location?.assign?.(`/job?id=${encodeURIComponent(successorId)}`);
                this.notice = preferenceRefreshFailed
                    ? `${commandLabel(command)} completed. Reload this job to see its current pin status.`
                    : outcome.message || `${commandLabel(command)} requested.`;
                sayNotice();
                return outcome;
            } catch (error) {
                const freshJob = error.payload?.job;
                if (error.status === 409 && freshJob?.id) {
                    this.applyStreamSnapshot(freshJob, false, false, proved, { asRead: true });
                    this.notice = 'This job changed. The latest details are shown.';
                    sayNotice();
                    return null;
                }
                this.notice = error.message || 'The command could not be completed.';
                sayNotice();
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
                this.announceNotice(this.notice);
            } catch (error) {
                const reason = error.message || 'Could not dismiss finished jobs.';
                this.notice = dismissed > 0
                    ? `${dismissed} finished job${dismissed === 1 ? '' : 's'} dismissed before an error: ${reason}`
                    : reason;
                if (refusal) this.notice = `${this.notice.replace(/\.$/, '')}. Not dismissed: ${refusal}`;
                this.announceNotice(this.notice);
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
