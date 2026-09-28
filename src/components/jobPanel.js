import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { announcePreferenceCommand, openJobPreferenceChannel, preferenceCommand } from '../utils/jobPreferenceChannel.js';
import * as userSettings from '../userSettings.js';
import { captureTrigger, focusedElement, focusFirstIn, focusOn, restoreFocus } from '../utils/focus.js';
import { blockingModal, isRendered, refuseOverModal } from '../utils/modality.js';
import {
    EVENT_SOURCE_CLOSED,
    advertisedCommands,
    canonicalStreamURL,
    classifyJobState,
    commandConfirmation,
    commandConfirmOptions,
    nextStreamRetryDelay,
    commandEndpoint,
    commandFocusSuccessorKeys,
    commandLocation,
    commandNoticeText,
    commandRefusalText,
    latestSnapshot,
    requestPassed,
    requestSettled,
    requestWatch,
    eventJob,
    lifecycleAnnouncement,
    commandLabel,
    commandDismissLabel,
    failureText,
    jobCommands,
    jobName,
    advertisedOutputs,
    reduceJobStreamEvent,
    resultAccessibleLabel,
    resultLinkLabel,
    resultOutput,
    resultURL,
    stateLabel,
    stateOf,
    streamCursorSequence,
    progressAccessibleText,
    progressIndeterminate,
    progressText,
    progressValue,
    jobAmountText,
    jobEtaText,
    jobRateText,
    jobStatsText,
    phaseText,
    scheduledText,
} from './jobCenter.js';
import { isWorking, presentState, stateSinceText, statesInGroup } from './jobStates.js';
import { blockedReasonText, kindLabel } from './jobVocabulary.js';
import {
    applyProgressFrame,
    mergeFetchedProgress,
    formatMetric,
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
// The groups are the shared state table's (server/jobview/job_states.json).
const FINISHED_STATES = statesInGroup('finished');
// Each group is its own bounded page, read by when each job entered its state:
// a job that has just finished or failed leads its group however long ago it
// was accepted. Only open work asks for the progress series: it is up to 120
// points per Job, and a finished row shows no graph. A failure somebody has
// retried (or continued) no longer needs attention: its retry is the Job to
// watch, and it is listed in its own right.
export function panelGroups(finishedLimit) {
    return [
        { key: 'attention', states: statesInGroup('attention'), limit: OPEN_WORK_LIMIT, series: false, notRetried: true },
        { key: 'active', states: statesInGroup('active'), limit: OPEN_WORK_LIMIT, series: true },
        { key: 'finished', states: FINISHED_STATES, limit: finishedLimit, series: false },
    ];
}

// The order of Active and scheduled: the work that is going on, newest state
// change first, then the scheduled work by when it starts, the soonest first.
// When a scheduled Job was scheduled says nothing about which starts next.
export function panelActiveOrder(jobs) {
    const scheduled = [];
    const rest = [];
    for (const job of jobs || []) (stateOf(job) === 'scheduled' ? scheduled : rest).push(job);
    const startsAt = job => {
        const at = Date.parse(job?.scheduledFor || '');
        return Number.isFinite(at) ? at : Number.POSITIVE_INFINITY;
    };
    scheduled.sort((a, b) => startsAt(a) - startsAt(b) || (String(a.id) < String(b.id) ? -1 : String(a.id) > String(b.id) ? 1 : 0));
    return [...rest, ...scheduled];
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

// An RFC 3339 time as epoch microseconds, or NaN. Date.parse keeps only
// milliseconds, and two server times within one millisecond must still order.
// Microseconds are as fine as a Number stays exact at.
export function epochMicros(raw) {
    const match = /^(.+T\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/.exec(String(raw || ''));
    if (!match) return NaN;
    const fraction = (match[2] || '').padEnd(6, '0');
    const millis = Date.parse(`${match[1]}.${fraction.slice(0, 3)}${match[3]}`);
    return Number.isFinite(millis) ? millis * 1000 + Number(fraction.slice(3, 6)) : NaN;
}

// When the server began rendering this page, as epoch microseconds, or null
// when the page does not say.
export function panelRenderedAt(doc = globalThis.document) {
    const parsed = epochMicros(doc?.querySelector?.('meta[name="x-jobs-panel-rendered-at"]')?.getAttribute('content'));
    return Number.isFinite(parsed) ? parsed : null;
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

// The drawer asks by the same rule as every other Job surface
// (commandConfirmation): only what stops work or cannot be taken back.
export function panelCommandConfirmation(command) {
    return commandConfirmation(command);
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

// One word per state for the row's icon and pill colour, from the shared state
// table. The pill's text is the state label, so the colour never carries the
// meaning alone.
export function panelStateTone(job) {
    return presentState(job).tone;
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
// What the Jobs shortcut says inside another dialog it will not open over.
const JOBS_REFUSED_OVER_DIALOG = 'Close this dialog first to open Jobs.';
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
export function panelFocusSuccessorKeys(key) {
    return commandFocusSuccessorKeys(key);
}

// Commands whose result the row shows at once: it gains or loses its pin, or
// comes back to the list. They succeed without a box; a screen reader hears
// the row and what happened to it. A Dismiss takes its row away, so it leaves
// a box that offers to undo it. Every other command leaves a box naming its
// Job (commandNoticeText), unless the answer already moved the row to another
// state: Cancel and Pause are requests the row may not reflect until the
// executor acts, and lineage pins, forgetting replay input and a plugin's own
// commands have results the row cannot show, partial ones included.
const ROW_SHOWN_COMMANDS = { pin: 'pinned', unpin: 'unpinned', undismiss: 'returned to the list' };

function commandDoneText(job, command) {
    return `${jobName(job, 'Job')} ${ROW_SHOWN_COMMANDS[command?.key]}.`;
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
        // What a command's box offers besides its words: a page its answer
        // named (noticeLink), which the drawer never opens on its own since the
        // page under it may hold unsaved work, or the Undismiss that takes a
        // Dismiss back (noticeUndo).
        noticeLink: null,
        noticeUndo: null,
        // A requested control's box leaves once its row has moved past the
        // version the command answered with: the row says the rest.
        _noticeWatch: null,
        // Jobs with a command in flight, whose controls take no second press.
        commandBusy: {},
        // The Jobs the drawer was opened to show (see openFromEvent).
        _revealJobIds: [],
        // Commands whose focus keepFocusOnRow still has to place, counted per
        // Job id.
        _commandFocusPending: {},
        // Where the reader's focus is in the drawer, for when a re-render takes
        // the element away (see startFocusKeeper).
        _focusMemo: null,
        _focusRestoreTimer: null,
        _focusObserver: null,
        _focusInHandler: null,
        _focusOutHandler: null,
        _resourceRefreshNotified: new Set(),
        // When the server began rendering this page; see trackResourceCompletion.
        _renderedAt: null,
        busy: false,
        // Dismiss finished is reading its first page or asking about it.
        _dismissAsking: false,
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
        // The epoch each held detail was read at: a detail read before the
        // latest preference change is stale however its version compares.
        _detailEpochs: new Map(),
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
        // What the last message carried, until a region has spoken it (see
        // messageInFlight): the count of outcomes, news, and at most one notice
        // (the newest notice wins).
        _countNews: null,
        _recentNews: [],
        _recentNotice: '',
        // Which rows a stream snapshot changed, and when, by a counter a refresh
        // reads at its start: a row changed after that may be missing from the
        // group lists, which are read one after another.
        _streamTouchSeq: 0,
        _streamTouched: new Map(),
        _ownerViewer: 0,

        init() {
            this.finishedLimit = panelFinishedLimit();
            this._renderedAt = panelRenderedAt();
            // Set on an administrator's drawer only: whose Job a row is matters
            // when the drawer lists every account's.
            this._ownerViewer = Number(this.$el?.dataset?.jobPanelViewer) || 0;
            this.ownerScope = this.$el?.dataset?.jobPanelOwnerScope === 'me' ? 'me' : '';
            this.adoptPendingOwnerChoice();
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
                        // By id: $refs is read before the drawer first renders
                        // (blockingModal), and Alpine keeps that empty read.
                        const panel = document.querySelector('#job-center-panel');
                        focusFirstIn(panel);
                        this.startFocusKeeper(panel);
                        this.adoptPendingAnnouncement();
                        const reveal = this._revealJobIds;
                        this._revealJobIds = [];
                        if (reveal.length) void this.revealJobs(reveal);
                    });
                } else {
                    this.onDrawerClosed();
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
            this.stopFocusKeeper();
            this._refreshGeneration += 1;
            this._refreshFloor = this._refreshGeneration;
            this._streamGeneration += 1;
            this._panelRefreshRequested = false;
            this.eventSource?.close();
            this.eventSource = null;
            this._liveRegion?.destroy();
            clearTimeout(this._drawerAnnounceTimer);
            clearTimeout(this._unsaidOutcomesTimer);
        },

        onDrawerClosed() {
            this.stopClock();
            this.stopFocusKeeper();
            // A box answers the command it followed while the reader watched;
            // it is not news to find on the next opening.
            this.clearNotice();
            // After the trap has let go and the drawer is gone: focus moved
            // while the trap is armed is pulled back inside, and then falls to
            // <body> when the drawer is removed.
            const trigger = this._lastTrigger;
            this._lastTrigger = null;
            setTimeout(() => {
                if (!this.isOpen) restoreFocus(trigger, this._trigger);
            }, 0);
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
        get activeJobs() { return panelActiveOrder(this.jobs.filter(job => classifyJobState(job) === 'active')); },
        get finishedJobs() { return this.jobs.filter(job => classifyJobState(job) === 'finished'); },
        // The drawer's sections, in the order a person acts on them. An empty
        // section is left out rather than drawn with nothing under it.
        // A group that holds fewer rows than its list had carries `more`, says
        // so (`moreText`), and links to where All jobs shows the rest.
        get groups() {
            // The Job Center's Finished includes failed and interrupted jobs, which
            // the drawer lists under Needs attention instead; its last group says
            // it holds only the rest.
            const lists = new Map(panelGroups(this.finishedLimit).map(group => [group.key, group]));
            return [
                { key: 'attention', title: 'Needs attention', jobs: this.attentionJobs, moreLabel: 'See every job that needs attention' },
                { key: 'active', title: 'Active and scheduled', jobs: this.activeJobs, moreLabel: 'See every active and scheduled job' },
                { key: 'finished', title: 'Finished, no attention needed', jobs: this.finishedJobs, moreLabel: 'See every finished job' },
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
        // The box's words, until what they report has reached the row.
        get noticeText() {
            const watch = this._noticeWatch;
            if (watch) {
                const row = this.jobs.find(job => job.id === watch.jobId);
                if (requestPassed(watch, row)) return '';
            }
            return this.notice;
        },
        get finishedCount() {
            return this.jobs.filter(job => classifyJobState(job) === 'finished' &&
                this.commandsFor(job).some(command => command.key === 'dismiss')).length;
        },

        handleShortcut(event) {
            if (!(event.metaKey || event.ctrlKey) || !event.shiftKey || String(event.key).toLowerCase() !== 'd') return;
            event.preventDefault();
            // No event: a shortcut has no trigger, so focus goes back to
            // wherever the reader pressed it (see focusedElement).
            this.toggle();
        },

        blockingModal() {
            if (typeof document === 'undefined') return null;
            return blockingModal([this._root, this.$refs?.panel]);
        },

        // `jobIds` names the Jobs whatever asked for the drawer just started (a
        // plugin action run, in the order it started them): the drawer shows one
        // of them, which may sit far below the failures listed first.
        openFromEvent(detail = null) {
            const reveal = Array.isArray(detail?.jobIds) ? detail.jobIds.filter(id => typeof id === 'string' && id) : [];
            if (!this.isOpen) {
                const blocker = this.blockingModal();
                if (blocker) {
                    refuseOverModal(blocker, JOBS_REFUSED_OVER_DIALOG);
                    return;
                }
                const requested = detail?.returnFocusTo;
                this._lastTrigger = (isRendered(requested) ? requested : null) ?? focusedElement() ?? this._trigger;
                this._revealJobIds = reveal;
                this.isOpen = true;
                return;
            }
            if (reveal.length) void this.revealJobs(reveal);
        },

        // Moves focus to the topmost row of the Jobs a run started, scrolled into
        // view. When the drawer lists none of them yet (they were accepted a
        // moment ago), it reads the newest one, the last started, which a
        // group's cap keeps longest, and adds its row. One a full group still
        // leaves out is offered as a link in the box instead. Focus the reader
        // has already moved somewhere of their own is left there.
        async revealJobs(ids) {
            const wanted = new Set(ids);
            const panel = () => document.querySelector('#job-center-panel');
            const firstRow = () => [...(panel()?.querySelectorAll('article[data-job-id]') || [])].find(row => wanted.has(row.dataset.jobId));
            const initialFocus = document.activeElement;
            let read = null;
            if (!firstRow() && !this.jobs.some(job => wanted.has(job.id))) {
                const id = ids[ids.length - 1];
                // Heard as any read is: an outcome it finds is said only when a
                // live event proves it happened on this stream (hearFromRead).
                const streamGeneration = this._streamGeneration;
                try {
                    const job = await this.requestJSON(`/v1/jobs/${encodeURIComponent(id)}`);
                    if (job?.id === id && !this.jobs.some(row => row.id === id)) {
                        const spoken = [];
                        read = job;
                        this.details[id] = job;
                        this.hearFromRead(job, streamGeneration, spoken);
                        this.upsert(job);
                        if (streamGeneration === this._streamGeneration) this.announceNews(spoken);
                    }
                } catch {
                    return;
                }
                await new Promise(resolve => (this.$nextTick ? this.$nextTick(resolve) : resolve()));
            }
            if (!this.isOpen) return;
            const title = firstRow()?.querySelector('a[id^="job-panel-title-"]');
            if (!title) {
                if (read && !this.jobs.some(job => job.id === read.id)) {
                    this.setNotice(`${read.title || read.kind || 'The job'} started.`, { link: { href: this.detailURL(read), label: 'Open the job' } });
                    this.announceNotice(this.notice);
                }
                return;
            }
            const active = document.activeElement;
            const untouched = !active || active === document.body || active === initialFocus || active.matches?.('button[aria-label="Close Jobs panel"]');
            if (!untouched || !focusOn(title)) return;
            title.scrollIntoView?.({ block: 'nearest' });
            this.noteFocus(title);
        },

        toggle(event = null) {
            const blocker = this.isOpen ? null : this.blockingModal();
            if (blocker) {
                refuseOverModal(blocker, JOBS_REFUSED_OVER_DIALOG);
                return;
            }
            if (!this.isOpen) this._lastTrigger = captureTrigger(event) ?? focusedElement() ?? this._trigger;
            this.isOpen = !this.isOpen;
        },

        // The isOpen watcher returns focus, once the drawer is gone.
        close() {
            this.isOpen = false;
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
        },

        // Whether the last message is still on its way: waiting in the drawer's
        // delay, or in the page region's, which a message the closing drawer
        // handed over waits in too. What it carries has not been spoken yet.
        messageInFlight() {
            return !!this._drawerAnnounceTimer || !!this._liveRegion?.pending?.();
        },

        // Once the last message has been spoken, nothing it carried is owed.
        forgetLanded() {
            if (this.messageInFlight()) return;
            this._countNews = null;
            this._recentNews = [];
            this._recentNotice = '';
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
            const changed = preferenceCommand(url, init, payload)?.jobIds;
            if (changed?.length) {
                // This tab changed them: what it holds about them is older now,
                // and so is every list read begun before, whose rows carry the
                // old pin at the same version. Those reads are fenced, the
                // lists read again, and the open drawer's details too (Forget
                // takes Retry away, with no version to say so).
                this.movePreferenceEpochs(changed);
                this.fenceEarlierReads();
                this.startScheduledPanelRefresh();
                this.loadStaleDetails();
            }
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
            // A pin this read shows differently from a row no newer than it was
            // changed somewhere the drawer did not hear (another browser). A
            // row newer than the read keeps its own pin.
            this.movePreferenceEpochs([...byId.values()]
                .filter(job => {
                    const held = shown.get(job.id);
                    return held && Object.hasOwn(job, 'pinned') && !!held.pinned !== !!job.pinned &&
                        Number(held.version || 0) <= Number(job.version || 0);
                })
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
                (Object.hasOwn(job, 'pinned') && !!detail.pinned !== !!job.pinned) ||
                (this._detailEpochs.get(String(job.id)) || 0) !== this.preferenceEpoch(job.id);
        },

        forgetDetailsOfGoneRows() {
            const shown = new Set(this.jobs.map(job => job.id));
            for (const id of Object.keys(this.details)) {
                if (!shown.has(id)) {
                    delete this.details[id];
                    this._detailEpochs.delete(String(id));
                }
            }
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
                    if (this.detailStale(job)) {
                        this.details[job.id] = job;
                        this._detailEpochs.set(String(job.id), this.preferenceEpoch(job.id));
                    }
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
            this._detailEpochs.set(String(job.id), epoch);
            const spoken = [];
            this.hearFromRead({ ...current, ...detail }, streamGeneration, spoken);
            // The row takes the detail's state, not its commands, outputs or
            // lineage: those live in `details`, fenced by version and epoch. A
            // row carrying commands counts as its own detail (loadStaleDetails),
            // which a copied list would make true long after it went stale.
            const { commands: _commands, outputs: _outputs, lineage: _lineage, ...state } = detail;
            this.jobs = this.bounded(this.jobs.map(row => row.id === job.id ? { ...row, ...state } : row));
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
            this.forgetLanded();
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

        // News the last message carried and no region has spoken yet, still true.
        pendingNews() {
            return this.messageInFlight() ? this.currentNews(this._recentNews) : [];
        },

        pendingNotice() {
            return this.messageInFlight() ? this._recentNotice : '';
        },

        // Every panel message goes through here: news, a notice, or both. What
        // the last message carried and no region has spoken yet is said again
        // with it, since the region would otherwise replace it, however long
        // the message has waited (a drawer closing hands it to the page region,
        // which waits again). A new notice replaces a pending one; news
        // accumulates, less anything superseded. A count of outcomes is carried
        // across a drop too: nothing else would say it again.
        say(entries = [], notice = '') {
            // A stopped drawer says nothing about Jobs: they came from the
            // database a reset replaced. A notice, such as the stop itself, is
            // still said.
            if (this.streamStopped) entries = [];
            this.forgetLanded();
            const carried = this._countNews ? [this._countNews] : [];
            const news = this.currentNews([...carried, ...this.pendingNews(), ...entries]);
            const text = notice || this.pendingNotice();
            this._countNews = news.find(entry => entry.jobId === null) || null;
            this._recentNews = news;
            this._recentNotice = text;
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
            this.clearNotice();
            this.error = '';
            this.signedOut = false;
            // What the reader was, or was about to be, told about Jobs belongs to
            // the other database too: the ledger, the proofs, every message not
            // yet landed and every timer that would say one. Only the stop is
            // said, and say() says nothing about a Job from here on.
            clearTimeout(this._drawerAnnounceTimer);
            clearTimeout(this._unsaidOutcomesTimer);
            this._drawerAnnounceTimer = null;
            this._unsaidOutcomesTimer = null;
            this._unsaidOutcomes = 0;
            this._countNews = null;
            this._recentNews = [];
            this._recentNotice = '';
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

        // The resource lists refresh for a download that succeeded after the server
        // began rendering this page: the page cannot hold its resource. One that
        // succeeded before the render is already in the page, and refreshing for
        // it would morph the lists under whatever the reader has open in them
        // every time the drawer first read it (and it stays in the drawer's
        // Finished group, so that is every page load). The two times are both the
        // server's, so the rule holds whenever and however the drawer first sees
        // the download: a read before or after the stream's catch-up, after a
        // reconnect, or in a command's answer. A page that does not say when it
        // was rendered refreshes for nothing, since it cannot tell. The render
        // time is read once, at init: a list morphed in later carries no head,
        // and the page's head does not change.
        // Known limit: the two times can come from different processes. With a
        // skew of d between their clocks, a download finishing within d of the
        // render is misread: refreshed for although the page lists it, or not
        // refreshed for although it does not (its resource then appears on the
        // next load). Widening the comparison moves the error to the other side.
        trackResourceCompletion(job) {
            if (!job?.id || job.state !== 'succeeded' ||
                (job.kind !== 'remote-download' && job.kind !== 'deferred-download') ||
                this._resourceRefreshNotified.has(job.id)) return;
            this._resourceRefreshNotified.add(job.id);
            if (this._resourceRefreshNotified.size > 256) {
                this._resourceRefreshNotified.delete(this._resourceRefreshNotified.values().next().value);
            }
            const finishedAt = epochMicros(job.finishedAt);
            if (this._renderedAt === null || !Number.isFinite(finishedAt) || finishedAt < this._renderedAt) return;
            if (globalThis.window?.dispatchEvent && globalThis.CustomEvent) {
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
        kindText(job) { return kindLabel(job?.kind); },
        // How long ago the row entered its state, counted on the drawer's own
        // clock. It is not in a live region: the ledger says what changed, and
        // this text is read when a reader reaches it, not every second.
        sinceText(job) { return stateSinceText(job, this.now); },
        sinceTitle(job) {
            const at = new Date(job?.stateEnteredAt || '');
            return Number.isNaN(at.getTime()) ? '' : at.toLocaleString();
        },
        // Why a blocked Job is blocked, in one line: the detail page's timeline
        // holds the rest.
        blockedText(job) { return stateOf(job) === 'blocked' ? blockedReasonText(job?.blockedReason) : ''; },

        setNotice(text, { link = null, undo = null, watch = null } = {}) {
            this.notice = text || '';
            this.noticeLink = text ? link : null;
            this.noticeUndo = text ? undo : null;
            this._noticeWatch = text ? watch : null;
        },

        clearNotice() {
            this.setNotice('');
        },

        commandBusyFor(job) {
            return !!this.commandBusy[job?.id];
        },

        // A read of one Job's detail after a command: the answer's snapshot
        // carries no commands, and a command such as Forget moves no version, so
        // nothing else would replace the controls the command made stale. It is
        // the preference-fenced read below.
        rereadJob(id, spoken = null) {
            return this.refreshJobPreference(id, spoken);
        },

        async refreshJobPreference(id, spoken = null) {
            const epoch = this.preferenceEpoch(id);
            const payload = await this.requestJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            const freshJob = payload.job || payload;
            // A preference changed elsewhere while this was read: the answer is
            // older than the row, and a fresh read replaces it.
            if (this.preferenceEpoch(id) !== epoch) {
                delete this.details[id];
                this.startScheduledPanelRefresh();
                return freshJob?.id ? freshJob : null;
            }
            // A read the row has since moved past carries the controls of an
            // older version: it is not kept, or the row would offer them again.
            const shown = this.jobs.find(row => row.id === id) || this.details[id];
            if (freshJob?.id && Number(freshJob.version || 0) < Number(shown?.version || 0)) return shown;
            if (freshJob?.id) {
                this.details[id] = freshJob;
                this._detailEpochs.set(String(id), epoch);
                // A read: any change of state in it is someone else's.
                this.applyStreamSnapshot(freshJob, false, false, spoken, { asRead: true });
            }
            return freshJob?.id ? freshJob : null;
        },

        // One command per Job at a time: a second press while the first is in
        // flight would be decided from the version the first is changing, and
        // turn its success into a conflict.
        async runCommand(job, command) {
            if (!job?.id || this.commandBusy[job.id]) return null;
            this.commandBusy = { ...this.commandBusy, [job.id]: true };
            const watch = this.watchReaderFocus();
            if (watch) this._commandFocusPending = { ...this._commandFocusPending, [job.id]: (this._commandFocusPending[job.id] || 0) + 1 };
            this._commandRefresh = null;
            try {
                return await this.runCommandUnfocused(job, command);
            } finally {
                const { [job.id]: _done, ...stillBusy } = this.commandBusy;
                this.commandBusy = stillBusy;
                // A Dismiss's refresh may reveal the job that takes the freed
                // place; focus is placed once it has.
                this.keepFocusOnRow(job.id, command, watch, this._commandRefresh);
                this._commandRefresh = null;
            }
        },

        // An administrator's Mine or Everyone choice, kept for their next page.
        chooseOwnerScope(choice) {
            const value = choice === 'mine' ? 'mine' : 'everyone';
            this.setOwnerScope(value === 'mine' ? 'me' : '');
            this.rememberPendingOwnerChoice(value);
            this.saveOwnerChoice(value);
        },

        // Sent at once, since the next page is rendered with the stored choice;
        // the settings store sends one key's writes one after another, so the
        // last choice made is the last one stored.
        saveOwnerChoice(value) {
            return userSettings.saveNow('jobsPanelScope', value);
        },

        // A page opened before the choice was stored is rendered with the old
        // one. The choice is carried in the tab's session: the next page
        // applies it if it was rendered without it, and stores it again
        // either way, since a write the page before left in flight can still
        // overtake the one its render reflects. It is dropped once that write
        // is answered. Keyed by the viewer, so another account in the tab
        // ignores it.
        pendingOwnerChoiceKey() {
            return `mahresources.jobsPanelScope.pending.${this._ownerViewer}`;
        },

        rememberPendingOwnerChoice(value) {
            try { globalThis.sessionStorage?.setItem(this.pendingOwnerChoiceKey(), value); }
            catch { /* the stored choice still applies on the next page once saved */ }
        },

        adoptPendingOwnerChoice() {
            if (!this._ownerViewer) return;
            let pending = null;
            try { pending = globalThis.sessionStorage?.getItem(this.pendingOwnerChoiceKey()) || null; }
            catch { return; }
            if (pending !== 'mine' && pending !== 'everyone') return;
            // Before the first read and the stream: nothing to reconnect yet.
            this.ownerScope = pending === 'mine' ? 'me' : '';
            const key = this.pendingOwnerChoiceKey();
            this.saveOwnerChoice(pending).then(stored => {
                if (!stored) return;
                try {
                    if (globalThis.sessionStorage?.getItem(key) === pending) globalThis.sessionStorage.removeItem(key);
                } catch { /* the next page stores it once more */ }
            }, () => {});
        },

        // The Undo a Dismiss's box offers.
        async undoDismiss() {
            const undo = this.noticeUndo;
            if (!undo) return null;
            return this.runCommand(undo.job, undo.command);
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
                const left = (this._commandFocusPending[jobId] || 1) - 1;
                const { [jobId]: _settled, ...others } = this._commandFocusPending;
                this._commandFocusPending = left > 0 ? { ...others, [jobId]: left } : others;
                // Focus lost elsewhere while this command's was pending was left
                // to it; whatever it did not place is looked at again.
                setTimeout(() => this.checkFocusLost(), 0);
                if (watch.opener.isConnected || watch.movedByReader || !this.isOpen) return;
                const panel = document.querySelector('#job-center-panel');
                const active = document.activeElement;
                if (active && active !== document.body && !panel?.contains(active)) return;
                this.focusRowOrNeighbour(jobId, watch.rows, [
                    ...panelFocusSuccessorKeys(command?.key).map(key => `button[data-command-key="${CSS.escape(key)}"]`),
                    'details[open] summary',
                    '[role="group"] button, [role="group"] summary',
                    'a[href]',
                ]);
            }, 0)));
        },

        // Puts focus on the first of `selectors` the Job's row has, scrolled
        // into view (the row may have moved to another group). A row that is
        // gone hands focus to the row that took its place, else the one before
        // it (`rows` is the order the reader last saw), else All jobs.
        focusRowOrNeighbour(jobId, rows, selectors) {
            const panel = document.querySelector('#job-center-panel');
            const rowFor = id => panel?.querySelector(`article[data-job-id="${CSS.escape(String(id))}"]`);
            const reveal = element => {
                if (!element || !isRendered(element) || !focusOn(element)) return false;
                element.scrollIntoView?.({ block: 'nearest' });
                this.noteFocus(element);
                return true;
            };
            const row = rowFor(jobId);
            if (row) {
                for (const selector of selectors) if (reveal(row.querySelector(selector))) return true;
                return false;
            }
            const at = (rows || []).indexOf(String(jobId));
            const neighbours = [...(rows || []).slice(at + 1), ...(rows || []).slice(0, Math.max(at, 0)).reverse()];
            const links = [
                ...neighbours.map(id => rowFor(id)?.querySelector('a[href]')),
                // One a refresh revealed, when no neighbour is left.
                panel?.querySelector('article[data-job-id] a[href]'),
                panel?.querySelector('[data-job-panel-all-jobs]'),
            ];
            for (const link of links) if (reveal(link)) return true;
            return false;
        },

        // Rows re-render all the time: a live change moves a row from one group
        // to another, which the drawer draws as a new element, and a refresh
        // replaces the controls a row offers. When that takes away the element
        // the reader was on, the trap drops focus on the Close button (or it
        // falls to <body>), and a keyboard reader loses their place in a list
        // of up to a hundred rows. The keeper remembers where focus is, notices
        // the element leaving, and puts focus back on the same row: the same
        // control, its counterpart, or the row's title, wherever the row now is.
        startFocusKeeper(panel) {
            this.stopFocusKeeper();
            if (!panel || typeof MutationObserver === 'undefined') return;
            this._focusInHandler = event => this.rememberFocus(event);
            panel.addEventListener('focusin', this._focusInHandler);
            // The element losing focus names where it went, which is heard even
            // when the browser dispatches no focusin for the element gaining it.
            this._focusOutHandler = event => {
                if (event.relatedTarget) this.noteFocus(event.relatedTarget);
            };
            panel.addEventListener('focusout', this._focusOutHandler);
            this._focusObserver = new MutationObserver(() => this.checkFocusLost());
            this._focusObserver.observe(panel, { childList: true, subtree: true });
            this._focusObserver.panel = panel;
            this.noteFocus(document.activeElement);
        },

        stopFocusKeeper() {
            if (this._focusObserver) {
                this._focusObserver.panel?.removeEventListener('focusin', this._focusInHandler);
                this._focusObserver.panel?.removeEventListener('focusout', this._focusOutHandler);
                this._focusObserver.disconnect();
            }
            clearTimeout(this._focusRestoreTimer);
            this._focusObserver = null;
            this._focusInHandler = null;
            this._focusOutHandler = null;
            this._focusRestoreTimer = null;
            this._focusMemo = null;
        },

        // A move the reader makes comes from the element losing focus, so it has
        // a relatedTarget; the trap's rescue after the focused element was
        // removed comes from nowhere, and is not remembered as theirs.
        rememberFocus(event) {
            const previous = this._focusMemo?.element;
            if (previous && !previous.isConnected && !event.relatedTarget) return;
            this.noteFocus(event.target);
        },

        noteFocus(target) {
            const panel = this._focusObserver?.panel;
            if (!target || !panel?.contains?.(target)) return;
            const row = target.closest?.('article[data-job-id]');
            this._focusMemo = {
                element: target,
                jobId: row?.dataset.jobId || null,
                // A control in the box (Undo, a link) goes when the box clears.
                inNotice: !!target.closest?.('[data-job-panel-notice]'),
                commandKey: target.dataset?.commandKey || '',
                selector: rowControlSelector(target),
                rows: row ? [...panel.querySelectorAll('article[data-job-id]')].map(article => article.dataset.jobId) : null,
            };
        },

        checkFocusLost() {
            const memo = this._focusMemo;
            if (!(memo?.jobId || memo?.inNotice) || memo.element.isConnected || this._focusRestoreTimer) return;
            // A command places focus itself on its own row once it settles
            // (keepFocusOnRow), and a control in the box goes because of one.
            const pending = this._commandFocusPending;
            if (memo.jobId ? pending[memo.jobId] > 0 : Object.keys(pending).length > 0) return;
            // After the trap's own rescue, which runs on the same mutations.
            this._focusRestoreTimer = setTimeout(() => {
                this._focusRestoreTimer = null;
                this.restoreLostFocus(memo);
            }, 0);
        },

        // Puts focus back where `memo` says the reader was. A render still in
        // progress may leave nothing to focus yet, so an attempt that places it
        // nowhere is made again a frame later, a few times, while the memo is
        // still the reader's and its element is still gone.
        restoreLostFocus(memo, attempt = 0) {
            if (this._focusMemo !== memo || memo.element.isConnected || !this.isOpen) return;
            const last = attempt >= FOCUS_RESTORE_ATTEMPTS - 1;
            const again = () => {
                if (!last) afterNextPaint(() => this.restoreLostFocus(memo, attempt + 1));
            };
            if (!memo.jobId) {
                // The box cleared: the stopped drawer's Reload page, else the
                // first row, else All jobs. A frame later, because x-show
                // reveals an element (the stopped notice) on the next animation
                // frame, after this timer.
                afterNextPaint(() => {
                    if (this._focusMemo !== memo || !this.isOpen) return;
                    const panel = document.querySelector('#job-center-panel');
                    for (const candidate of [
                        panel?.querySelector('[data-job-panel-stopped] button'),
                        panel?.querySelector('article[data-job-id] a[href]'),
                        panel?.querySelector('[data-job-panel-all-jobs]'),
                    ]) {
                        if (candidate && isRendered(candidate) && focusOn(candidate)) {
                            this.noteFocus(candidate);
                            return;
                        }
                    }
                    again();
                });
                return;
            }
            // A row the drawer still lists but has not drawn yet is being
            // redrawn in another group: it is waited for rather than focus
            // going to a neighbour.
            const drawn = document.querySelector(`#job-center-panel article[data-job-id="${CSS.escape(String(memo.jobId))}"]`);
            if (!drawn && !last && this.jobs.some(job => job.id === memo.jobId)) {
                again();
                return;
            }
            const placed = this.focusRowOrNeighbour(memo.jobId, memo.rows, [
                memo.selector,
                ...(memo.commandKey ? panelFocusSuccessorKeys(memo.commandKey).map(other => `button[data-command-key="${CSS.escape(other)}"]`) : []),
                'a[id^="job-panel-title-"]',
            ]);
            if (!placed) again();
        },

        async runCommandUnfocused(job, command) {
            const confirmation = panelCommandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(confirmation, commandConfirmOptions(job, command));
                if (!accepted) return null;
            }
            // A confirmation answered after the drawer stopped would send a
            // command about a Job the other database described.
            if (this.streamStopped) return null;
            const key = commandKey();
            // Changes a live event proved, said with the command's notice.
            const proved = [];
            const name = jobName(job, 'Job');
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
                const keepsRecord = RECORD_COMMANDS.has(command?.key) || command?.key === 'dismiss' || command?.key === 'undismiss';
                if (freshJob?.id) this.applyStreamSnapshot(freshJob, false, false, proved, { asRead: keepsRecord });
                let now = freshJob || null;
                let rereadFailed = false;
                if (command?.key === 'dismiss' || command?.key === 'undismiss') {
                    // Dismissing records a preference and emits no job event, so
                    // no refresh would take the row away (or bring it back): it
                    // leaves now, and a fresh refresh supersedes any that read the
                    // lists before the change and brings in whatever the freed
                    // place, or the returned Job, makes room for.
                    if (command.key === 'dismiss') this.jobs = this.jobs.filter(row => row.id !== job.id);
                    this._commandRefresh = this.refresh();
                } else {
                    try { now = await this.rereadJob(job.id, proved) || now; }
                    catch { rereadFailed = true; }
                    if (this.streamStopped) return null;
                }
                // The drawer sits over every page, and the page may hold input
                // nobody has saved: a page a command's answer names is offered as
                // a link, never opened in its place.
                const successorId = outcome.successorId || outcome.successorID || result.successorId || result.successorID;
                const location = successorId ? `/job?id=${encodeURIComponent(successorId)}` : commandLocation(outcome);
                // Only a lifecycle command's answer can be what moved the row; a
                // record-keeping command's result is said whatever else changed,
                // and a change of state it happened to read is heard as a read.
                // A request the executor has already carried out is said as its
                // result too (requestSettled).
                const latest = latestSnapshot(freshJob, now);
                const movedOn = !keepsRecord && latest?.state && stateOf(latest) !== stateOf(job) &&
                    (outcome.code !== 'requested' || requestSettled(job, freshJob, now, outcome));
                let spoken = '';
                if (rereadFailed && (command?.key === 'pin' || command?.key === 'unpin')) {
                    this.setNotice(`${commandLabel(command)} completed. Reload this job to see its current pin status.`);
                } else if (command?.key === 'dismiss') {
                    const undismiss = advertisedCommands(this.details[job.id] || job).find(entry => entry?.key === 'undismiss');
                    this.setNotice(`${name} dismissed.`, { undo: undismiss ? { job, command: undismiss } : null });
                } else if (Object.hasOwn(ROW_SHOWN_COMMANDS, command?.key)) {
                    this.clearNotice();
                    spoken = commandDoneText(job, command);
                } else if (location) {
                    this.setNotice(successorId
                        ? `${commandLabel(command)} started a new job for ${name}.`
                        : `${commandLabel(command)} for ${name} opens another page.`,
                    { link: { href: location, label: successorId ? 'Open the new job' : 'Open it' } });
                } else if (movedOn) {
                    // The answer moved the row to another state, which it shows.
                    this.clearNotice();
                    spoken = lifecycleAnnouncement({ ...job, ...latest });
                } else {
                    this.setNotice(commandNoticeText(job, command, outcome), {
                        watch: outcome.code === 'requested' ? { jobId: job.id, ...requestWatch(job) } : null,
                    });
                }
                this.announceNotice(this.notice || spoken, proved);
                return outcome;
            } catch (error) {
                if (this.streamStopped) return null;
                // A refusal that carries the Job reports someone else's change,
                // heard as a read would hear it.
                const freshJob = error.payload?.job || error.payload?.result?.job;
                if (freshJob?.id) this.applyStreamSnapshot(freshJob, false, false, proved, { asRead: true });
                let now = freshJob?.id ? freshJob : null;
                // Whatever refused it, the controls on screen offered a command
                // the Job did not take: they are read again.
                if (error.status !== 404 && !error.streamStopped) {
                    try { now = await this.rereadJob(job.id, proved) || now; }
                    catch { /* the refusal is still said */ }
                }
                if (this.streamStopped) return null;
                this.setNotice(commandRefusalText(job, command, error, now));
                this.announceNotice(this.notice, proved);
                return null;
            }
        },

        // Dismisses every finished job this viewer has not dismissed, not only the
        // few the panel shows. The list is walked by keyset cursor, so rows leaving
        // the undismissed filter while we page do not shift it, and each page fits
        // one bulk request. The server answers per job, so it alone decides which
        // jobs may be dismissed. Only counts are kept: a backlog can be far larger
        // than anything worth holding in the page, which is also why this one asks
        // first, saying how many it reaches, where a single Dismiss offers Undo.
        async dismissFinished() {
            if (this.busy || this._dismissAsking || this.streamStopped || this.finishedCount === 0) return { dismissed: 0, total: 0 };
            let first;
            // The scope the reader confirmed is the one dismissed, page after
            // page, whatever the drawer is switched to while this runs.
            const ownerScope = this.ownerScope;
            this._dismissAsking = true;
            try {
                first = await this.requestJSON(buildFinishedPageURL('', ownerScope));
                const count = (first.jobs || []).length;
                if (count === 0) {
                    // What the drawer showed has already gone.
                    void this.refresh();
                    return { dismissed: 0, total: 0 };
                }
                // An administrator's drawer says whose jobs: the choice beside it
                // can differ from what the dialog was asked about.
                const whose = this._ownerViewer ? (ownerScope === 'me' ? 'your' : 'everyone\'s') : '';
                if (ownerScope !== this.ownerScope) return this.refuseChangedScope();
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(
                    dismissFinishedConfirmation(count, !!first.nextCursor, this.finishedJobs.length, whose),
                    { title: 'Dismiss finished jobs', confirmLabel: first.nextCursor ? 'Dismiss all' : `Dismiss ${count}`, destructive: false },
                );
                if (!accepted || this.streamStopped) return { dismissed: 0, total: 0 };
                if (ownerScope !== this.ownerScope) return this.refuseChangedScope();
            } catch (error) {
                if (!this.streamStopped) {
                    this.setNotice(error.message || 'Could not dismiss finished jobs.');
                    this.announceNotice(this.notice);
                }
                return { dismissed: 0, total: 0 };
            } finally {
                this._dismissAsking = false;
            }
            this.busy = true;
            let dismissed = 0;
            let total = 0;
            let refusal = '';
            try {
                let cursor = '';
                let page = first;
                do {
                    if (!page) page = await this.requestJSON(buildFinishedPageURL(cursor, ownerScope));
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
                    page = null;
                } while (cursor);
                const plural = total === 1 ? '' : 's';
                // Full success shows no box: the rows leaving say it.
                this.setNotice(dismissed === total
                    ? ''
                    : `${dismissed} of ${total} finished job${plural} dismissed. Not dismissed: ${refusal}`);
                this.announceNotice(this.notice || `${dismissed} finished job${plural} dismissed.`);
            } catch (error) {
                if (this.streamStopped) {
                    this.busy = false;
                    return { dismissed, total };
                }
                const reason = error.message || 'Could not dismiss finished jobs.';
                let notice = dismissed > 0
                    ? `${dismissed} finished job${dismissed === 1 ? '' : 's'} dismissed before an error: ${reason}`
                    : reason;
                if (refusal) notice = `${notice.replace(/\.$/, '')}. Not dismissed: ${refusal}`;
                this.setNotice(notice);
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

        // The drawer was switched to another account scope while Dismiss
        // finished read or asked: what was counted is not what is shown.
        refuseChangedScope() {
            this.setNotice('The jobs shown changed while Dismiss finished was asking. Nothing was dismissed; try again.');
            this.announceNotice(this.notice);
            return { dismissed: 0, total: 0 };
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
        scheduledText(job) { return scheduledText(job, this.now); },
        failureText(job) { return failureText(job); },
        commandLabel(command) { return commandLabel(command); },
        phaseText(job) { return phaseText(job); },
        progressText(job) { return progressText(job); },
        progressValue(job) { return progressValue(job); },
        // Only running work pulses: a paused or queued Job with no total is
        // waiting, not working.
        progressIndeterminate(job) { return progressIndeterminate(job); },
        progressAccessibleText(job) { return progressAccessibleText(job); },
        showsProgress(job) {
            const progress = job?.progress || {};
            return classifyJobState(job) === 'active' && (
                Number.isFinite(progress.completed) || !!progress.message || isWorking(job));
        },
        // The line above the bar: what the executor says it is doing, else its
        // phase. The counts are in statsText, formatted, rather than here raw.
        progressLabel(job) {
            return job?.progress?.message || phaseText(job) || stateLabel(job);
        },
        // Everything the bar shows, for a reader who cannot see it.
        progressValueText(job) {
            const value = progressValue(job);
            const parts = [value === null ? '' : `${value}%`, jobAmountText(job), jobRateText(job, this.now), jobEtaText(job, this.now)].filter(Boolean);
            return parts.length ? parts.join(', ') : progressAccessibleText(job);
        },
        statsText(job) { return jobStatsText(job, this.now); },
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
    if (group.notRetried) params.set('noInboundRelationship', 'retry-of');
    params.set('limit', String(group.limit));
    params.set('order', 'stateEntered');
    if (ownerScope === 'me') params.set('owner', 'me');
    if (group.series) params.set('include', 'progressSeries');
    return `/v1/jobs?${params}`;
}

// What Dismiss finished asks before it runs. `count` is the first page of
// finished jobs the viewer has not dismissed, and `more` says there are pages
// after it; `shown` is how many of them the drawer lists.
// It names the states it reaches, so nobody expects a failure to go with them,
// and on an administrator's drawer whose jobs (`whose`: 'your' or
// "everyone's"; '' where the drawer offers no choice).
export function dismissFinishedConfirmation(count, more, shown, whose = '') {
    const after = 'Failed jobs stay in Needs attention. Dismissed jobs stay on All jobs under the Dismissed filter, where each can be undismissed.';
    if (more) {
        const every = whose ? `every one of ${whose} jobs` : 'every job';
        return `Dismiss ${every} that succeeded or was cancelled and that you have not dismissed? That is more than ${count}, and the drawer shows ${shown}. ${after}`;
    }
    const hidden = Math.max(0, count - shown);
    const which = whose
        ? `of ${whose} finished jobs that succeeded or were cancelled`
        : count === 1 ? 'finished job that succeeded or was cancelled' : 'finished jobs that succeeded or were cancelled';
    return `Dismiss ${count} ${which}?${hidden > 0 ? ` ${hidden} of them ${hidden === 1 ? 'is' : 'are'} not shown here.` : ''} ${after}`;
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

// How many frames a lost focus is looked for before the keeper gives up.
const FOCUS_RESTORE_ATTEMPTS = 4;

function afterNextPaint(callback) {
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(() => setTimeout(callback, 0));
    else setTimeout(callback, 0);
}

// The selector that finds, in a re-rendered row, the control a reader was on.
function rowControlSelector(element) {
    const key = element?.dataset?.commandKey;
    if (key) return `button[data-command-key="${CSS.escape(key)}"]`;
    if (element?.id?.startsWith?.('job-panel-title-')) return 'a[id^="job-panel-title-"]';
    if (element?.tagName === 'SUMMARY') return 'summary';
    if (element?.tagName === 'A') return '[role="group"] a[href]';
    return 'a[id^="job-panel-title-"]';
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
