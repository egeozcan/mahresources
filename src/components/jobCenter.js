import { announcePreferenceCommand, openJobPreferenceChannel, preferenceCommand } from '../utils/jobPreferenceChannel.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { tellDrawerOfJobs } from '../utils/jobAnnouncements.js';
import { focusOn, keepFocusWithin } from '../utils/focus.js';
import {
    applyProgressFrame,
    formatAmount,
    mergeFetchedProgress,
    liveEtaText,
    liveRateText,
    formatMetric,
    formatRate,
    metricSummary,
    graphLatest,
    graphSeries,
    graphSummary,
    sparklinePath,
} from './jobProgress.js';
import { isWorking, presentState, scheduledStartText } from './jobStates.js';

export { JOB_STATES } from './jobStates.js';

export function advertisedCommands(job) {
    return Array.isArray(job?.commands) ? job.commands : [];
}

// Pin and Unpin, and Dismiss and Undismiss, are advertised in pairs, since
// each is idempotent and a mixed selection may take either; one Job shows the
// one its viewer's own preference leaves to do.
export function jobCommands(job) {
    return advertisedCommands(job).filter(command => {
        if (command?.key === 'pin') return job?.pinned !== true;
        if (command?.key === 'unpin') return job?.pinned === true;
        if (command?.key === 'dismiss') return job?.dismissed !== true;
        if (command?.key === 'undismiss') return job?.dismissed === true;
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

// Where focus goes when the command control that had it is re-rendered away:
// its counterpart first, since Pin becomes Unpin, then the same command drawn
// again.
const COMMAND_COUNTERPARTS = { pin: 'unpin', unpin: 'pin', pause: 'resume', resume: 'pause', dismiss: 'undismiss', undismiss: 'dismiss' };

export function commandFocusSuccessorKeys(key) {
    return [COMMAND_COUNTERPARTS[key], key].filter(Boolean);
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

// A command may answer with a page to open: its outcome's detail names a
// location on this site (the run history "Inspect command history" resolves
// to). Anything that is not a same-origin path is never followed.
export function commandLocation(outcome) {
    const detail = outcome?.detail;
    if (!detail || typeof detail !== 'object') return '';
    return safeResultURL(detail.location);
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
    const entity = openableEntityOutput(outputs.filter(output => output?.key !== FAILURE_OUTPUT_KEY), outputs);
    if (entity) return entity;
    if (job.kind !== 'plugin-action') return null;
    return outputs.find(output => {
        if (output?.type !== 'summary' || output.availability !== 'available' || !output.destinationUrl) return false;
        const url = outputLinkURL(output, outputs);
        return url === output.destinationUrl && Boolean(safeResultURL(url));
    }) || null;
}

// The entity a failed job's failure is about: the resource a download collided
// with, which the library already held. It is named by its key, never inferred
// from "some entity output", because a job that published an entity and then
// failed would otherwise present what it made as what its failure was about. It
// is shown only while the failure is the collision itself: an output cannot be
// withdrawn, so a replay of the same Job that failed differently, or succeeded,
// still carries it, and it is never the job's result. The endpoint re-checks
// that the viewer may open it, as a result link's does. Go names both in
// application_context (JobDownloadExistingResourceOutput, JobDownloadResourceExistsCode).
export const FAILURE_OUTPUT_KEY = 'existing-resource';
const FAILURE_OUTPUT_CODE = 'resource-exists';

export function failureOutput(job) {
    if (job?.state !== 'failed' || job?.failure?.code !== FAILURE_OUTPUT_CODE) return null;
    const outputs = advertisedOutputs(job);
    return openableEntityOutput(outputs.filter(output => output?.key === FAILURE_OUTPUT_KEY), outputs);
}

function openableEntityOutput(candidates, outputs = candidates) {
    return candidates.find(output => output?.type === 'entity' &&
        output.availability === 'available' &&
        safeResultURL(outputLinkURL(output, outputs))) || null;
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

// The label every Job surface shows for a state (server/jobview/job_states.json).
export function stateLabel(job) {
    return presentState(job).label;
}

// When scheduled work starts, for a Job still waiting for its time.
export function scheduledText(job, now = Date.now()) {
    return scheduledStartText(job, now);
}

// Why a Job failed, in the words its Kind recorded: the message, or the code when
// the Kind gave none, so a failed row never stands without a reason. Only a
// failed Job carries a failure; the Service refuses one on any other transition.
export function failureText(job) {
    const failure = job?.failure;
    if (!failure) return '';
    return String(failure.message ?? '').trim() || String(failure.code ?? '').trim();
}

// The phase shown beside the state, or nothing when the state label already
// says it: a partial success's label is its phase.
// A Job's owner or actor as the server named it, "Deleted account" when that
// account has been deleted, and the bare user number only when the server named
// nobody: a viewer is told another account's name only if they administer it.
export function jobAccountText(job, role) {
    if (job?.[`${role}Deleted`]) return 'Deleted account';
    const name = job?.[`${role}Name`];
    if (name) return name;
    const id = job?.[`${role}UserId`];
    return id === null || id === undefined ? '' : `Account ${id}`;
}

export function phaseText(job) {
    return isPartialSuccess(job) ? '' : String(job?.phase || '');
}

// The drawer group a state belongs to: attention, active, finished (the finished
// Jobs that need no attention) or other.
export function classifyJobState(jobOrState) {
    return presentState(jobOrState).group;
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
    const hasDismissed = chosenJobs.some(job => job.dismissed === true);
    const hasUndismissed = chosenJobs.some(job => job.dismissed !== true);
    return advertisedCommands(chosenJobs[0])
        .filter(command => command.bulk && commandMaps.every(commands => commands.has(command.key)))
        .filter(command => {
            if (command.key === 'pin' && hasPinned && !hasUnpinned) return false;
            if (command.key === 'unpin' && hasUnpinned && !hasPinned) return false;
            if (command.key === 'dismiss' && hasDismissed && !hasUndismissed) return false;
            if (command.key === 'undismiss' && hasUndismissed && !hasDismissed) return false;
            return true;
        });
}

function announcementFor(job, previous, replay) {
    if (replay || !previous || stateOf(job) === stateOf(previous)) return '';
    return lifecycleAnnouncement(job);
}

// What is said when a Job reaches its current state.
export function lifecycleAnnouncement(job) {
    const reason = failureText(job);
    return `${job.title || job.kind || 'Job'} ${stateLabel(job).toLowerCase()}${reason ? `: ${reason}` : ''}.`;
}

// The Job a stream message carries, if it carries one.
export function eventJob(message) {
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
        ...mergeJobSnapshot(current, incoming),
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

// The canonical stream for a page that shows Jobs. One that holds no cursor
// starts at the head: it reads (or was rendered with) its Jobs itself and
// reconciles once caught up, so the history published before it would only be
// replayed to be ignored, on every page load. One that holds a cursor resumes
// from it, v2:0 included (a database that had published nothing when the page
// caught up), so what it missed while disconnected is replayed and a database
// restored under it is still recognised. owner=me narrows it to the viewer's
// own Jobs, as the lists take it.
export function canonicalStreamURL(lastSequence = 0, ownerScope = '', caughtUpOnce = false) {
    const params = new URLSearchParams({ version: '2' });
    if (caughtUpOnce || lastSequence > 0) params.set('cursor', `v2:${lastSequence}`);
    else params.set('start', 'head');
    if (ownerScope === 'me') params.set('owner', 'me');
    return `/v1/jobs/events?${params}`;
}

// EventSource.CLOSED: the browser gave up on the stream, as it does on any
// answer but a 200 (a proxy's 502, a 401 once the session ended), and will not
// reconnect by itself.
export const EVENT_SOURCE_CLOSED = 2;
const STREAM_RETRY_MIN_MS = 1000;
const STREAM_RETRY_MAX_MS = 30000;

// How long a page waits before opening its stream again after the browser gave
// up on it, doubling from one attempt to the next.
export function nextStreamRetryDelay(previous = 0) {
    return previous > 0 ? Math.min(previous * 2, STREAM_RETRY_MAX_MS) : STREAM_RETRY_MIN_MS;
}

// Answers a `job-caught-up` whose `reset` says the cursor this page resumed from
// was never issued by the database now serving it: one restored from an older
// backup, or wiped. Everything the page holds, every answer still on its way
// and every dialog still open belongs to the database that did, so the page is
// loaded again rather than repaired in place, and the stream is closed first so
// nothing it says in the meantime is applied. Returns whether it reloaded.
export function reloadAfterStreamReset(boundary, eventSource) {
    if (boundary?.reset !== true) return false;
    eventSource?.close?.();
    globalThis.location?.reload?.();
    return true;
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
    if (progress.phase) return progress.phase;
    if (completed !== null && completed !== undefined) return String(completed);
    if (isWorking(job)) return 'Working';
    // Work nobody is doing that reported only metrics names the first of them,
    // as the /jobs card does.
    return Array.isArray(progress.metrics) ? metricSummary(progress.metrics[0]) : '';
}

// Whether a Job has progress to show: something it reported, a success (which
// reads as complete), or work an executor is doing now. A Job that is waiting,
// paused or stopped with nothing reported shows none; its phase alone is shown
// beside its state.
export function showsProgress(job) {
    if (stateOf(job) === 'succeeded' || isWorking(job)) return true;
    const progress = job?.progress || {};
    // A total alone reports no work done.
    return Number.isFinite(progress.completed) || !!progress.message ||
        (Array.isArray(progress.metrics) && progress.metrics.length > 0);
}

export function progressValue(job) {
    if (progressSupersededBySuccess(job)) return 100;
    const progress = job?.progress || {};
    if (progress.completed === null || progress.completed === undefined || !(progress.total > 0)) return null;
    return Math.max(0, Math.min(100, Math.round((progress.completed / progress.total) * 100)));
}

// Whether the progress bar may animate and read "In progress". An unknown total
// on work nobody is doing — waiting, paused or stopped — stays unknown, but
// nothing is working on it.
export function progressIndeterminate(job) {
    return progressValue(job) === null && isWorking(job);
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
        // A requested control's notice leaves once the Job has moved past the
        // version the command answered with: the page says the rest.
        _noticeWatch: null,
        // A command is in flight; the controls take no second press.
        commandBusy: false,
        connectionStatus: 'disconnected',
        eventSource: null,
        lastSequence: 0,
        streamCaughtUp: false,
        _streamRetryTimer: null,
        _streamRetryDelay: 0,
        _reconcileOnCatchUp: true,
        _reconcileAfterLoad: false,
        _reconcileTimer: null,
        _reconcileDelay: 0,
        // Moves when this Job's viewer preferences change (a pin here or on
        // another page): a read begun before is older than what the page
        // holds, since a pin moves no version.
        _preferenceEpoch: 0,
        _preferences: null,
        // Set by destroy(): nothing is read or retried after it.
        _destroyed: false,
        // Set once the stream has given a cursor (a catch-up), which a reopened
        // stream then resumes from, even v2:0.
        _holdsCursor: false,
        now: Date.now(),
        _clockTimer: null,
        _liveRegion: null,

        init() {
            if (!this.detailId) {
                this.detailId = new URLSearchParams(globalThis.location?.search || '').get('id') || '';
            }
            this._liveRegion = createLiveRegion();
            this._preferences = openJobPreferenceChannel(message => this.hearPreferenceChange(message));
            // Keeps "about 14 s left" counting down between progress frames.
            this._clockTimer = setInterval(() => { this.now = Date.now(); }, 1000);
            // Kept: a method called from a directive sees that element as $el.
            this._root = this.$el || null;
            this._focusKeeper = this.$el ? keepCommandFocus(this.$el) : null;
            this.connect();
            this.load();
        },

        destroy() {
            this._destroyed = true;
            this._focusKeeper?.stop();
            if (this._clockTimer) clearInterval(this._clockTimer);
            clearTimeout(this._streamRetryTimer);
            clearTimeout(this._reconcileTimer);
            this._preferences?.close();
            this._preferences = null;
            const source = this.eventSource;
            this.eventSource = null;
            source?.close();
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
            const changed = preferenceCommand(url, init, payload);
            if (changed?.jobIds.includes(this.detailId)) this._preferenceEpoch += 1;
            // Sent on the channel this page listens on, which does not hear
            // its own messages; every other page, this tab's drawer included,
            // does.
            announcePreferenceCommand(url, init, payload, this._preferences);
            return payload;
        },

        async load() {
            this.loading = true;
            this.error = '';
            const epoch = this._preferenceEpoch;
            try {
                await this.loadDetail(this.detailId);
                // A preference changed while the page read its Job: the read
                // may predate it, so it is read again.
                if (epoch !== this._preferenceEpoch) this._reconcileAfterLoad = true;
            } catch (error) {
                this.error = error.message || 'Could not load this job.';
            } finally {
                this.loading = false;
                // A failed read keeps the obligation for the read that succeeds.
                if (this._reconcileAfterLoad && this.detail) {
                    this._reconcileAfterLoad = false;
                    this.reconcileDetail();
                }
            }
        },

        async loadDetail(id) {
            if (!id) throw new Error('A job ID is required.');
            const payload = await this.fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            this.detail = payload.job || payload;
            // The Job is followed from here, before its timeline is read: a
            // stream message about it while that read runs must find it.
            this.details[id] = this.detail;
            this.jobs = this.detail ? [this.detail] : [];
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
            return this.detail;
        },

        async refreshJobPreference(id) {
            const epoch = this._preferenceEpoch;
            const payload = await this.fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
            const freshJob = payload.job || payload;
            // A pin changed elsewhere while this was read: read it again.
            if (epoch !== this._preferenceEpoch) this.reconcileDetail();
            else if (freshJob?.id) this.updateJob(freshJob);
            return freshJob;
        },

        get noticeText() {
            const watch = this._noticeWatch;
            if (watch && requestPassed(watch, this.detail)) return '';
            return this.notice;
        },

        // The Job's detail read again after a command: the answer's snapshot
        // carries no commands, and a command such as Forget moves no version, so
        // nothing else would replace the controls the command made stale. It is
        // the preference-fenced read, so a pin changed elsewhere meanwhile is
        // read again rather than overwritten.
        async rereadJob(id) {
            const freshJob = await this.refreshJobPreference(id);
            return freshJob?.id ? freshJob : null;
        },

        // Another page (or this page's drawer) changed this Job's viewer
        // preferences: no Job event says so, so the page reads it again.
        hearPreferenceChange(message) {
            if (!Array.isArray(message?.jobIds) || !message.jobIds.map(String).includes(String(this.detailId))) return;
            this._preferenceEpoch += 1;
            // The page's own read is still on its way: load() reads again
            // once it lands, because the epoch moved under it.
            // Whichever read applies a change of state first says it, once
            // the stream is live: this one may overtake the read the change's
            // own event started, which then finds nothing new.
            if (this.detail) this.reconcileDetail({ announce: this.streamCaughtUp });
        },

        detailURL(job) {
            return `/job?id=${encodeURIComponent(job?.id || '')}`;
        },

        // One command at a time: a second press while the first is in flight
        // would be decided from the version the first is changing.
        async runCommand(job, command) {
            if (this.commandBusy) return null;
            this.commandBusy = true;
            try {
                return await this.runCommandUnguarded(job, command);
            } finally {
                this.commandBusy = false;
            }
        },

        async runCommandUnguarded(job, command) {
            const confirmation = commandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(confirmation, {
                    ...commandConfirmOptions(job, command),
                    fallbackFocus: () => commandFocusTarget(this._root, command.key),
                });
                if (!accepted) return null;
            }
            const key = idempotencyKey();
            try {
                const payload = await this.fetchJSON(commandEndpoint(job, command), {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                    body: JSON.stringify({ expectedVersion: command.jobVersion ?? job.version, idempotencyKey: key }),
                });
                const outcome = payload.result || payload;
                const freshJob = outcome.job || payload.job;
                if (freshJob?.id) this.updateJob(freshJob);
                let now = freshJob?.id ? freshJob : null;
                let rereadFailed = false;
                try { now = await this.rereadJob(job.id) || now; }
                catch { rereadFailed = true; }
                // This page is the Job's own, so a successor or a page the answer
                // names is opened: nothing unsaved is left behind here.
                const successorId = outcome.successorId || outcome.successorID || payload.successorId || payload.successorID;
                const location = successorId ? `/job?id=${encodeURIComponent(successorId)}` : commandLocation(outcome);
                if (location) globalThis.location?.assign?.(location);
                const settled = requestSettled(job, freshJob, now, outcome);
                this.notice = rereadFailed && (command?.key === 'pin' || command?.key === 'unpin')
                    ? `${commandLabel(command)} completed. Reload this job to see its current pin status.`
                    : settled ? lifecycleAnnouncement({ ...job, ...latestSnapshot(freshJob, now) }) : commandNoticeText(job, command, outcome);
                this._noticeWatch = outcome.code === 'requested' && !settled ? requestWatch(job) : null;
                this._liveRegion?.announce(this.notice);
                return outcome;
            } catch (error) {
                const fresh = error.payload?.job || error.payload?.result?.job || error.payload?.snapshot;
                if (fresh?.id) this.updateJob(fresh);
                let now = fresh?.id ? fresh : null;
                // Whatever refused it, the controls offered a command the Job did
                // not take: they are read again.
                if (error.status !== 404) {
                    try { now = await this.rereadJob(job.id) || now; }
                    catch { /* the refusal is still said */ }
                }
                this.notice = commandRefusalText(job, command, error, now);
                this._noticeWatch = null;
                this._liveRegion?.announce(this.notice);
                return null;
            }
        },

        connect() {
            if (this.eventSource || typeof EventSource === 'undefined') return;
            clearTimeout(this._streamRetryTimer);
            this._streamRetryTimer = null;
            if (this.connectionStatus !== 'reconnecting') this.connectionStatus = 'connecting';
            const source = new EventSource(canonicalStreamURL(this.lastSequence, '', this._holdsCursor));
            this.eventSource = source;
            const current = handler => event => { if (this.eventSource === source) handler(event); };
            source.addEventListener('open', current(() => { this.connectionStatus = 'connected'; }));
            source.addEventListener('error', current(() => {
                this.connectionStatus = 'reconnecting';
                this.streamCaughtUp = false;
                if (source.readyState !== EVENT_SOURCE_CLOSED) return;
                // The browser will not try again; the page does, resuming from
                // its cursor.
                source.close();
                this.eventSource = null;
                this._streamRetryDelay = nextStreamRetryDelay(this._streamRetryDelay);
                this._streamRetryTimer = setTimeout(() => this.connect(), this._streamRetryDelay);
            }));
            source.addEventListener('job-caught-up', current(event => this.markStreamCaughtUp(event)));
            source.addEventListener('job-progress', current(event => this.handleProgressFrame(event)));
            for (const eventName of ['message', 'job']) {
                source.addEventListener(eventName, current(event => this.handleStreamMessage(event)));
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
            if (reloadAfterStreamReset(boundary, this.eventSource)) return;
            this.lastSequence = Math.max(this.lastSequence, sequence);
            this._holdsCursor = true;
            this.streamCaughtUp = true;
            this._streamRetryDelay = 0;
            // The page read its Job while the stream was connecting at the
            // head, so a change made between the two is in neither: it is read
            // again once, now, or once the page's own read has finished.
            if (this._reconcileOnCatchUp) {
                this._reconcileOnCatchUp = false;
                if (this.loading || !this.detail) this._reconcileAfterLoad = true;
                else this.reconcileDetail();
            }
        },

        // Reads this page's Job again and applies it through the snapshot
        // guard. The obligation outlives a failed read: it is tried again after
        // a delay that doubles up to a minute, and a read begun before a
        // preference change is read again rather than applied.
        reconcileDetail({ announce = false } = {}) {
            if (!this.detailId || this._destroyed) return;
            clearTimeout(this._reconcileTimer);
            this._reconcileTimer = null;
            const epoch = this._preferenceEpoch;
            this.fetchJSON(`/v1/jobs/${encodeURIComponent(this.detailId)}`)
                .then(payload => {
                    if (this._destroyed) return;
                    this._reconcileDelay = 0;
                    if (epoch !== this._preferenceEpoch) {
                        this.reconcileDetail({ announce });
                        return;
                    }
                    const snapshot = payload.job || payload;
                    if (snapshot?.id && this.jobs.some(job => job.id === snapshot.id)) this.applyStreamSnapshot(snapshot, null, announce);
                })
                .catch(() => {
                    if (this._destroyed) return;
                    this._reconcileDelay = Math.min(Math.max(this._reconcileDelay * 2, 2000), 60000);
                    this._reconcileTimer = setTimeout(() => this.reconcileDetail({ announce }), this._reconcileDelay);
                });
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
                if (!this.jobs.some(job => job.id === result.jobId)) {
                    // This page's Job, while its own read has not landed: that
                    // read may predate the change, so it is read again after.
                    if (String(result.jobId) === String(this.detailId)) this._reconcileAfterLoad = true;
                    return;
                }
                const epoch = this._preferenceEpoch;
                this.fetchJSON(`/v1/jobs/${encodeURIComponent(result.jobId)}`)
                    .then(payload => {
                        const snapshot = payload.job || payload;
                        if (!this.jobs.some(job => job.id === snapshot.id)) return;
                        // A preference changed during the read (a pin, or
                        // Forget taking Retry away): none of this answer is
                        // applied, and a fresh read carries the Job's change,
                        // said as this one would have been.
                        if (epoch !== this._preferenceEpoch && String(snapshot.id) === String(this.detailId)) {
                            this.reconcileDetail({ announce: announceSnapshot });
                            return;
                        }
                        this.applyStreamSnapshot(snapshot, null, announceSnapshot);
                    })
                    // A read that failed repairs nothing: this page's Job is
                    // read again until it answers, and the change it carries
                    // is still said.
                    .catch(() => { if (String(result.jobId) === String(this.detailId)) this.reconcileDetail({ announce: announceSnapshot }); });
                return;
            }
            const incoming = eventJob(message);
            const previous = this.jobs.find(job => job.id === incoming?.id);
            this.jobs = result.jobs;
            if (result.announcement) this.sayLifecycle(previous, incoming, result.announcement);
        },

        // A Job's state change goes to the drawer's ledger, which says it once
        // if the drawer follows the Job (utils/jobAnnouncements.js); this page
        // says it only when the drawer hands it back.
        sayLifecycle(previous, job, text) {
            if (text && tellDrawerOfJobs([{ previous, next: job }]).length) this._liveRegion?.announce(text);
        },

        updateJob(job) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence);
            this.applyStreamSnapshot(job, result);
        },

        applyStreamSnapshot(job, previousResult = null, announce = false) {
            if (!job?.id) return;
            // A snapshot older than the one the page shows (a read answered
            // after a live update) replaces nothing.
            const shown = this.detail?.id === job.id ? this.detail : this.details[job.id];
            if (Number(job.version || 0) < Number(shown?.version || 0)) return;
            const result = previousResult || reduceJobStreamEvent(this.jobs, { job }, this.lastSequence);
            const held = new Map(this.jobs.map(current => [current.id, current]));
            if (result.changed) this.jobs = result.jobs.map(next => mergeFetchedProgress(next, held.get(next.id)));
            // An older snapshot than the one shown is not applied: it would roll
            // the page back.
            const stale = current => current && Number(job.version || 0) > 0 && Number(job.version || 0) < Number(current.version || 0);
            if (!stale(this.details[job.id])) this.details[job.id] = mergeFetchedProgress(mergeJobSnapshot(this.details[job.id], job), this.details[job.id]);
            if (this.detail?.id === job.id && !stale(this.detail)) this.detail = mergeFetchedProgress(mergeJobSnapshot(this.detail, job), this.detail);
            if (announce && result.announcement) this.sayLifecycle(held.get(job.id), job, result.announcement);
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
        scheduledText(job) { return scheduledText(job, this.now); },
        showsProgress(job) { return showsProgress(job); },
        phaseText(job) { return phaseText(job); },
        accountText(job, role) { return jobAccountText(job, role); },
        stateClass(job) { return classifyJobState(job); },
        commandLabel(command) { return commandLabel(command); },
        advertisedCommands(job) { return advertisedCommands(this.details[job.id] || job); },
        commandsFor(job) { return jobCommands(this.details[job.id] || job); },
        advertisedOutputs(job) { return advertisedOutputs(job); },
        failureOutput(job) { return failureOutput(job); },
        warningEvents() { return warningEvents(this.timeline); },
        outputEndpoint(output) { return outputEndpoint(output); },
        outputLinkURL(output, outputs) { return outputLinkURL(output, outputs); },
        outputJSONLinkURL(output, outputs) { return outputJSONLinkURL(output, outputs); },
        outputLinkLabel(output, outputs) { return outputLinkLabel(output, outputs); },
        outputLinkAccessibleLabel(output, outputs) { return outputLinkAccessibleLabel(output, outputs); },
    };
}

// The dismiss label of a command's confirmation. Pause is asked about a download
// that also offers Cancel, so a "Cancel" button in its dialog reads as cancelling
// the download: it dismisses with "Go back" instead. Every other command keeps the
// dialog's own default.
export function commandDismissLabel(command) {
    return command?.key === 'pause' ? 'Go back' : undefined;
}

const FORGET_CONFIRMATION = 'Forget this job’s saved replay input. Retry, Continue and Repeat will no longer be possible. Its sanitized history remains, and its outputs and artifacts are not affected. This cannot be undone.';

// A command asks first only when it stops work or cannot be taken back: a
// Kind's own confirmation, a destructive command, and Forget. Dismiss, Pin and
// their inverses change only the viewer's own list and retention, and each can
// be undone, so they run at once, however many Jobs they reach.
// The detail page's commands are drawn from the Job's current offer, so a
// command that changes it takes away the button that ran it, and so does a live
// change that ends the Job. Focus goes to the button that replaced it, else the
// first command left, else the Job's heading.
function keepCommandFocus(root) {
    const group = () => root.querySelector('[role="group"][aria-label="Advertised job commands"]');
    return keepFocusWithin(root, {
        describe: element => (element.dataset?.commandKey && group()?.contains(element) ? { key: element.dataset.commandKey } : null),
        restore: ({ key }) => {
            const target = commandFocusTarget(root, key);
            if (target) focusOn(target);
        },
    });
}

// The control that stands in for a detail-page command that is gone: its
// counterpart, the same command drawn again, the first command left, else the
// Job's heading.
function commandFocusTarget(root, key) {
    const buttons = root?.querySelector('[role="group"][aria-label="Advertised job commands"]');
    const candidates = [
        ...commandFocusSuccessorKeys(key).map(other => buttons?.querySelector(`button[data-command-key="${CSS.escape(other)}"]`)),
        buttons?.querySelector('button'),
        root?.querySelector('h1'),
    ];
    return candidates.find(candidate => candidate?.isConnected && candidate.checkVisibility?.() !== false) || null;
}

export function commandConfirmation(command) {
    if (command?.key === 'forget') return FORGET_CONFIRMATION;
    if (command?.confirmation) return command.confirmation;
    if (command?.destructive) return `Run ${commandLabel(command)}?`;
    return '';
}

// The confirmation dialog's title names the Job as well as the command, and
// its confirming button is styled as destructive only for a command that is.
export function commandConfirmOptions(job, command) {
    const label = commandLabel(command);
    const title = String(job?.title || job?.kind || '').trim();
    return {
        title: title ? `${label}: ${title}` : label,
        confirmLabel: label,
        cancelLabel: commandDismissLabel(command),
        destructive: command?.destructive === true,
    };
}

function jobName(job) {
    return String(job?.title || job?.kind || 'This job').trim();
}

// What a refused command says. A Kind's refusal is its own reason; the other
// refusals are the service's codes, said in terms of this Job. `now` is the
// Job as read after the refusal, when it could be read.
export function commandRefusalText(job, command, error, now = null) {
    const payload = error?.payload || {};
    const result = payload.result || {};
    const label = commandLabel(command);
    const name = jobName(job);
    switch (result.code) {
    case 'refused':
        return withReason(`${label} refused for ${name}`, result.message || payload.error);
    case 'not-advertised':
        return now?.state && stateOf(now) !== stateOf(job)
            ? `${label} is no longer offered for ${name}, which is now ${stateLabel(now).toLowerCase()}.`
            : `${label} is no longer offered for ${name}.`;
    case 'conflict':
        return `${name} changed before ${label} was sent. Its latest details are shown.`;
    case 'in-flight':
        return `${label} is already being run for ${name}.`;
    case 'chain-conflict':
        return `${name} has already been retried. Open its latest retry instead.`;
    case 'not-found':
        return `${name} is no longer available.`;
    default:
        return withReason(`${label} could not be completed for ${name}`, result.message || payload.error || error?.message);
    }
}

// "<what happened>: <the reason as it was given>", or the first part alone.
function withReason(what, reason) {
    const text = String(reason || '').trim();
    return text ? `${what}: ${text}` : `${what}.`;
}

// The box a command leaves once it answered, naming its Job. A requested
// control waits on the executor, so it says what was asked for; an applied
// one says what the service did.
export function commandNoticeText(job, command, outcome) {
    const label = commandLabel(command);
    const name = jobName(job);
    if (outcome?.code === 'requested') return `${label} requested for ${name}.`;
    const message = String(outcome?.message || '').trim();
    return message ? `${name}: ${message}.`.replace(/\.\.$/, '.') : `${label} completed for ${name}.`;
}

// A requested control is settled once the Job has left the state the reader
// acted on: that is the command's result, or whatever overtook it, at whatever
// version it arrived, the answer's own included. Until then its box says
// "requested"; after, it would say that of something already done.
export function requestSettled(job, answered, now, outcome) {
    if (outcome?.code !== 'requested') return false;
    const latest = latestSnapshot(answered, now);
    return !!latest?.state && requestPassed(requestWatch(job), latest);
}

// The newer of a command's answer and the read after it: a read served before
// the answer was written is older than it, and says nothing about the result.
export function latestSnapshot(answered, now) {
    return now && Number(now.version || 0) >= Number(answered?.version || 0) ? now : answered || now;
}

// What a requested control's box waits for: the Job leaving `job`'s state.
export function requestWatch(job) {
    return { state: stateOf(job) };
}

// Whether `current` (the Job as shown now, or nothing once it is gone) has
// passed what the box was waiting for.
export function requestPassed(watch, current) {
    return !current || stateOf(current) !== watch.state;
}

// Fields a snapshot leaves out when they are empty. A newer snapshot that
// leaves one out says it is empty now, so the value an older one carried must
// not survive the merge: a cancelled Job has no phase, not the "cancelling" it
// had while it stopped.
const SNAPSHOT_OMITTED_WHEN_EMPTY = ['phase', 'failure', 'controlIntent', 'replayAvailability', 'scheduledFor', 'queuedAt', 'startedAt', 'lastResumedAt', 'finishedAt', 'expiresAt'];

export function mergeJobSnapshot(current, incoming) {
    const merged = { ...(current || {}), ...incoming };
    if (current && incoming && Number(incoming.version || 0) > Number(current.version || 0)) {
        for (const key of SNAPSHOT_OMITTED_WHEN_EMPTY) if (!(key in incoming)) delete merged[key];
    }
    return merged;
}
