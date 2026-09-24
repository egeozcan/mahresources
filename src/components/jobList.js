import { findListContainer } from '../utils/listContainer.js';
import { morphAndReinitChangedComponents } from '../utils/shortcodeElementMorph.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { commandConfirmation, commandLabel, selectedBulkCommands, stateLabel, streamCursorSequence } from './jobCenter.js';

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
    onUnavailable = () => {},
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
            }
            morphRegion(root, refreshed, QUICK_FILTERS_SELECTOR, morph, null);
            morphRegion(root, refreshed, PAGINATION_SELECTOR, morph, 'footer');
        } catch (error) {
            logger.error('Failed to refresh the job list:', error);
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

export function stateChangeAnnouncement(changes) {
    if (!changes.length) return '';
    if (changes.length > 3) return `${changes.length} jobs changed state.`;
    return changes.map(job => `${job.title || job.kind || 'Job'} ${stateLabel(job).toLowerCase()}.`).join(' ');
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
        eventSource: null,
        streamCaughtUp: false,
        _missedWhileCatchingUp: false,
        _refresher: null,
        _liveRegion: null,

        init() {
            this._liveRegion = createLiveRegion();
            localizeJobTimes(this.$root);
            this._refresher = createJobListRefresher({
                onRowChanges: changes => this._liveRegion?.announce(stateChangeAnnouncement(changes)),
                onUnavailable: () => {
                    this.connectionStatus = 'unavailable';
                    this.eventSource?.close();
                },
            });
            this._onRefreshRequest = () => this._refresher.request();
            this._onNotice = event => { this.notice = event.detail?.message || ''; };
            window.addEventListener('job-list-refresh', this._onRefreshRequest);
            window.addEventListener('job-list-notice', this._onNotice);
            this.connect();
        },

        destroy() {
            this.eventSource?.close();
            this._refresher?.destroy();
            this._liveRegion?.destroy();
            window.removeEventListener('job-list-refresh', this._onRefreshRequest);
            window.removeEventListener('job-list-notice', this._onNotice);
        },

        get connectionText() {
            if (this.connectionStatus === 'connected') return 'Live updates connected';
            if (this.connectionStatus === 'reconnecting') return 'Reconnecting to live updates';
            if (this.connectionStatus === 'unavailable') return 'Live updates stopped; reload the page';
            return 'Connecting to live updates';
        },

        connect() {
            if (this.eventSource) return;
            if (typeof EventSource === 'undefined') {
                this.connectionStatus = 'unavailable';
                return;
            }
            this.eventSource = new EventSource('/v1/jobs/events?version=2');
            this.eventSource.addEventListener('open', () => { this.connectionStatus = 'connected'; });
            this.eventSource.addEventListener('error', () => {
                this.connectionStatus = 'reconnecting';
                this.streamCaughtUp = false;
            });
            this.eventSource.addEventListener('job-caught-up', (event) => {
                let boundary;
                try { boundary = JSON.parse(event.data); } catch { return; }
                if (streamCursorSequence(boundary?.cursor) === null) return;
                this.streamCaughtUp = true;
                if (this._missedWhileCatchingUp) {
                    this._missedWhileCatchingUp = false;
                    this._refresher.request();
                }
            });
            for (const name of ['message', 'job']) {
                this.eventSource.addEventListener(name, (event) => this.handleStreamMessage(event));
            }
        },

        handleStreamMessage(event) {
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

        init() {
            this.$watch(() => this.selectionKey(), () => { void this.sync(); });
            void this.sync();
        },

        selectionKey() {
            const selection = this.$selection;
            // A pin is the viewer's preference, not a change to the Job, so it moves
            // no version: the row's pinned bit is part of the key too.
            return [...selection.selectedIds].map(id => {
                const entity = selection.options[id]?.entity;
                return `${id}:${entity?.version ?? ''}:${entity?.pinned ? 1 : 0}`;
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
                return !detail || detail.version !== entity?.version || Boolean(detail.pinned) !== Boolean(entity?.pinned);
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
            // The rendered row's pin state is current the moment the list refreshes,
            // since a pin moves no version.
            const jobs = ids.map(id => {
                const detail = this.details[id];
                const entity = options[id]?.entity;
                if (!detail || detail.version !== entity?.version) return null;
                return { ...detail, pinned: Boolean(entity?.pinned) };
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
            const confirmation = commandConfirmation(command);
            if (confirmation) {
                const accepted = await window.Alpine?.store('confirmDialog')?.ask(
                    `${confirmation} This applies to ${ids.length} selected ${ids.length === 1 ? 'job' : 'jobs'}.`,
                    { title: commandLabel(command), confirmLabel: commandLabel(command) },
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
                this.outcomes = payload.results || payload.outcomes || [];
                const applied = this.outcomes.filter(outcome => outcome.status === 'succeeded' || outcome.code === 'applied').length;
                if (response.ok) {
                    this.report(`${applied} of ${ids.length} ${ids.length === 1 ? 'job' : 'jobs'}: ${commandLabel(command).toLowerCase()}.`);
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
        init() {
            for (const input of this.timeInputs()) {
                const instant = input.dataset.instant;
                if (!instant) continue;
                input.value = datetimeInputFromInstant(instant, input.dataset.bound === 'end');
                if (input.value.length > 16) input.step = '1';
                input.dataset.shown = input.value;
            }
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
                input.removeAttribute('name');
                input.after(hidden);
            }
        },
    };
}
