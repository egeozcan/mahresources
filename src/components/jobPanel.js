import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { announcePreferenceCommand, openJobPreferenceChannel, preferenceCommand } from '../utils/jobPreferenceChannel.js';
import { captureTrigger, focusedElement, focusFirstIn, focusOn, restoreFocus } from '../utils/focus.js';
import { blockingModal, isRendered } from '../utils/modality.js';
import {
    EVENT_SOURCE_CLOSED,
    advertisedCommands,
    canonicalStreamURL,
    classifyJobState,
    nextStreamRetryDelay,
    commandEndpoint,
    commandLocation,
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
// group. A group with more says so, and so does its badge: the rest are on
// All jobs.
const OPEN_WORK_LIMIT = 50;
// Finished rows follow the deployment's download_cockpit_limit, published on the
// page as a meta tag; this is the fallback when the tag is missing.
const DEFAULT_FINISHED_LIMIT = 10;
// The list API's page ceiling. The boot flag is not bounded like the runtime
// setting, and a page over the ceiling is refused, which would blank the drawer.
const MAX_FINISHED_LIMIT = 200;
const PANEL_REFRESH_MAX_WAIT_MS = 500;
// How long a page waits for its stream to catch up before reading the lists
// anyway. The catch-up schedules the read that counts, so on a healthy stream
// the lists are read once per page load, not twice.
const FIRST_READ_WAIT_MS = 1500;
// A list read that failed is tried again after a delay that doubles from the
// first of these to the last.
const LIST_RETRY_MIN_MS = 2000;
const LIST_RETRY_MAX_MS = 60000;
// Detail reads (a row's commands and outputs) in flight at once while the
// drawer opens, so they leave the browser's per-host connections to the rest
// of the page.
const DETAIL_READ_CONCURRENCY = 4;
// One list page is one bulk dismiss: the server's MaxPageSize and MaxBulkCommandJobs are both 200.
const FINISHED_PAGE_LIMIT = 200;
const FINISHED_STATES = ['succeeded', 'cancelled'];
// Each group is its own bounded page, read by when each job entered its state:
// a job that has just finished or failed leads its group however long ago it
// was accepted. Only open work asks for the progress series: it is up to 120
// points per Job, and a finished row shows no graph.
export function panelGroups(finishedLimit) {
    return [
        { key: 'attention', states: ['blocked', 'failed', 'interrupted'], limit: OPEN_WORK_LIMIT, series: false },
        { key: 'active', states: ['scheduled', 'queued', 'running', 'paused'], limit: OPEN_WORK_LIMIT, series: true },
        { key: 'finished', states: FINISHED_STATES, limit: finishedLimit, series: false },
    ];
}

// Where All jobs shows the rest of a group the drawer caps.
export function panelGroupJobsURL(group, ownerScope = '') {
    const params = new URLSearchParams();
    group.states.forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    if (ownerScope === 'me') params.set('owner', 'me');
    return `/jobs?${params}`;
}

// A count the drawer shows: the rows it holds, marked when the group has more.
export function panelBadgeText(count, more) {
    return more ? `${count}+` : String(count);
}

export function panelFinishedLimit(doc = globalThis.document) {
    const raw = doc?.querySelector?.('meta[name="x-jobs-panel-finished-limit"]')?.getAttribute('content');
    const parsed = Number.parseInt(raw || '', 10);
    return Number.isFinite(parsed) && parsed > 0 ? Math.min(parsed, MAX_FINISHED_LIMIT) : DEFAULT_FINISHED_LIMIT;
}

// The owner line an administrator's drawer shows on somebody else's Job. The
// viewer's own Jobs, and work that never had an owner, show none; a viewer who is
// not an administrator (no viewer id) sees only their own Jobs and is told
// nothing.
export function panelOwnerText(job, viewerId) {
    if (!viewerId) return '';
    if (job?.ownerDeleted) return 'Owner: deleted account';
    const owner = job?.ownerUserId;
    if (owner === null || owner === undefined || Number(owner) === Number(viewerId)) return '';
    return `Owner: ${job.ownerName || `account ${owner}`}`;
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
// technology because the button's aria-label replaces them as its name. Before
// the drawer has read any list it has no counts to give, and says why rather
// than giving zeroes.
export function panelCountsText({
    active = 0, attention = 0, activeMore = false, attentionMore = false,
    loaded = true, failed = false, signedOut = false,
} = {}) {
    if (!loaded) {
        if (signedOut) return 'Signed out; jobs are not shown';
        return failed ? 'Jobs could not be loaded' : 'Loading jobs';
    }
    // "Showing": each group is capped, so these count rows shown, not every job.
    const more = ', more on All jobs';
    const activeText = `${active} active or scheduled job${active === 1 ? '' : 's'}${activeMore ? more : ''}`;
    const attentionText = `${attention} needing attention${attentionMore ? more : ''}`;
    return activeMore ? `Showing ${activeText}, and ${attentionText}` : `Showing ${activeText} and ${attentionText}`;
}

// What the drawer says, and shows in place of its list, once its stream reset.
const STREAM_STOPPED_NOTICE = "Job updates stopped because this server's database was restored or replaced. Reload the page to see current jobs.";

function streamStoppedError() {
    const error = new Error('Job updates stopped.');
    error.streamStopped = true;
    return error;
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
// The lifecycle event types that leave a Job in a state the drawer lists under
// Needs attention or Finished.
const OUTCOME_EVENTS = new Set(['blocked', 'succeeded', 'failed', 'cancelled', 'interrupted', 'not-started']);
// Both live regions replace a message that has not landed within 50 ms. News
// made that close together is said together rather than cancelled.
const NEWS_COALESCE_MS = 50;
// Outcomes the proof store could not keep are counted for this long and then
// said as one message.
const UNSAID_OUTCOMES_COALESCE_MS = 1000;

export function unsaidOutcomesText(count) {
    return count === 1
        ? '1 job finished or needs attention; see the Jobs panel.'
        : `${count} jobs finished or need attention; see the Jobs panel.`;
}

// Where focus goes when the command control that had it leaves the row: its
// counterpart first, since Pin becomes Unpin, then the same command re-rendered.
const COMMAND_COUNTERPARTS = { pin: 'unpin', unpin: 'pin', pause: 'resume', resume: 'pause' };

export function panelFocusSuccessorKeys(key) {
    return [COMMAND_COUNTERPARTS[key], key].filter(Boolean);
}

// Commands whose result the row shows at once: it leaves, or gains or loses
// its pin. They succeed without a box; a screen reader hears the row and what
// happened to it. Every other command keeps the server's words in the box:
// Cancel, Pause and Resume are requests the row may not reflect until the
// executor acts, and lineage pins, forgetting replay input and a plugin's own
// commands have results the row cannot show, partial ones included.
const ROW_SHOWN_COMMANDS = { dismiss: 'dismissed', pin: 'pinned', unpin: 'unpinned' };

function commandDoneText(job, command) {
    return `${job?.title || job?.kind || 'Job'} ${ROW_SHOWN_COMMANDS[command?.key]}.`;
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
        // Set when the stream reset: see stopForStreamReset.
        streamStopped: false,
        connectionStatus: 'disconnected',
        // Why the last list read failed, while no read since has succeeded.
        error: '',
        notice: '',
        // Whether any list read has succeeded: until one has, the drawer has no
        // counts to show and says so, rather than showing zeroes.
        loaded: false,
        // Set while the server answers that the session has ended.
        signedOut: false,
        // '' lists every Job the viewer may see, 'me' only the viewer's own:
        // an administrator's drawer can hold either. Lists and stream follow it.
        ownerScope: '',
        _resourceRefreshNotified: new Set(),
        busy: false,
        finishedLimit: DEFAULT_FINISHED_LIMIT,
        // Whether each group's list had more than it holds.
        groupHasMore: { attention: false, active: false, finished: false },
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
        // Every list read is numbered as it starts. Its rows apply unless a read
        // that started later has already applied, or a read started no later
        // than the floor, which a dismissal and a scope change raise so that
        // nothing read before them puts their rows back.
        _refreshGeneration: 0,
        _appliedGeneration: 0,
        _refreshFloor: 0,
        _failedGeneration: 0,
        _firstReadTimer: null,
        _listRetryTimer: null,
        _listRetryDelay: 0,
        _streamRetryTimer: null,
        _streamRetryDelay: 0,
        // Detail reads: the rows in flight, the rows waiting for one of the
        // DETAIL_READ_CONCURRENCY readers the whole drawer shares, and the pass
        // those readers are working through (see loadStaleDetails).
        _detailReads: new Set(),
        _detailQueue: [],
        _detailReaders: 0,
        _detailOutcomes: new Set(),
        _detailDrain: null,
        _resolveDetailDrain: null,
        _detailLoad: null,
        // A pin moves no version, so a read cannot tell by version whether it
        // predates a preference change. Each Job's epoch moves when a change is
        // seen (this tab's command, another page's announcement, a list read
        // that shows a different pin), and a read begun at an older epoch is
        // older than what the drawer holds.
        _preferenceEpochs: new Map(),
        _broadcast: null,
        // Set once the stream has given a cursor (a catch-up), which a reopened
        // stream then resumes from, even v2:0.
        _holdsCursor: false,
        // Set by destroy(): nothing is read, retried or reopened after it.
        _destroyed: false,
        _onlineHandler: null,
        _visibilityHandler: null,
        _streamGeneration: 0,
        // What the reader has been told about each job: its state and version,
        // and the stream generation that was current when it was recorded. See
        // hearJob.
        _heard: new Map(),
        // Versions a live lifecycle event proved news before any read saw them.
        // See boundLiveProofs.
        _liveVersions: new Map(),
        // Outcomes the proof store dropped before their jobs were heard, not
        // yet said.
        _unsaidOutcomes: 0,
        _unsaidOutcomesTimer: null,
        // The count most recently said, until the message carrying it has
        // landed (see announce).
        _countNews: null,
        _landTimer: null,
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
        _ownerViewer: 0,

        init() {
            this.finishedLimit = panelFinishedLimit();
            // Set on an administrator's drawer only: whose Job a row is matters
            // when the drawer lists every account's.
            this._ownerViewer = Number(this.$el?.dataset?.jobPanelViewer) || 0;
            this.ownerScope = this.$el?.dataset?.jobPanelOwnerScope === 'me' ? 'me' : '';
            this._liveRegion = createLiveRegion();
            this._trigger = this.$el?.querySelector?.('.job-panel-trigger') || null;
            this._root = this.$el || null;
            this._keydownHandler = event => this.handleShortcut(event);
            this._panelOpenHandler = event => this.openFromEvent(event.detail);
            document.addEventListener('keydown', this._keydownHandler);
            window.addEventListener('jobs-panel-open', this._panelOpenHandler);
            // A network that comes back, or a tab the reader returns to, is
            // the moment to try again rather than wait out a backoff: the
            // reader may have signed in again in another tab.
            this._onlineHandler = () => this.retryNow();
            this._visibilityHandler = () => { if (document.visibilityState === 'visible') this.retryNow(); };
            window.addEventListener('online', this._onlineHandler);
            document.addEventListener('visibilitychange', this._visibilityHandler);
            this._broadcast = openJobPreferenceChannel(message => this.hearPreferenceBroadcast(message));
            this.$watch?.('isOpen', open => {
                if (open) {
                    this.startClock();
                    // A list read that failed is tried again as the reader
                    // opens the drawer to look; otherwise only the rows whose
                    // commands the drawer does not hold yet are read.
                    if (this.error || this._listRetryTimer) this.retryNow();
                    else this.loadStaleDetails();
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
            // The lists are read once the stream has caught up, which is at
            // once now that a fresh page replays nothing; a stream that has not
            // caught up by FIRST_READ_WAIT_MS does not hold the lists back.
            this.connect();
            if (this.eventSource) {
                this._firstReadTimer = setTimeout(() => {
                    this._firstReadTimer = null;
                    this.startScheduledPanelRefresh();
                }, FIRST_READ_WAIT_MS);
            } else {
                this.refresh();
            }
        },

        destroy() {
            this._destroyed = true;
            if (this._keydownHandler) document.removeEventListener('keydown', this._keydownHandler);
            if (this._panelOpenHandler) window.removeEventListener('jobs-panel-open', this._panelOpenHandler);
            if (this._onlineHandler) window.removeEventListener('online', this._onlineHandler);
            if (this._visibilityHandler) document.removeEventListener('visibilitychange', this._visibilityHandler);
            this._broadcast?.close();
            this._broadcast = null;
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this.stopClock();
            this.clearRetryTimers();
            this._refreshGeneration += 1;
            this._refreshFloor = this._refreshGeneration;
            this._streamGeneration += 1;
            this._panelRefreshRequested = false;
            this.eventSource?.close();
            this.eventSource = null;
            this._liveRegion?.destroy();
            clearTimeout(this._drawerAnnounceTimer);
            clearTimeout(this._unsaidOutcomesTimer);
            clearTimeout(this._landTimer);
        },

        clearRetryTimers() {
            clearTimeout(this._firstReadTimer);
            clearTimeout(this._listRetryTimer);
            clearTimeout(this._streamRetryTimer);
            this._firstReadTimer = null;
            this._listRetryTimer = null;
            this._streamRetryTimer = null;
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
        // A group that holds fewer rows than its list had carries `more`, says
        // so (`moreText`), and links to where All jobs shows the rest.
        get groups() {
            const lists = new Map(panelGroups(this.finishedLimit).map(group => [group.key, group]));
            return [
                { key: 'attention', title: 'Needs attention', jobs: this.attentionJobs, moreLabel: 'See every job that needs attention' },
                { key: 'active', title: 'Active and scheduled', jobs: this.activeJobs, moreLabel: 'See every active and scheduled job' },
                { key: 'finished', title: 'Finished', jobs: this.finishedJobs, moreLabel: 'See every finished job' },
            ].filter(group => group.jobs.length > 0).map(group => ({
                ...group,
                more: !!this.groupHasMore[group.key],
                countText: panelBadgeText(group.jobs.length, !!this.groupHasMore[group.key]),
                moreText: group.key === 'finished'
                    ? `Showing the ${group.jobs.length} most recently finished.`
                    : `Showing the ${group.jobs.length} most recent.`,
                moreURL: panelGroupJobsURL(lists.get(group.key), this.ownerScope),
            }));
        },
        get activeCount() { return this.counts.active; },
        get attentionCount() { return this.counts.attention; },
        get activeBadge() { return panelBadgeText(this.counts.active, this.groupHasMore.active); },
        get attentionBadge() { return panelBadgeText(this.counts.attention, this.groupHasMore.attention); },
        get finishedHasMore() { return !!this.groupHasMore.finished; },
        get countsText() {
            return panelCountsText({
                ...this.counts,
                activeMore: this.groupHasMore.active, attentionMore: this.groupHasMore.attention,
                loaded: this.loaded, failed: !!this.error, signedOut: this.signedOut,
            });
        },
        get connectionText() {
            if (this.connectionStatus === 'stopped') return 'Live updates stopped';
            if (this.signedOut) return 'Signed out';
            if (this.connectionStatus === 'connected') return 'Live updates connected';
            if (this.connectionStatus === 'reconnecting') return 'Reconnecting';
            return 'Connecting to live updates';
        },
        // Back to this page once signed in again.
        get signInURL() {
            const here = `${globalThis.location?.pathname || '/'}${globalThis.location?.search || ''}`;
            return `/login?next=${encodeURIComponent(here)}`;
        },
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
            } else {
                this._liveRegion?.cancel?.();
                inside.textContent = '';
                this._drawerAnnounceTimer = setTimeout(() => {
                    this._drawerAnnounceTimer = null;
                    const region = this._drawerAnnouncer();
                    if (region) region.textContent = message;
                    else this._liveRegion?.announce(message);
                }, 50);
            }
            // Timers of one delay run in the order they were set, so this one
            // runs once the region has put the message in place: from then on a
            // count it carried has been spoken.
            clearTimeout(this._landTimer);
            this._landTimer = setTimeout(() => {
                this._landTimer = null;
                this._countNews = null;
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

        // Every request the drawer makes goes through here, so this is where a
        // stopped drawer is fenced: nothing is sent once it stopped, and an
        // answer to a request sent before is refused rather than returned, so
        // no continuation after an await applies it or announces anything.
        // Callers' failure paths check streamStopped before they say anything.
        async requestJSON(url, init = {}) {
            if (this.streamStopped) throw streamStoppedError();
            const response = await fetch(url, {
                ...init,
                headers: { Accept: 'application/json', ...(init.headers || {}) },
            });
            const payload = await response.json().catch(() => ({}));
            if (this.streamStopped) throw streamStoppedError();
            if (!response.ok) {
                const error = new Error(payload.error || `Request failed (${response.status})`);
                error.status = response.status;
                error.payload = payload;
                throw error;
            }
            this.movePreferenceEpochs(preferenceCommand(url, init, payload)?.jobIds);
            announcePreferenceCommand(url, init, payload, this._broadcast);
            return payload;
        },

        // A page of this viewer (another tab, or a Job page in this one)
        // dismissed, pinned or forgot these jobs. Dismissed rows leave at once,
        // as they do in the tab that dismissed them, and every read begun
        // before the change is fenced, so none of them puts a row back.
        hearPreferenceBroadcast(message) {
            if (this.streamStopped || this._destroyed) return;
            const ids = new Set(Array.isArray(message?.jobIds) ? message.jobIds.map(String) : []);
            this.movePreferenceEpochs([...ids]);
            for (const id of ids) delete this.details[id];
            if (message?.command === 'dismiss') this.jobs = this.jobs.filter(job => !ids.has(String(job.id)));
            this.refresh();
        },

        // Reads the lists now, superseding every read already in flight: what
        // they read is older than whatever made this read necessary (a
        // dismissal, a scope change, the reader asking). The reads the stream
        // schedules go through startScheduledPanelRefresh and supersede nothing.
        async refresh() {
            if (this.streamStopped || this._destroyed) return;
            this.fenceEarlierReads();
            const generation = ++this._refreshGeneration;
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this._panelRefreshRequested = false;
            return this.refreshAtGeneration(generation);
        },

        fenceEarlierReads() {
            this._refreshFloor = this._refreshGeneration;
        },

        movePreferenceEpochs(ids) {
            for (const id of ids || []) this._preferenceEpochs.set(String(id), this.preferenceEpoch(id) + 1);
        },

        preferenceEpoch(id) {
            return this._preferenceEpochs.get(String(id)) || 0;
        },

        // Whether a list read's answer may still apply: no read that started
        // after it has applied, and nothing since it started has fenced it. A
        // slow read is not thrown away because a newer one is still on its way:
        // rows from the older answer beat an empty drawer, and the newer answer
        // replaces them when it lands.
        readStillCurrent(generation) {
            return generation > this._appliedGeneration && generation > this._refreshFloor;
        },

        async refreshAtGeneration(generation) {
            const streamGeneration = this._streamGeneration;
            const touchedFrom = this._streamTouchSeq;
            const ownerScope = this.ownerScope;
            const groups = panelGroups(this.finishedLimit);
            let pages;
            try {
                pages = await Promise.all(groups.map(group => this.requestJSON(buildPanelListURL(group, ownerScope))));
            } catch (error) {
                // Only the newest read's failure is the drawer's state: an older
                // one failing says nothing a newer answer will not.
                if (!this.streamStopped && generation === this._refreshGeneration) this.listReadFailed(error, generation);
                return;
            }
            if (!this.readStillCurrent(generation) || ownerScope !== this.ownerScope) return;
            this._appliedGeneration = generation;
            this.listReadSucceeded(generation);
            const byId = new Map();
            const hasMore = {};
            pages.forEach((payload, index) => {
                hasMore[groups[index].key] = !!payload.nextCursor;
                for (const job of payload.jobs || []) if (!byId.has(job.id)) byId.set(job.id, job);
            });
            this.groupHasMore = hasMore;
            // A list read before a stream snapshot the panel has since
            // applied is older than the row it would replace; the newer row
            // stays, rather than the row rolling back.
            const shown = new Map(this.jobs.map(job => [job.id, job]));
            // A pin this read shows differently from the row was changed
            // somewhere the drawer did not hear (another browser).
            this.movePreferenceEpochs([...byId.values()]
                .filter(job => shown.has(job.id) && Object.hasOwn(job, 'pinned') && !!shown.get(job.id).pinned !== !!job.pinned)
                .map(job => job.id));
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
            // One announcement for everything the lists found, said now: two
            // made in a row would each cancel the one before it. Detail reads
            // that follow say their own news, so none of this waits on them.
            const spoken = [];
            for (const job of listed) this.hearFromRead(job, streamGeneration, spoken);
            const nextJobs = this.bounded(listed);
            nextJobs.forEach(job => this.trackResourceCompletion(job));
            this.jobs = nextJobs;
            this.forgetDetailsOfGoneRows();
            this.announceNews(spoken);
            // Not awaited: a read never waits on detail reads, so one that
            // stalls holds up neither the next read nor anything after it.
            this._detailLoad = this.loadStaleDetails();
        },

        // Rows from a read that started before a failed one still apply, but
        // they are older than what the failed read was for: the failure, and
        // the retry it scheduled, stand until a read started after it succeeds.
        listReadSucceeded(generation) {
            this.loaded = true;
            if (generation < this._failedGeneration) return;
            this.error = '';
            this.signedOut = false;
            this._listRetryDelay = 0;
            clearTimeout(this._listRetryTimer);
            this._listRetryTimer = null;
        },

        // A failed read keeps the rows the drawer holds and says why above
        // them, and the drawer reads again after a delay that doubles up to
        // LIST_RETRY_MAX_MS, and at once when the reader opens the drawer, asks
        // to, or comes back to the tab. A 401 means the session ended: the
        // drawer says so and offers to sign in rather than a server message.
        listReadFailed(error, generation) {
            this._failedGeneration = generation;
            this.signedOut = error?.status === 401;
            this.error = this.signedOut ? '' : error?.message || 'Could not load jobs.';
            this.scheduleListRetry();
        },

        scheduleListRetry() {
            if (this.streamStopped || this._destroyed || this._listRetryTimer) return;
            const delay = this._listRetryDelay || LIST_RETRY_MIN_MS;
            this._listRetryDelay = Math.min(delay * 2, LIST_RETRY_MAX_MS);
            this._listRetryTimer = setTimeout(() => {
                this._listRetryTimer = null;
                this.startScheduledPanelRefresh();
            }, delay);
        },

        // Try again now: the reader asked, opened the drawer, or came back.
        retryNow() {
            if (this.streamStopped) return;
            if (!this.eventSource) {
                clearTimeout(this._streamRetryTimer);
                this._streamRetryTimer = null;
                this.connect();
            }
            if (this.error || this.signedOut || this._listRetryTimer || !this.loaded) {
                clearTimeout(this._listRetryTimer);
                this._listRetryTimer = null;
                this.startScheduledPanelRefresh();
            }
        },

        // A row's detail (its commands and outputs) is read only while the
        // drawer is open, where the controls it carries are shown, and only
        // again once the row's version or its pin has moved on: a detail read
        // for every row on every refresh was most of what a page load cost.
        // A command is re-checked by the server against the version it names,
        // so an offer read for an older version is refused, not run.
        detailStale(job) {
            const detail = this.details[job?.id];
            return !detail || Number(detail.version || 0) < Number(job.version || 0) ||
                (Object.hasOwn(job, 'pinned') && !!detail.pinned !== !!job.pinned);
        },

        forgetDetailsOfGoneRows() {
            const shown = new Set(this.jobs.map(job => job.id));
            for (const id of Object.keys(this.details)) if (!shown.has(id)) delete this.details[id];
        },

        // Queues the rows whose detail is stale and starts readers for them, at
        // most DETAIL_READ_CONCURRENCY across every pass, and answers when the
        // pass in progress has drained. A pass that ends with a failed read
        // tries again after the list retry's delay; one that ends with a row
        // that moved on while it was read starts another.
        loadStaleDetails() {
            if (!this.isOpen || this.streamStopped || this._destroyed) return this._detailDrain || Promise.resolve();
            for (const job of this.jobs) {
                if (advertisedCommands(job).length) {
                    // A row that carries its commands (a stream snapshot or a
                    // command's answer) is its own detail.
                    if (this.detailStale(job)) this.details[job.id] = job;
                    continue;
                }
                if (this.detailStale(job) && !this._detailReads.has(job.id) && !this._detailQueue.includes(job.id)) {
                    this._detailQueue.push(job.id);
                }
            }
            while (this._detailReaders < DETAIL_READ_CONCURRENCY && this._detailQueue.length) this.startDetailReader();
            return this._detailDrain || Promise.resolve();
        },

        startDetailReader() {
            this._detailReaders += 1;
            if (!this._detailDrain) this._detailDrain = new Promise(resolve => { this._resolveDetailDrain = resolve; });
            (async () => {
                for (let id = this._detailQueue.shift(); id !== undefined; id = this._detailQueue.shift()) {
                    const job = this.jobs.find(row => row.id === id);
                    if (!job || !this.detailStale(job) || this.streamStopped || this._destroyed) continue;
                    this._detailOutcomes.add(await this.loadDetail(job));
                }
                this._detailReaders -= 1;
                if (this._detailReaders === 0) this.finishDetailPass();
            })();
        },

        finishDetailPass() {
            const outcomes = this._detailOutcomes;
            const resolve = this._resolveDetailDrain;
            this._detailOutcomes = new Set();
            this._detailDrain = null;
            this._resolveDetailDrain = null;
            if (!this.streamStopped && !this._destroyed) {
                if (outcomes.has('failed')) this.scheduleListRetry();
                else if (outcomes.has('moved')) this.loadStaleDetails();
            }
            resolve?.();
        },

        // Reads one row's detail and merges it into the row, heard as any read
        // is. Answers 'moved' when the row moved on while it was read, and
        // 'failed' when the read failed or answered older than the row it was
        // asked for (a lagging replica): the drawer tries those again after a
        // delay rather than at once, or a replica that stays behind would be
        // asked in a loop.
        async loadDetail(job) {
            this._detailReads.add(job.id);
            const streamGeneration = this._streamGeneration;
            const askedFor = Number(job.version || 0);
            const epoch = this.preferenceEpoch(job.id);
            let detail;
            try {
                detail = await this.requestJSON(`/v1/jobs/${encodeURIComponent(job.id)}`);
            } catch {
                return 'failed';
            } finally {
                this._detailReads.delete(job.id);
            }
            const current = this.jobs.find(row => row.id === job.id);
            // A row a newer list read dropped stays dropped.
            if (!current) return 'read';
            // A detail older than the row shown would roll it back. A pin
            // moves no version, so a pin that changed while the detail was read
            // makes it older too, however its version compares.
            if (Number(detail.version || 0) < Number(current.version || 0)) {
                return Number(current.version || 0) > askedFor ? 'moved' : 'failed';
            }
            if (this.preferenceEpoch(job.id) !== epoch) return 'moved';
            this.details[job.id] = detail;
            const spoken = [];
            this.hearFromRead({ ...current, ...detail }, streamGeneration, spoken);
            this.jobs = this.bounded(this.jobs.map(row => row.id === job.id ? { ...row, ...detail } : row));
            this.announceNews(spoken);
            return 'read';
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
        // The first sight of a job (no entry) is news only when the job has
        // already reached an outcome, a state the drawer lists under Needs
        // attention or Finished, and a live lifecycle event shows it got there
        // after this page connected. A job's whole life can fit in one publish
        // tick, so its first read is often its outcome, and nothing after it
        // would say anything. Work still open at its first sight stays unsaid,
        // since its outcome will be said as a change when it comes.
        // The proof works as it does for a change: a live event before the read
        // marks the version (provenLive), and a read before the live event
        // withholds the outcome over every version up to its own, for the event
        // to release. What finished before the page connected has no live event
        // and stays silent.
        // Every path that puts a row on screen hears it; a row set on screen
        // some other way counts as heard in its shown state on the first stream
        // generation, before any stream was connected.
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
            const firstSeenOutcome = !entry && ['attention', 'finished'].includes(classifyJobState(job));
            const proof = this._liveVersions.get(job.id);
            const liveFrom = proof ? proof.version : undefined;
            const provenLive = liveFrom !== undefined && version >= liveFrom;
            let said = this.streamCaughtUp && (changed || firstSeenOutcome) && (
                proofOnly ? provenLive
                    : live && (!sameGenerationOnly || (changed && entry.generation === generation) || provenLive)
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
            // Only an observation that could speak uses the proof up; a stale
            // read must leave it for the live one that follows. What it says
            // goes back to the caller, which says it: a read at once, a command
            // with its own notice.
            if (provenLive && this.streamCaughtUp && (live || proofOnly)) this._liveVersions.delete(job.id);
            // It stays while the job is still in that state, whatever versions a
            // same-state change (a control request) adds; saying it, or a change
            // of state, retires it.
            let withheld = !said && withheldIn
                ? { text: withheldIn.withheld, from: withheldIn.withheldFrom, to: withheldIn.withheldVersion } : null;
            if (sameGenerationOnly && (changed || firstSeenOutcome) && !said) {
                withheld = { text: lifecycleAnnouncement(job), from: entry?.version ?? 0, to: version };
            }
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
            const previous = this._liveVersions.get(jobId);
            this._liveVersions.delete(jobId);
            this._liveVersions.set(jobId, {
                version: Math.max(version, previous?.version || 0),
                outcome: OUTCOME_EVENTS.has(message.type),
            });
            this.boundLiveProofs();
            return '';
        },

        // A proof leaves the store in three ways only: its job is heard (hearJob
        // uses it up), the store is past HEARD_LIMIT, or the stream drops
        // (dropStream), which counts every outcome still owed. Nothing else
        // drops one: not a list read that left the job out, which a later read
        // after a dismissal may list, and not a failed read. When the store is full, a proof that is not an outcome
        // goes first: without it
        // a job's first read withholds its outcome, and the job's own outcome
        // event, still to come, releases it. Only when every proof is an
        // outcome does the oldest go, and that outcome is counted and said
        // with the others in one message (sayUnsaidOutcomes) rather than lost
        // without a word. Every one is counted: a proof still in the store has
        // not been used up, so nothing has said its outcome, and whether a
        // later read could say it depends on the reconnects between. If one
        // does, the job is heard twice, once in the count, which is the lesser
        // harm.
        boundLiveProofs() {
            while (this._liveVersions.size > HEARD_LIMIT) {
                let dropped = null;
                let oldestOutcome = null;
                for (const [jobId, proof] of this._liveVersions) {
                    if (!proof.outcome) { dropped = jobId; break; }
                    oldestOutcome ??= jobId;
                }
                if (dropped === null) {
                    dropped = oldestOutcome ?? this._liveVersions.keys().next().value;
                    this.countUnsaidOutcome();
                }
                this._liveVersions.delete(dropped);
            }
        },

        countUnsaidOutcome() {
            if (this.streamStopped) return;
            this._unsaidOutcomes += 1;
            if (!this._unsaidOutcomesTimer) {
                this._unsaidOutcomesTimer = setTimeout(() => this.sayUnsaidOutcomes(), UNSAID_OUTCOMES_COALESCE_MS);
            }
        },

        sayUnsaidOutcomes() {
            this._unsaidOutcomesTimer = null;
            // A count still on its way to the region is replaced, so it is added in.
            const count = this._unsaidOutcomes + (this._countNews?.count || 0);
            this._unsaidOutcomes = 0;
            if (count > 0) this.say([{ jobId: null, count, text: unsaidOutcomesText(count) }]);
        },

        // A list or detail read, which may speak only if no reconnect happened
        // since the read began. What it says joins `spoken`, for the caller to
        // say.
        hearFromRead(job, streamGeneration, spoken) {
            const said = this.hearJob(job, {
                live: streamGeneration === this._streamGeneration, sameGenerationOnly: true, generation: streamGeneration,
            });
            if (said) spoken.push(this.newsEntry(job.id, said));
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
        // request, leave it true. One entry per job, the latest. A count of
        // outcomes that could not be said one by one belongs to no job and
        // stays true.
        currentNews(entries) {
            const byJob = new Map();
            for (const entry of entries) {
                if (entry.jobId === null) {
                    byJob.delete(null);
                    byJob.set(null, entry);
                    continue;
                }
                const heard = this._heard.get(entry.jobId);
                const newer = this._liveVersions.get(entry.jobId)?.version;
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
        // a pending one; news accumulates, less anything superseded. A count of
        // outcomes is carried until its message has actually landed, whatever
        // the clock says and across a drop: nothing else would say it again.
        say(entries = [], notice = '') {
            // A stopped drawer says nothing about Jobs: they came from the
            // database a reset replaced. A notice, such as the stop itself, is
            // still said.
            if (this.streamStopped) entries = [];
            const carried = this._countNews ? [this._countNews] : [];
            const news = this.currentNews([...carried, ...this.pendingNews(), ...entries]);
            const text = notice || this.pendingNotice();
            this._countNews = news.find(entry => entry.jobId === null) || null;
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
            if (this.streamStopped) return;
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
            if (this.streamStopped || this._destroyed) return;
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

        connect() {
            if (this.eventSource || this.streamStopped || this._destroyed || typeof EventSource === 'undefined') return;
            clearTimeout(this._streamRetryTimer);
            this._streamRetryTimer = null;
            if (this.connectionStatus !== 'reconnecting') this.connectionStatus = 'connecting';
            // What a reconnect replays is recorded, not said: it arrives
            // before the catch-up.
            const source = new EventSource(canonicalStreamURL(this.lastSequence, this.ownerScope, this._holdsCursor));
            this.eventSource = source;
            // A source this drawer has replaced (a scope change, a retry) may
            // still deliver what it had queued; only the current one counts.
            const current = handler => event => { if (this.eventSource === source) handler(event); };
            source.addEventListener('open', current(() => { this.connectionStatus = 'connected'; }));
            source.addEventListener('error', current(() => this.handleStreamError(source)));
            source.addEventListener('job-caught-up', current(event => this.markStreamCaughtUp(event)));
            source.addEventListener('job-progress', current(event => this.handleProgressFrame(event)));
            for (const eventName of ['message', 'job']) {
                source.addEventListener(eventName, current(event => this.handleStreamMessage(event)));
            }
        },

        // The browser reconnects a stream that dropped, but not one answered
        // with anything other than 200 (a proxy's 502, a 401 once the session
        // ended): it closes it for good. The drawer then opens a new one itself,
        // after a growing delay (nextStreamRetryDelay), and reads the lists
        // meanwhile, which also says whether the session ended.
        handleStreamError(source) {
            this.dropStream();
            if (source.readyState !== EVENT_SOURCE_CLOSED) return;
            source.close();
            this.eventSource = null;
            this.scheduleStreamRetry();
            this.startScheduledPanelRefresh();
        },

        scheduleStreamRetry() {
            if (this.streamStopped || this._destroyed || this._streamRetryTimer) return;
            const delay = this._streamRetryDelay = nextStreamRetryDelay(this._streamRetryDelay);
            this._streamRetryTimer = setTimeout(() => {
                this._streamRetryTimer = null;
                this.connect();
            }, delay);
        },

        // Lists every Job the viewer may see (''), or only their own ('me').
        // The stream is reopened with the new scope, resuming from its cursor,
        // and the lists are read again; reads for the old scope never apply.
        // What the new scope brings in is recorded, not said: the stream
        // generation changes, as on a reconnect.
        setOwnerScope(scope) {
            const next = scope === 'me' ? 'me' : '';
            if (next === this.ownerScope || this.streamStopped) return;
            this.ownerScope = next;
            const source = this.eventSource;
            this.eventSource = null;
            source?.close();
            this.dropStream();
            this.connect();
            this.refresh();
        },

        // A live progress frame updates the row it names in place. It is not a
        // lifecycle event: it never refetches the list, never inserts a row the
        // list did not return, and is never announced — a screen reader told
        // every second that a download moved would hear nothing else.
        handleProgressFrame(event) {
            if (this.streamStopped) return;
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

        // The stream dropped. What happens before it catches up again arrives as
        // replay, so no proof crosses into the next stream generation: a later
        // read cannot tell a transition proved live from one made while
        // disconnected. An outcome still unheard was delivered live, though,
        // so it goes into the count (sayUnsaidOutcomes) rather than silently.
        // A list read in flight still applies its rows: what it saw is history
        // to the stream that follows, and is recorded as such (hearFromRead).
        dropStream() {
            this.connectionStatus = 'reconnecting';
            this.streamCaughtUp = false;
            this._streamGeneration += 1;
            for (const proof of this._liveVersions.values()) {
                if (proof.outcome) this.countUnsaidOutcome();
            }
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
            if (boundary.reset === true) {
                this.stopForStreamReset();
                return;
            }
            const wasCaughtUp = this.streamCaughtUp;
            this.lastSequence = Math.max(this.lastSequence, sequence);
            this._holdsCursor = true;
            this.streamCaughtUp = true;
            this._streamGeneration += 1;
            this._streamRetryDelay = 0;
            // The read after a catch-up is the one that counts: it replaces the
            // first page load's fallback.
            clearTimeout(this._firstReadTimer);
            this._firstReadTimer = null;
            if (!wasCaughtUp) this.schedulePanelRefresh();
        },

        // A reset stream is served by a database that did not issue the cursor
        // this drawer resumed from: one restored from an older backup, or wiped.
        // The drawer sits on every page, and the page around it may hold input
        // nobody has saved, so it does not reload the page: it stops. It closes
        // the stream, drops every row and every read in flight, reads and sends
        // nothing more, and says so with a way to reload. Nothing it held is
        // repaired in place, since all of it came from the other database.
        stopForStreamReset() {
            this.eventSource?.close();
            this.eventSource = null;
            this.streamStopped = true;
            this.streamCaughtUp = false;
            this.connectionStatus = 'stopped';
            this._refreshGeneration += 1;
            this._refreshFloor = this._refreshGeneration;
            this._streamGeneration += 1;
            this.clearRetryTimers();
            if (this._panelRefreshTimer) clearTimeout(this._panelRefreshTimer);
            if (this._panelRefreshMaxTimer) clearTimeout(this._panelRefreshMaxTimer);
            this._panelRefreshTimer = null;
            this._panelRefreshMaxTimer = null;
            this._panelRefreshRequested = false;
            this.jobs = [];
            this.details = {};
            this.groupHasMore = { attention: false, active: false, finished: false };
            this.notice = '';
            this.error = '';
            this.signedOut = false;
            // What the reader was, or was about to be, told about Jobs belongs to
            // the other database too: the ledger, the proofs, every message not
            // yet landed and every timer that would say one. Only the stop is
            // said, and say() says nothing about a Job from here on.
            clearTimeout(this._drawerAnnounceTimer);
            clearTimeout(this._unsaidOutcomesTimer);
            clearTimeout(this._landTimer);
            this._drawerAnnounceTimer = null;
            this._unsaidOutcomesTimer = null;
            this._landTimer = null;
            this._unsaidOutcomes = 0;
            this._countNews = null;
            this._recentNews = [];
            this._recentNotice = '';
            this._newsAt = 0;
            this._heard = new Map();
            this._liveVersions = new Map();
            this._streamTouched = new Map();
            this._liveRegion?.cancel?.();
            const inside = this._drawerAnnouncer();
            if (inside) inside.textContent = '';
            this.announceNotice(STREAM_STOPPED_NOTICE);
        },

        reloadPage() {
            globalThis.location?.reload?.();
        },

        async handleStreamMessage(event) {
            if (this.streamStopped) return;
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
        ownerText(job) { return panelOwnerText(job, this._ownerViewer); },

        async refreshJobPreference(id, spoken = null) {
            const epoch = this.preferenceEpoch(id);
            const payload = await this.requestJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            const freshJob = payload.job || payload;
            // A preference changed elsewhere while this was read: the answer is
            // older than the row, and a fresh read replaces it.
            if (this.preferenceEpoch(id) !== epoch) {
                delete this.details[id];
                this.startScheduledPanelRefresh();
                return freshJob;
            }
            if (freshJob?.id) {
                this.details[id] = freshJob;
                // A read: any change of state in it is someone else's.
                this.applyStreamSnapshot(freshJob, false, false, spoken, { asRead: true });
            }
            return freshJob;
        },

        async runCommand(job, command) {
            const watch = this.watchReaderFocus();
            this._commandRefresh = null;
            try {
                return await this.runCommandUnfocused(job, command);
            } finally {
                // A Dismiss's refresh may reveal the job that takes the freed
                // place; focus is placed once it has.
                this.keepFocusOnRow(job.id, command, watch, this._commandRefresh);
                this._commandRefresh = null;
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
            // The rows in order, for when the command removes its own row.
            const rows = [...document.querySelectorAll('#job-center-panel article[data-job-id]')].map(row => row.dataset.jobId);
            const watch = { opener, rows, movedByReader: false, stop: () => {} };
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
        keepFocusOnRow(jobId, command, watch, settled = null) {
            if (!watch) return;
            Promise.resolve(settled).catch(() => {}).then(() => this.$nextTick?.(() => setTimeout(() => {
                watch.stop();
                if (watch.opener.isConnected || watch.movedByReader || !this.isOpen) return;
                const panel = document.querySelector('#job-center-panel');
                const active = document.activeElement;
                if (active && active !== document.body && !panel?.contains(active)) return;
                const rowFor = id => panel?.querySelector(`article[data-job-id="${CSS.escape(String(id))}"]`);
                const row = rowFor(jobId);
                if (!row) {
                    // The command took its row away (Dismiss): the row that took
                    // its place, else the one before it, else All jobs.
                    const at = watch.rows.indexOf(String(jobId));
                    const neighbours = [...watch.rows.slice(at + 1), ...watch.rows.slice(0, Math.max(at, 0)).reverse()];
                    const links = [
                        ...neighbours.map(id => rowFor(id)?.querySelector('a[href]')),
                        // One the refresh revealed, when no neighbour is left.
                        panel?.querySelector('article[data-job-id] a[href]'),
                        panel?.querySelector('[data-job-panel-all-jobs]'),
                    ];
                    for (const link of links) {
                        if (link && isRendered(link) && focusOn(link)) return;
                    }
                    return;
                }
                const candidates = [
                    ...panelFocusSuccessorKeys(command?.key).map(key => row.querySelector(`button[data-command-key="${CSS.escape(key)}"]`)),
                    row.querySelector('details[open] summary'),
                    row.querySelector('[role="group"] button, [role="group"] summary'),
                    row.querySelector('a[href]'),
                ];
                for (const candidate of candidates) {
                    if (candidate && isRendered(candidate) && focusOn(candidate)) return;
                }
            }, 0)));
        },

        async runCommandUnfocused(job, command) {
            const confirmation = panelCommandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(confirmation, {
                    title: commandLabel(command), confirmLabel: commandLabel(command),
                });
                if (!accepted) return null;
            }
            // A confirmation answered after the drawer stopped would send a
            // command about a Job the other database described.
            if (this.streamStopped) return null;
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
                    if (this.streamStopped) return null;
                }
                // Dismissing records a preference and emits no job event, so no
                // refresh would take the row away: it leaves now, and a fresh
                // refresh supersedes any that read the lists before the dismissal
                // and brings in whatever the freed place makes room for.
                if (command?.key === 'dismiss') {
                    this.jobs = this.jobs.filter(row => row.id !== job.id);
                    this._commandRefresh = this.refresh();
                }
                const successorId = outcome.successorId || outcome.successorID || result.successorId || result.successorID;
                const location = successorId ? `/job?id=${encodeURIComponent(successorId)}` : commandLocation(outcome);
                if (location && !this.streamStopped) globalThis.location?.assign?.(location);
                const rowShowsIt = Object.hasOwn(ROW_SHOWN_COMMANDS, command?.key);
                this.notice = preferenceRefreshFailed
                    ? `${commandLabel(command)} completed. Reload this job to see its current pin status.`
                    : rowShowsIt ? '' : outcome.message || `${commandLabel(command)} requested.`;
                this.announceNotice(this.notice || commandDoneText(job, command), proved);
                return outcome;
            } catch (error) {
                if (this.streamStopped) return null;
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
            if (this.busy || this.streamStopped || this.finishedCount === 0) return { dismissed: 0, total: 0 };
            this.busy = true;
            let dismissed = 0;
            let total = 0;
            let refusal = '';
            try {
                let cursor = '';
                do {
                    const page = await this.requestJSON(buildFinishedPageURL(cursor, this.ownerScope));
                    const jobIds = (page.jobs || []).map(job => job.id);
                    if (jobIds.length) {
                        const key = commandKey();
                        const payload = await this.requestJSON('/v1/jobs/commands/dismiss', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                            body: JSON.stringify({ jobIds, idempotencyKey: key }),
                        });
                        // A refresh issued before this dismissal committed
                        // would put its rows back; the fence discards it. One
                        // issued from here on already sees the dismissal.
                        this.fenceEarlierReads();
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
                // Full success shows no box: the rows leaving say it.
                this.notice = dismissed === total
                    ? ''
                    : `${dismissed} of ${total} finished job${plural} dismissed. Not dismissed: ${refusal}`;
                this.announceNotice(this.notice || `${dismissed} finished job${plural} dismissed.`);
            } catch (error) {
                if (this.streamStopped) {
                    this.busy = false;
                    return { dismissed, total };
                }
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

function buildPanelListURL(group, ownerScope = '') {
    const params = new URLSearchParams();
    group.states.forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    params.set('limit', String(group.limit));
    params.set('order', 'stateEntered');
    if (ownerScope === 'me') params.set('owner', 'me');
    if (group.series) params.set('include', 'progressSeries');
    return `/v1/jobs?${params}`;
}

// The drawer's finished Jobs, in its owner scope: Dismiss finished dismisses
// what the drawer lists, not every account's.
function buildFinishedPageURL(cursor, ownerScope = '') {
    const params = new URLSearchParams();
    FINISHED_STATES.forEach(state => params.append('state', state));
    params.set('dismissed', 'false');
    params.set('limit', String(FINISHED_PAGE_LIMIT));
    if (ownerScope === 'me') params.set('owner', 'me');
    if (cursor) params.set('cursor', cursor);
    return `/v1/jobs?${params}`;
}

// When a job entered the state it is shown in, in milliseconds: the order the
// lists are read in. A row from an older server does not carry it; acceptance
// stands in. Compared as instants, since the text of two instants need not sort
// as they do (a shorter fraction, another offset).
function stateSince(job) {
    const at = Date.parse(job?.stateEnteredAt || job?.acceptedAt || '');
    return Number.isFinite(at) ? at : 0;
}

// Identity breaks a tie as the server's listing breaks it: by the bytes.
function compareIDsDescending(a, b) {
    const left = String(a.id);
    const right = String(b.id);
    return left < right ? 1 : left > right ? -1 : 0;
}

// Newest state change first, each group held to its own limit: a burst of new
// running work must not push the failures a person has to act on out of the
// drawer, and a job that has just finished or failed leads its group.
function boundedPanelJobs(jobs, finishedLimit = DEFAULT_FINISHED_LIMIT) {
    const unique = new Map();
    for (const job of jobs || []) {
        if (!unique.has(job.id)) unique.set(job.id, job);
    }
    const limits = { attention: OPEN_WORK_LIMIT, active: OPEN_WORK_LIMIT, finished: finishedLimit };
    const kept = { attention: 0, active: 0, finished: 0, other: 0 };
    return [...unique.values()]
        .sort((a, b) => stateSince(b) - stateSince(a) || compareIDsDescending(a, b))
        .filter(job => {
            const group = classifyJobState(job);
            if (!(group in limits)) return false;
            kept[group] += 1;
            return kept[group] <= limits[group];
        });
}
