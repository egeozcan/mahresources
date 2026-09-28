import { findListContainer } from '../utils/listContainer.js';
import { morphAndReinitChangedComponents } from '../utils/shortcodeElementMorph.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { announcePreferenceCommand, openJobPreferenceChannel } from '../utils/jobPreferenceChannel.js';
import {
    EVENT_SOURCE_CLOSED, canonicalStreamURL, commandConfirmation, commandDismissLabel, commandFocusSuccessorKeys, commandLabel,
    lifecycleAnnouncement, nextStreamRetryDelay, progressAccessibleText, progressIndeterminate, progressText, progressValue,
    reloadAfterStreamReset, selectedBulkCommands, streamCursorSequence,
} from './jobCenter.js';
import { drawerAnnouncesJob } from '../utils/jobAnnouncements.js';
import { applyProgressFrame, formatDuration, formatRate, liveEtaText, liveRateText } from './jobProgress.js';
import { terminalStates } from './jobStates.js';
import { focusOn, keepFocusWithin } from '../utils/focus.js';

export const JOB_LIST_REFRESH_DEBOUNCE_MS = 500;
// After a failed refetch: long enough not to hammer a struggling server, short
// enough that a page left open catches up without another event or a reload.
export const JOB_LIST_REFRESH_RETRY_MS = 5000;

// The regions of /jobs a live refresh replaces. The list is the shared list
// container; the quick filters carry counts; the pagination nav lives in the
// base layout's footer, and a first page that grows a second one gains it.
const QUICK_FILTERS_SELECTOR = '[data-job-quick-filters]';
const PAGINATION_SELECTOR = 'footer nav[aria-label="Pagination"]';

/**
 * Morph one region of the page to its refreshed counterpart. A region present on
 * only one side is inserted or removed, so the pagination nav can appear and
 * disappear as the list grows and shrinks.
 */
function morphRegion(root, refreshed, selector, morph, insertInto) {
    const current = root.querySelector(selector);
    const next = refreshed.querySelector(selector);
    if (current && next) {
        morph(current, next);
    } else if (current) {
        current.remove();
    } else if (next && insertInto) {
        const parent = root.querySelector(insertInto);
        parent?.prepend(root.importNode ? root.importNode(next, true) : next);
    }
}

/**
 * Keep a native <details> open across a morph. The server always renders them
 * closed, and morph copies attributes, so without this every live refresh would
 * shut the card a reader had just opened.
 */
export function keepDetailsOpen() {
    return {
        updating(el, toEl) {
            if (el?.tagName === 'DETAILS' && toEl?.tagName === 'DETAILS' && el.open) toEl.setAttribute('open', '');
        },
    };
}

/**
 * The /jobs live refresh: batched (the first event schedules one refresh and
 * later ones join it), one request at a time, with a dirty bit so
 * an event arriving during a request produces one trailing refresh rather than
 * being lost. It refetches the page the reader is on — same filters, same keyset
 * position — and morphs the regions above in place.
 */
export function createJobListRefresher({
    root = document,
    fetchImpl = (...args) => fetch(...args),
    currentURL = () => window.location.href,
    morph = (from, to) => morphAndReinitChangedComponents(from, to, keepDetailsOpen()),
    onRowChanges = () => {},
    // Told after each refresh has morphed the list in.
    onRefreshed = () => {},
    onUnavailable = () => {},
    // Told when a refresh fails, and when one succeeds again, so the page can
    // say that what it shows may be out of date.
    onFailed = () => {},
    onRecovered = () => {},
    logger = console,
    debounceMs = JOB_LIST_REFRESH_DEBOUNCE_MS,
    retryMs = JOB_LIST_REFRESH_RETRY_MS,
} = {}) {
    let timer = null;
    let inFlight = false;
    let dirty = false;
    let destroyed = false;

    const refresh = async () => {
        if (destroyed || inFlight || !dirty) return;
        dirty = false;
        inFlight = true;
        try {
            const response = await fetchImpl(currentURL(), { headers: { Accept: 'text/html' } });
            if (response.redirected) {
                // An expired session answers with the login page after a redirect,
                // as a 200. That is not transient and not the list: stop, and say so,
                // rather than strip the page or poll the login form.
                destroyed = true;
                onUnavailable();
                return;
            }
            if (!response.ok) throw new Error(`job list refresh failed: HTTP ${response.status}`);
            const html = await response.text();
            if (destroyed) return;
            const refreshed = new DOMParser().parseFromString(html, 'text/html');
            const list = findListContainer(root);
            const nextList = findListContainer(refreshed);
            // A document without the list is not an empty list: every region below
            // would be removed on its word. Treat it as a failed refresh.
            if (!nextList) throw new Error('job list refresh returned a page without the list');
            if (list) {
                const before = rowStates(list);
                morph(list, nextList);
                localizeJobTimes(list);
                const changes = stateChanges(before, rowStates(list));
                if (changes.length) onRowChanges(changes);
                onRefreshed();
            }
            morphRegion(root, refreshed, QUICK_FILTERS_SELECTOR, morph, null);
            morphRegion(root, refreshed, PAGINATION_SELECTOR, morph, 'footer');
            onRecovered();
        } catch (error) {
            logger.error('Failed to refresh the job list:', error);
            onFailed(error);
            // The change that asked for this refresh is still unshown, and the next
            // event may never come: try again rather than leave the page stale.
            dirty = true;
            if (!destroyed && timer === null) schedule(retryMs);
            return;
        } finally {
            inFlight = false;
        }
        if (!destroyed && dirty && timer === null) schedule();
    };

    const schedule = (delay = debounceMs) => {
        if (timer !== null) clearTimeout(timer);
        timer = setTimeout(() => {
            timer = null;
            void refresh();
        }, delay);
    };

    return {
        request() {
            if (destroyed) return;
            dirty = true;
            // A refresh already due absorbs this event; restarting its timer instead
            // would let a steady stream of events postpone every refresh forever.
            if (timer === null && !inFlight) schedule();
        },
        destroy() {
            destroyed = true;
            if (timer !== null) clearTimeout(timer);
            timer = null;
        },
    };
}

/** Each rendered card's Job id, title and state, read from its selection payload. */
export function rowStates(container) {
    const states = new Map();
    for (const card of container.querySelectorAll('[data-job-id]')) {
        try {
            const entity = JSON.parse(card.querySelector('[data-entity]')?.dataset.entity || 'null');
            if (entity?.id) states.set(entity.id, entity);
        } catch {
            // A card without a readable payload has no state to compare.
        }
    }
    return states;
}

/**
 * The Jobs whose state a refresh changed, among those on the page both before and
 * after it. A row that appeared or left is a change of membership, which the
 * refreshed list itself shows; announcing it would repeat every filter's churn.
 */
export function stateChanges(before, after) {
    const changes = [];
    for (const [id, next] of after) {
        const previous = before.get(id);
        if (previous && previous.state !== next.state) changes.push(next);
    }
    return changes;
}

// A refresh's state changes as one message, each in the words the drawer uses,
// a failure with its reason.
export function stateChangeAnnouncement(changes) {
    if (!changes.length) return '';
    if (changes.length > 3) return `${changes.length} jobs changed state.`;
    return changes.map(job => lifecycleAnnouncement(job)).join(' ');
}

const TERMINAL_STATES = new Set(terminalStates());

/**
 * What a /jobs card's progress block shows for a Job, worked out as the server
 * draws the card (jobRowProgressBar and jobRowStats in job_template_context.go)
 * with the helpers the drawer and a Job's page use: the line above the bar, its
 * value, whether it pulses, what it says to a screen reader, and the speed line
 * (the live speed and time left while running, the average once finished).
 */
export function cardProgressView(job, now = Date.now()) {
    const progress = job?.progress || {};
    let stats = '';
    if (job?.state === 'running') {
        stats = [liveRateText(progress, now), liveEtaText(progress, now)].filter(Boolean).join(' · ');
    } else if (TERMINAL_STATES.has(job?.state)) {
        const average = formatRate(progress.averageRate, progress.unit);
        stats = average ? `average ${average}` : '';
    }
    return {
        text: progressText(job),
        value: progressValue(job),
        indeterminate: progressIndeterminate(job),
        accessible: progressAccessibleText(job),
        stats,
    };
}

/**
 * Draw a progress view into a card's progress block, in place of what the
 * server drew there. A card with no block (work that reported nothing yet) is
 * left for the refresh its next lifecycle change brings.
 */
export function applyCardProgress(card, view, title) {
    const block = card?.querySelector('[data-job-progress]');
    if (!block) return false;
    const text = block.querySelector('[data-job-progress-text]');
    const value = block.querySelector('[data-job-progress-value]');
    const bar = block.querySelector('[data-job-progress-bar]');
    const fill = block.querySelector('[data-job-progress-fill]');
    const stats = block.querySelector('[data-job-stats]');
    if (text) text.textContent = view.text;
    if (value) value.textContent = view.value !== null ? `${view.value}%` : view.indeterminate ? 'In progress' : '';
    if (bar) {
        if (view.value !== null) bar.setAttribute('aria-valuenow', String(view.value));
        else bar.removeAttribute('aria-valuenow');
        bar.setAttribute('aria-valuetext', view.accessible);
        bar.setAttribute('aria-label', `${title} progress: ${view.accessible}`);
    }
    if (fill) {
        fill.classList.toggle('w-full', view.value === null && view.indeterminate);
        fill.classList.toggle('motion-safe:animate-pulse', view.value === null && view.indeterminate);
        fill.style.width = view.value !== null ? `${view.value}%` : '';
    }
    if (stats) {
        stats.textContent = view.stats;
        stats.hidden = !view.stats;
    }
    return true;
}

function cardFor(root, jobId) {
    for (const card of root?.querySelectorAll?.('[data-job-id]') || []) {
        if (card.getAttribute('data-job-id') === jobId) return card;
    }
    return null;
}

function cardEntity(card) {
    try {
        return JSON.parse(card?.querySelector('[data-entity]')?.dataset.entity || 'null');
    } catch {
        return null;
    }
}

/**
 * The /jobs page component: one SSE connection that refreshes the server-rendered
 * list when Jobs change. History replayed before the stream catches up refreshes
 * the page once, at the catch-up boundary, rather than once per message. State changes of
 * Jobs already on the page are announced from the refresh itself, because a
 * stream message may name a Job without carrying its snapshot.
 */
export function jobList() {
    return {
        connectionStatus: 'connecting',
        notice: '',
        // Set while the list could not be refreshed; the refresher keeps trying.
        refreshFailed: false,
        eventSource: null,
        streamCaughtUp: false,
        lastSequence: 0,
        // The page was rendered before its stream connected at the head, so
        // what changed in between is in neither: the first catch-up reconciles.
        _missedWhileCatchingUp: true,
        _refresher: null,
        _liveRegion: null,
        _streamRetryTimer: null,
        _streamRetryDelay: 0,
        _preferences: null,
        _onRefreshRequest: null,
        _onNotice: null,
        // The newest live progress each card was drawn from, by Job id, and
        // the clock that counts their time left down between frames.
        _progress: new Map(),
        _progressClock: null,
        // Set once the stream has given a cursor, which a reopened stream then
        // resumes from, even v2:0.
        _holdsCursor: false,

        init() {
            this._liveRegion = createLiveRegion();
            localizeJobTimes(this.$root);
            this._refresher = createJobListRefresher({
                onRowChanges: changes => this.announceChanges(changes),
                onUnavailable: () => {
                    this.connectionStatus = 'unavailable';
                    clearTimeout(this._streamRetryTimer);
                    const source = this.eventSource;
                    this.eventSource = null;
                    source?.close();
                },
                onRefreshed: () => this.reapplyProgress(),
                onFailed: () => { this.refreshFailed = true; },
                onRecovered: () => { this.refreshFailed = false; },
            });
            this._onRefreshRequest = () => this._refresher.request();
            this._onNotice = event => { this.notice = event.detail?.message || ''; };
            window.addEventListener('job-list-refresh', this._onRefreshRequest);
            window.addEventListener('job-list-notice', this._onNotice);
            // A dismissal, pin or forget made elsewhere emits no Job event.
            this._preferences = openJobPreferenceChannel(() => this._refresher?.request());
            this.connect();
        },

        destroy() {
            clearTimeout(this._streamRetryTimer);
            clearInterval(this._progressClock);
            this._progressClock = null;
            const source = this.eventSource;
            this.eventSource = null;
            source?.close();
            this._refresher?.destroy();
            this._preferences?.close();
            this._liveRegion?.destroy();
            window.removeEventListener('job-list-refresh', this._onRefreshRequest);
            window.removeEventListener('job-list-notice', this._onNotice);
        },

        // The drawer announces the Jobs it follows, on this page as on every
        // other; the list says only the changes it does not follow, such as
        // another account's Job while an administrator's drawer lists their own.
        announceChanges(changes) {
            const message = stateChangeAnnouncement(changes.filter(job => !drawerAnnouncesJob(job)));
            if (message) this._liveRegion?.announce(message);
        },

        get connectionText() {
            if (this.connectionStatus === 'unavailable') return 'Live updates stopped; reload the page';
            if (this.refreshFailed) return 'The list could not be refreshed and may be out of date; trying again';
            if (this.connectionStatus === 'connected') return 'Live updates connected';
            if (this.connectionStatus === 'reconnecting') return 'Reconnecting to live updates';
            return 'Connecting to live updates';
        },

        connect() {
            if (this.eventSource || this.connectionStatus === 'unavailable') return;
            if (typeof EventSource === 'undefined') {
                this.connectionStatus = 'unavailable';
                return;
            }
            clearTimeout(this._streamRetryTimer);
            this._streamRetryTimer = null;
            const source = new EventSource(canonicalStreamURL(this.lastSequence, '', this._holdsCursor));
            this.eventSource = source;
            const current = handler => event => { if (this.eventSource === source) handler(event); };
            source.addEventListener('open', current(() => { this.connectionStatus = 'connected'; }));
            source.addEventListener('error', current(() => {
                this.connectionStatus = 'reconnecting';
                this.streamCaughtUp = false;
                if (source.readyState !== EVENT_SOURCE_CLOSED) return;
                // The browser will not try again; the page does, resuming from
                // its cursor, and reconciles once caught up.
                source.close();
                this.eventSource = null;
                this._missedWhileCatchingUp = true;
                this._streamRetryDelay = nextStreamRetryDelay(this._streamRetryDelay);
                this._streamRetryTimer = setTimeout(() => this.connect(), this._streamRetryDelay);
            }));
            source.addEventListener('job-caught-up', current((event) => {
                let boundary;
                try { boundary = JSON.parse(event.data); } catch { return; }
                const sequence = streamCursorSequence(boundary?.cursor);
                if (sequence === null) return;
                if (reloadAfterStreamReset(boundary, source)) return;
                this.lastSequence = Math.max(this.lastSequence, sequence);
                this._holdsCursor = true;
                this.streamCaughtUp = true;
                this._streamRetryDelay = 0;
                if (this._missedWhileCatchingUp) {
                    this._missedWhileCatchingUp = false;
                    this._refresher.request();
                }
            }));
            for (const name of ['message', 'job']) {
                source.addEventListener(name, current((event) => this.handleStreamMessage(event)));
            }
            source.addEventListener('job-progress', current((event) => this.handleProgressFrame(event)));
        },

        // A live progress frame redraws the progress of the card it names, as
        // the drawer's rows are: in place, never announced, and never a reason
        // to refetch the page, which a lifecycle change is. A frame older than
        // the card, or for a Job not on this page, changes nothing.
        handleProgressFrame(event) {
            let frame;
            try { frame = JSON.parse(event.data); } catch { return; }
            if (!frame?.jobId) return;
            const card = cardFor(this.$root, frame.jobId);
            const entity = cardEntity(card);
            if (!entity?.id) return;
            const held = this._progress.get(frame.jobId);
            // A frame reported before what the card already shows, drawn by a
            // refresh or by a later frame, would move its bar back.
            const reportedAt = Date.parse(frame.progress?.updatedAt || '');
            const drawnAt = Math.max(
                Date.parse(card.querySelector('[data-job-progress]')?.dataset.progressUpdatedAt || '') || 0,
                Date.parse(held?.progress?.updatedAt || '') || 0,
            );
            if (Number.isFinite(reportedAt) && reportedAt < drawnAt) return;
            const base = held && Number(held.version || 0) >= Number(entity.version || 0)
                ? { ...held, state: entity.state }
                : { ...entity, progress: {} };
            const next = applyProgressFrame(base, frame);
            if (next === base) return;
            this._progress.set(frame.jobId, next);
            applyCardProgress(card, cardProgressView(next, Date.now()), next.title || entity.title || 'Job');
            this.keepProgressClock();
        },

        // After a refresh, a card the server drew from older progress than a
        // frame the page holds is drawn from the frame again, so the refresh
        // does not move a bar back. A card whose Job left running, or whose
        // drawing is as new as the frame, is the server's.
        reapplyProgress() {
            for (const [jobId, held] of this._progress) {
                const card = cardFor(this.$root, jobId);
                const entity = cardEntity(card);
                const drawnAt = Date.parse(card?.querySelector('[data-job-progress]')?.dataset.progressUpdatedAt || '');
                const heldAt = Date.parse(held.progress?.updatedAt || '');
                if (!entity || entity.state !== 'running' || !(heldAt > drawnAt || Number.isNaN(drawnAt))) {
                    this._progress.delete(jobId);
                    continue;
                }
                const job = { ...held, state: entity.state, version: Math.max(Number(held.version || 0), Number(entity.version || 0)) };
                this._progress.set(jobId, job);
                applyCardProgress(card, cardProgressView(job, Date.now()), job.title || entity.title || 'Job');
            }
            this.keepProgressClock();
        },

        // "about 31 s left" counts down between frames, and a speed nothing has
        // reported for a while goes, as in the drawer: once a second while a
        // running card is drawn from a frame.
        keepProgressClock() {
            const running = [...this._progress.values()].some(job => job.state === 'running');
            if (running && !this._progressClock) {
                this._progressClock = setInterval(() => this.tickProgress(), 1000);
            } else if (!running && this._progressClock) {
                clearInterval(this._progressClock);
                this._progressClock = null;
            }
        },

        tickProgress() {
            const now = Date.now();
            for (const [jobId, job] of this._progress) {
                const card = cardFor(this.$root, jobId);
                if (!card) {
                    this._progress.delete(jobId);
                    continue;
                }
                applyCardProgress(card, cardProgressView(job, now), job.title || 'Job');
            }
            this.keepProgressClock();
        },

        handleStreamMessage(event) {
            const sequence = streamCursorSequence(event?.lastEventId);
            if (sequence !== null) this.lastSequence = Math.max(this.lastSequence, sequence);
            let message;
            try { message = JSON.parse(event.data); } catch { return; }
            // A replayed message is not announced as it arrives, but it is still a
            // change the rendered page predates: the one between rendering and
            // connecting, or everything missed while reconnecting. One refresh at
            // the catch-up boundary reconciles all of them.
            if (message?.replay === true || !this.streamCaughtUp) {
                this._missedWhileCatchingUp = true;
                return;
            }
            this._refresher.request();
        },
    };
}

// What a bulk command did, said in the past tense for the commands whose result
// the rows show; any other command is said as done.
const BULK_DONE = {
    pin: count => `Pinned ${count}`,
    unpin: count => `Unpinned ${count}`,
    dismiss: count => `Dismissed ${count}`,
    undismiss: count => `Returned ${count}`,
    retry: count => `Retried ${count}`,
    cancel: count => `Cancelled ${count}`,
};

// A refusal's code, for an outcome that carries no message of its own.
const BULK_REFUSAL_TEXT = {
    'not-advertised': 'No longer offered',
    'not-found': 'Not found',
    conflict: 'Changed meanwhile; try again',
    'chain-conflict': 'Already retried',
    'in-flight': 'Already running',
    'key-reused': 'Sent twice; try again',
};

function selectedJobs(count, total) {
    return `${count} of ${total} selected ${total === 1 ? 'job' : 'jobs'}`;
}

function namedList(names) {
    if (names.length <= 3) return names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names.at(-1)}` : names[0];
    return `${names.slice(0, 3).join(', ')} and ${names.length - 3} more`;
}

/**
 * A bulk command's results in words: one row per Job, named by its title and
 * linked, because the refresh the command causes can take its card away; and
 * one sentence for the notice and the live region, saying what was done to how
 * many of the selected Jobs, and which were not, by name.
 */
export function bulkCommandReport(command, ids, results, titles = {}) {
    const outcomes = (results || []).map(result => {
        const done = result.status === 'succeeded' && result.code === 'applied';
        const requested = result.code === 'requested';
        return {
            jobId: result.jobId,
            title: titles[result.jobId] || 'Job',
            url: `/job?id=${encodeURIComponent(result.jobId)}`,
            text: done ? 'Done' : requested ? 'Requested'
                : (result.message || BULK_REFUSAL_TEXT[result.code] || result.code || result.status || 'Not done'),
            outcome: done ? 'done' : requested ? 'requested' : 'refused',
        };
    });
    const total = ids.length;
    const count = kind => outcomes.filter(outcome => outcome.outcome === kind).length;
    const done = count('done');
    const requested = count('requested');
    const refused = outcomes.filter(outcome => outcome.outcome === 'refused');
    const label = commandLabel(command);
    const parts = [];
    if (done > 0) {
        const phrase = BULK_DONE[command?.key];
        parts.push(phrase
            ? `${phrase(selectedJobs(done, total))}${command.key === 'undismiss' ? ' to the list' : ''}.`
            : `${label} done for ${selectedJobs(done, total)}.`);
    }
    if (requested > 0) parts.push(`${label} requested for ${selectedJobs(requested, total)}.`);
    if (refused.length > 0) {
        const lead = parts.length ? 'Not done for' : `${label} was not done for`;
        const names = namedList(refused.map(outcome => outcome.title));
        parts.push(refused.length === 1
            ? `${lead} ${names}: ${refused[0].text}.`
            : `${lead} ${names}; each says why in the list of outcomes.`);
    }
    return {
        message: parts.join(' '),
        outcomes: outcomes.map(({ outcome: _, ...row }) => row),
    };
}

/**
 * What a bulk command asks before it runs: the Kind's own words when every
 * selected Job's command says the same thing, and a plain question when they
 * differ, since one Kind's warning ("Stop this download?") is wrong about another.
 */
export function bulkCommandConfirmation(command, jobs) {
    const asked = new Set((jobs || []).map(job => commandConfirmation(
        (job?.commands || []).find(offered => offered.key === command?.key) || command,
    )));
    if (asked.size <= 1) return asked.size ? [...asked][0] : commandConfirmation(command);
    return `${commandLabel(command)} the selected jobs?`;
}

function idempotencyKey() {
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `job-command-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

/**
 * The /jobs bulk bar's commands. Commands belong to each Job and are read from
 * its detail when it is selected — advertising them for every rendered row would
 * cost an adapter call per card on every render and every refresh. A detail is
 * re-read when the row's version moves, so an offer never outlives the state it
 * was computed from. The bar offers only what every selected Job advertises.
 */
export function jobBulkCommands({ fetchImpl = (...args) => fetch(...args) } = {}) {
    return {
        details: {},
        loading: false,
        busy: false,
        error: '',
        outcomes: [],
        _generation: 0,
        _root: null,
        _focusKeeper: null,

        init() {
            this.$watch(() => this.selectionKey(), () => { void this.sync(); });
            void this.sync();
            // A command that changes the offer takes away the button that ran it
            // (Pin becomes Unpin), and one that empties the selection hides the
            // whole bar. Focus goes to the button that replaced it, else Select
            // All, which shows once nothing is selected, else the first card,
            // else the page's main region when the list is left empty.
            // Kept: a method called from a directive sees that element as $el.
            this._root = this.$el || null;
            const bar = this.$el?.closest?.('.bulk-editors');
            this._focusKeeper = this.$el ? keepFocusWithin(this.$el, {
                observe: bar?.parentElement || this.$el,
                attributes: true,
                describe: element => (element.dataset?.commandKey ? { key: element.dataset.commandKey } : null),
                restore: ({ key }) => {
                    const target = bulkFocusTarget(this._root, key);
                    if (target) focusOn(target);
                },
            }) : null;
        },

        destroy() {
            this._focusKeeper?.stop();
        },

        selectionKey() {
            const selection = this.$selection;
            // A pin or a dismissal is the viewer's preference, not a change to the
            // Job, so it moves no version: the row's bits are part of the key too.
            return [...selection.selectedIds].map(id => {
                const entity = selection.options[id]?.entity;
                return `${id}:${entity?.version ?? ''}:${entity?.pinned ? 1 : 0}:${entity?.dismissed ? 1 : 0}`;
            }).join(',');
        },

        selectedIds() {
            return [...this.$selection.selectedIds];
        },

        async sync() {
            const generation = ++this._generation;
            const selection = this.$selection;
            // An error from an earlier read belonged to the selection that made it.
            this.error = '';
            // Outcomes outlive the refresh their command triggers, which reshapes the
            // selection; they end with the selection itself, when the bar hides.
            if (selection.selectedIds.size === 0) this.outcomes = [];
            const stale = this.selectedIds().filter(id => {
                const detail = this.details[id];
                const entity = selection.options[id]?.entity;
                return !detail || detail.version !== entity?.version || Boolean(detail.pinned) !== Boolean(entity?.pinned) ||
                    Boolean(detail.dismissed) !== Boolean(entity?.dismissed);
            });
            if (!stale.length) {
                // A newer selection with nothing to read supersedes any read still
                // in flight, which will now never clear the flag itself.
                this.loading = false;
                return;
            }
            this.loading = true;
            try {
                const read = await Promise.all(stale.map(async id => {
                    const response = await fetchImpl(`/v1/jobs/${encodeURIComponent(id)}`, { headers: { Accept: 'application/json' } });
                    if (!response.ok) throw new Error(`Could not read the commands of a selected job (${response.status}).`);
                    const payload = await response.json();
                    return payload.job || payload;
                }));
                if (generation !== this._generation) return;
                const details = { ...this.details };
                for (const detail of read) details[detail.id] = detail;
                this.details = details;
            } catch (error) {
                if (generation === this._generation) this.error = error.message;
            } finally {
                if (generation === this._generation) this.loading = false;
            }
        },

        commands() {
            const ids = this.selectedIds();
            const options = this.$selection.options;
            // An offer is only as current as the detail it was read from. A row a
            // live refresh moved past its cached detail offers nothing until the
            // re-read lands, rather than a command its new state may no longer have;
            // selectedBulkCommands answers nothing when any selected Job is missing.
            // The rendered row's pin and dismissal are current the moment the list
            // refreshes, since neither moves a version.
            const jobs = ids.map(id => {
                const detail = this.details[id];
                const entity = options[id]?.entity;
                if (!detail || detail.version !== entity?.version) return null;
                return { ...detail, pinned: Boolean(entity?.pinned), dismissed: Boolean(entity?.dismissed) };
            });
            return selectedBulkCommands(jobs.filter(Boolean), ids);
        },

        commandLabel(command) {
            return commandLabel(command);
        },

        /**
         * Every result of a command reaches three places: this bar (a failure as its
         * error), the live region, and the page notice. The bar is not enough on its
         * own, because a live refresh can remove every selected card — before,
         * during or after the request — and the bar hides with the selection.
         */
        report(message, failed = false) {
            if (failed) this.error = message;
            this.$selection.announce?.(message);
            window.dispatchEvent(new CustomEvent('job-list-notice', { detail: { message } }));
        },

        async run(command) {
            const ids = this.selectedIds();
            if (!ids.length || this.busy || this.loading) return;
            const confirmation = bulkCommandConfirmation(command, ids.map(id => this.details[id]));
            if (confirmation) {
                const accepted = await window.Alpine?.store('confirmDialog')?.ask(
                    `${confirmation} This applies to ${ids.length} selected ${ids.length === 1 ? 'job' : 'jobs'}.`,
                    {
                        title: commandLabel(command), confirmLabel: commandLabel(command), cancelLabel: commandDismissLabel(command),
                        destructive: command?.destructive === true, fallbackFocus: () => bulkFocusTarget(this._root, command.key),
                    },
                );
                if (!accepted) return;
                // The dialog blocks the reader, not the live refresh: a card can leave
                // the page while it is open. What was confirmed is the selection the
                // dialog named, so a changed one is refused rather than acted on.
                const now = this.selectedIds();
                if (now.length !== ids.length || now.some(id => !ids.includes(id))) {
                    this.report('The selection changed while you were confirming. Review it and try again.', true);
                    return;
                }
            }
            // The titles are read now: the refresh the command causes can take
            // a card, and its title, away before the answer is shown.
            const titles = Object.fromEntries(ids.map(id => [id, this.$selection.options[id]?.entity?.title || this.details[id]?.title || '']));
            const key = idempotencyKey();
            this.busy = true;
            this.outcomes = [];
            this.error = '';
            try {
                const response = await fetchImpl(`/v1/jobs/commands/${encodeURIComponent(command.key)}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'Idempotency-Key': key },
                    body: JSON.stringify({ jobIds: ids, idempotencyKey: key }),
                });
                const payload = await response.json().catch(() => ({}));
                if (response.ok) {
                    announcePreferenceCommand(`/v1/jobs/commands/${encodeURIComponent(command.key)}`, {
                        method: 'POST', body: JSON.stringify({ jobIds: ids }),
                    }, payload);
                }
                const report = bulkCommandReport(command, ids, payload.results || payload.outcomes || [], titles);
                this.outcomes = report.outcomes;
                if (response.ok) {
                    this.report(report.message);
                } else {
                    this.report(payload.error || `The bulk command could not be completed (${response.status}).`, true);
                }
            } catch (error) {
                this.report(error.message || 'The bulk command could not be completed.', true);
            } finally {
                this.busy = false;
                window.dispatchEvent(new CustomEvent('job-list-refresh'));
            }
        },
    };
}

// The control that stands in for a bulk command that is gone: its counterpart,
// the same command drawn again, the first command left, else Select All (shown
// once nothing is selected), the first card, or the page's main region.
function bulkFocusTarget(root, key) {
    const candidates = [
        ...commandFocusSuccessorKeys(key).map(other => root?.querySelector(`button[data-command-key="${CSS.escape(other)}"]`)),
        root?.querySelector('button[data-command-key]'),
        ...document.querySelectorAll('[data-bulk-select-all]'),
        document.querySelector('[data-job-id] a[href]'),
        document.querySelector('main'),
    ];
    return candidates.find(candidate => candidate?.isConnected && candidate.checkVisibility?.() !== false) || null;
}

// A summary duration, which the API gives in nanoseconds.
function summaryDuration(stats) {
    return `median ${formatDuration((stats?.median || 0) / 1e9)}, 95% within ${formatDuration((stats?.p95 || 0) / 1e9)}`;
}

async function answerOf(response, what) {
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || `${what} failed (${response.status}).`);
    return payload;
}

/**
 * The Job Center's summary panel: the figures of the Jobs the list's filter
 * selects, read when the reader asks for them, and a summary export of the
 * same filter over a longer range. `query` is that filter as the API reads it
 * (jobAPIQuery in job_template_context.go).
 */
export function jobSummary({ fetchImpl = (...args) => fetch(...args) } = {}) {
    return {
        query: '',
        // The filter as a summary export seals it (jobSummaryExportFilter):
        // the viewer's own Jobs asked for by id.
        exportQuery: '',
        window: '30d',
        loading: false,
        error: '',
        summary: null,
        exportFrom: '',
        exportTo: '',
        exportFormat: 'csv',
        exporting: false,
        exportError: '',
        exported: null,
        _read: 0,

        init() {
            this.query = this.$root?.dataset.summaryQuery || '';
            this.exportQuery = this.$root?.dataset.exportQuery || '';
        },

        // Each read is numbered: an answer for a window the reader has since
        // changed is not applied over the one they chose.
        async load() {
            const read = ++this._read;
            this.loading = true;
            this.error = '';
            try {
                const separator = this.query ? '&' : '';
                const response = await fetchImpl(`/v1/jobs/summary?${this.query}${separator}window=${encodeURIComponent(this.window)}`, { headers: { Accept: 'application/json' } });
                const summary = await answerOf(response, 'Reading the summary');
                if (read === this._read) this.summary = summary;
            } catch (error) {
                if (read === this._read) {
                    this.summary = null;
                    this.error = error.message;
                }
            } finally {
                if (read === this._read) this.loading = false;
            }
        },

        figures() {
            const summary = this.summary;
            if (!summary) return [];
            const failures = (summary.failures || []).map(failure => `${failure.class} ${failure.count}`).join(', ');
            return [
                { label: 'Jobs', value: String(summary.total) },
                { label: 'Succeeded', value: `${summary.succeeded} of ${summary.terminal} finished (${Math.round((summary.successRate || 0) * 100)}%)` },
                { label: 'Failed', value: String(summary.failed) },
                { label: 'Time queued', value: summaryDuration(summary.queue) },
                { label: 'Time running', value: summaryDuration(summary.run) },
                ...(failures ? [{ label: 'Failures by class', value: failures }] : []),
            ];
        },

        // The range is whole days in the reader's zone: from the first day's
        // start to the end of the last.
        async exportSummary() {
            // One export per press: a second while the first is on its way would
            // queue the same costly export twice.
            if (this.exporting) return;
            this.exporting = true;
            this.exportError = '';
            this.exported = null;
            try {
                const to = new Date(`${this.exportTo}T00:00`);
                to.setDate(to.getDate() + 1);
                const response = await fetchImpl(`/v1/jobs/summary/export${this.exportQuery ? `?${this.exportQuery}` : ''}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                    body: JSON.stringify({ from: new Date(`${this.exportFrom}T00:00`).toISOString(), to: to.toISOString(), format: this.exportFormat }),
                });
                const job = (await answerOf(response, 'The summary export')).job || {};
                this.exported = { url: `/job?id=${encodeURIComponent(job.id)}`, title: job.title || 'Job summary export' };
            } catch (error) {
                this.exportError = error.message;
            } finally {
                this.exporting = false;
            }
        },
    };
}

function pad(value, width = 2) {
    return String(value).padStart(width, '0');
}

function localInputValue(date, withSeconds) {
    const minute = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
    return withSeconds ? `${minute}:${pad(date.getSeconds())}` : minute;
}

/**
 * Show each card's times in the reader's own zone. The server renders them in
 * its zone, which the rest of the app shares, but the Job Center's detail page
 * and its filter inputs speak the reader's: without this, a card would label a
 * Job 15:00 that the filter calls 08:00. `data-local-time="seconds"` keeps the
 * seconds.
 */
export function localizeJobTimes(root = document) {
    for (const time of root.querySelectorAll('time[data-local-time][datetime]')) {
        const date = new Date(time.getAttribute('datetime'));
        if (Number.isNaN(date.getTime())) continue;
        time.textContent = localInputValue(date, time.dataset.localTime === 'seconds').replace('T', ' ');
    }
}

/**
 * Show an acceptance bound in the reader's own zone. A bound names a whole unit —
 * an end bound's last instant is the minute's :59.999 — so an instant on a minute
 * boundary (or an end bound at the last millisecond of one) shows to the minute,
 * and any other to the second.
 */
export function datetimeInputFromInstant(instant, end = false) {
    const date = new Date(instant);
    if (Number.isNaN(date.getTime())) return '';
    const endOfMinute = end && date.getSeconds() === 59 && date.getMilliseconds() === 999;
    const onMinute = date.getSeconds() === 0 && date.getMilliseconds() === 0;
    return localInputValue(date, !(endOfMinute || onMinute));
}

/**
 * The instant a datetime input's local value names, as RFC 3339. A start is the
 * unit's first instant and an end its last, matching how the server reads a
 * bound written to the minute or the second — but in the reader's zone rather
 * than the server's, which is the point of converting here.
 */
export function instantFromDatetimeInput(value, end = false) {
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?$/.test(value)) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    if (!end) return date.toISOString();
    // The last nanosecond of the unit, as the server reads a local bound: a Date
    // holds milliseconds, so the last one is written out to nanoseconds by hand.
    date.setTime(date.getTime() + (value.length > 16 ? 1000 : 60_000) - 1);
    return date.toISOString().replace(/\.(\d{3})Z$/, '.$1999999Z');
}

/**
 * The /jobs filter form's acceptance bounds. Without JavaScript the inputs submit
 * server-local times; with it they show and submit the reader's own. A bound the
 * reader did not touch goes back as the exact instant it arrived as, so a
 * bookmark's bound survives another filter changing.
 */
export function jobFilterTimes() {
    return {
        _onPageShow: null,

        init() {
            this.showInstants();
            // A page the back-forward cache brings back is the one the reader
            // left, form and all: the choices made for the page they went on
            // to, and the instants submit() swapped in. Its address is this
            // page's, so the form goes back to what was rendered for it.
            this._onPageShow = event => { if (event.persisted) this.restore(); };
            window.addEventListener('pageshow', this._onPageShow);
        },

        destroy() {
            window.removeEventListener('pageshow', this._onPageShow);
        },

        showInstants() {
            for (const input of this.timeInputs()) {
                const instant = input.dataset.instant;
                if (!instant) continue;
                input.value = datetimeInputFromInstant(instant, input.dataset.bound === 'end');
                if (input.value.length > 16) input.step = '1';
                input.dataset.shown = input.value;
            }
        },

        restore() {
            for (const hidden of this.$root.querySelectorAll('input[type="hidden"][data-bound-instant]')) hidden.remove();
            for (const input of this.timeInputs()) {
                if (input.dataset.name) input.name = input.dataset.name;
            }
            this.$root.reset();
            this.showInstants();
        },

        timeInputs() {
            return [...this.$root.querySelectorAll('input[type="datetime-local"][data-bound]')];
        },

        submit() {
            for (const input of this.timeInputs()) {
                if (!input.value || !input.name) continue;
                const unchanged = input.dataset.instant && input.value === input.dataset.shown;
                const instant = unchanged
                    ? input.dataset.instant
                    : instantFromDatetimeInput(input.value, input.dataset.bound === 'end');
                if (!instant) continue;
                const hidden = document.createElement('input');
                hidden.type = 'hidden';
                hidden.name = input.name;
                hidden.value = instant;
                hidden.dataset.boundInstant = '';
                input.dataset.name = input.name;
                input.removeAttribute('name');
                input.after(hidden);
            }
        },
    };
}
