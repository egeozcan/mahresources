import { createLiveRegion } from '../utils/ariaLiveRegion.js';

export const JOB_STATES = Object.freeze([
    'scheduled', 'queued', 'running', 'paused', 'blocked',
    'succeeded', 'failed', 'cancelled', 'interrupted',
]);
export const JOB_COMMAND_FILTER_ENABLED = true;

const ACTIVE_STATES = ['scheduled', 'queued', 'running', 'paused'];
const ATTENTION_STATES = ['blocked', 'failed', 'interrupted'];
const FINISHED_STATES = ['succeeded', 'cancelled'];
const FILTER_LIST_KEYS = Object.freeze({ kinds: 'kind', states: 'state', origins: 'origin' });
const FILTER_QUERY_KEYS = Object.freeze([
    'search', 'command', 'kind', 'state', 'origin', 'ownerId', 'actorId',
    'acceptedAfter', 'acceptedBefore', 'relationship', 'pinned', 'dismissed',
]);

function readBoolean(params, key) {
    const value = params.get(key);
    if (value === null || value === '') return null;
    if (value === 'true' || value === '1') return true;
    if (value === 'false' || value === '0') return false;
    return null;
}

function readDate(params, key) {
    return params.get(key) || '';
}

export function dateTimeQueryValue(value) {
    if (!value) return '';
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? '' : parsed.toISOString();
}

export function dateTimeLocalValue(value) {
    if (!value) return '';
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return '';
    const localDate = new Date(parsed.getTime() - parsed.getTimezoneOffset() * 60_000);
    return localDate.toISOString().slice(0, 16);
}

export function parseJobCenterURL(input = globalThis.location?.search || '') {
    const params = input instanceof URLSearchParams
        ? input
        : new URLSearchParams(String(input).startsWith('?') ? String(input).slice(1) : String(input));
    return {
        view: params.get('view') === 'all' || params.has('cursor') || FILTER_QUERY_KEYS.some(key => params.has(key)) ? 'all' : 'home',
        filters: {
            search: params.get('search') || '',
            command: params.get('command') || '',
            kinds: params.getAll('kind'),
            states: params.getAll('state'),
            origins: params.getAll('origin'),
            ownerId: params.get('ownerId') || '',
            actorId: params.get('actorId') || '',
            acceptedAfter: readDate(params, 'acceptedAfter'),
            acceptedBefore: readDate(params, 'acceptedBefore'),
            relationship: params.get('relationship') || '',
            pinned: readBoolean(params, 'pinned'),
            dismissed: readBoolean(params, 'dismissed'),
        },
        cursor: params.get('cursor') || null,
    };
}

export function serializeJobCenterURL(state) {
    const params = new URLSearchParams();
    if (state.view === 'all') params.set('view', 'all');
    const filters = state.filters || {};
    if (filters.search) params.set('search', filters.search);
    if (filters.command) params.set('command', filters.command);
    for (const [key, parameter] of Object.entries(FILTER_LIST_KEYS)) {
        for (const value of filters[key] || []) {
            if (value !== '') params.append(parameter, value);
        }
    }
    for (const key of ['ownerId', 'actorId', 'acceptedAfter', 'acceptedBefore', 'relationship']) {
        if (filters[key]) params.set(key, key.startsWith('accepted') ? dateTimeQueryValue(filters[key]) : filters[key]);
    }
    for (const key of ['pinned', 'dismissed']) {
        if (filters[key] !== null && filters[key] !== undefined) params.set(key, String(filters[key]));
    }
    if (state.cursor) params.set('cursor', cursorToken(state.cursor));
    return params.toString();
}

export function buildJobListURL({ filters = {}, states = null, cursor = null, limit = 50 } = {}) {
    const params = new URLSearchParams();
    for (const [key, parameter] of Object.entries(FILTER_LIST_KEYS)) {
        const values = key === 'states' && states ? states : filters[key] || [];
        for (const value of values) {
            if (value !== '') params.append(parameter, value);
        }
    }
    if (filters.search) params.set('search', filters.search);
    if (JOB_COMMAND_FILTER_ENABLED && filters.command) params.set('command', filters.command);
    for (const key of ['ownerId', 'actorId', 'acceptedAfter', 'acceptedBefore', 'relationship']) {
        if (filters[key]) params.set(key, key.startsWith('accepted') ? dateTimeQueryValue(filters[key]) : filters[key]);
    }
    for (const key of ['pinned', 'dismissed']) {
        if (filters[key] !== null && filters[key] !== undefined) params.set(key, String(filters[key]));
    }
    if (cursor) params.set('cursor', cursorToken(cursor));
    params.set('limit', String(limit));
    return `/v1/jobs?${params.toString()}`;
}

export function advertisedCommands(job) {
    return Array.isArray(job?.commands) ? job.commands : [];
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

export function stateOf(job) {
    return String(job?.state || 'unknown').toLowerCase();
}

export function stateLabel(job) {
    const state = stateOf(job);
    if (!state) return 'Unknown';
    return state.charAt(0).toUpperCase() + state.slice(1).replaceAll('-', ' ');
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
    return advertisedCommands(chosenJobs[0])
        .filter(command => command.bulk && commandMaps.every(commands => commands.has(command.key)));
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

export function splitJobSections(jobs) {
    const sections = { attention: [], active: [], finished: [], other: [] };
    for (const job of jobs || []) sections[classifyJobState(job)].push(job);
    return sections;
}

function cursorToken(cursor) {
    if (typeof cursor === 'string') return cursor;
    if (cursor && typeof cursor === 'object') {
        if (typeof cursor.token === 'string') return cursor.token;
        if (typeof cursor.cursor === 'string') return cursor.cursor;
    }
    return String(cursor || '');
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

export function progressText(job) {
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
    const progress = job?.progress || {};
    if (progress.completed === null || progress.completed === undefined || !(progress.total > 0)) return null;
    return Math.max(0, Math.min(100, Math.round((progress.completed / progress.total) * 100)));
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

export function jobCenter(options = {}) {
    return {
        detailId: options.detailId || '',
        view: 'home',
        filters: {},
        commandFilterEnabled: JOB_COMMAND_FILTER_ENABLED,
        jobs: [],
        sections: { attention: [], active: [], finished: [], other: [] },
        summary: null,
        details: {},
        selectedIds: new Set(),
        nextCursor: null,
        _allPageCursors: [],
        _listGeneration: 0,
        bulkOutcomes: [],
        timeline: [],
        timelineError: '',
        detail: null,
        loading: true,
        loadingMore: false,
        error: '',
        notice: '',
        connectionStatus: 'disconnected',
        eventSource: null,
        lastSequence: 0,
        streamCaughtUp: false,
        _liveRegion: null,
        _refreshTimer: null,
        _streamRefreshGeneration: 0,
        _streamRefreshRequested: false,
        _streamRefreshPromise: null,

        init() {
            const state = parseJobCenterURL(globalThis.location?.search || '');
            this.view = state.view;
            this.filters = state.filters;
            this.nextCursor = state.cursor;
            if (!this.detailId && /^\/job(?:\.body)?$/.test(globalThis.location?.pathname || '')) {
                this.detailId = new URLSearchParams(globalThis.location.search).get('id') || '';
            }
            this._liveRegion = createLiveRegion();
            this.connect();
            this.load();
        },

        destroy() {
            if (this._refreshTimer) clearTimeout(this._refreshTimer);
            this._streamRefreshGeneration += 1;
            this._listGeneration += 1;
            this._streamRefreshRequested = false;
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
                if (this.detailId) await this.loadDetail(this.detailId);
                else {
                    await Promise.all([this.loadSummary(), this.view === 'all' || this.hasFilters() ? this.loadAll(this.nextCursor) : this.loadHome()]);
                }
            } catch (error) {
                this.error = error.message || 'Could not load jobs.';
            } finally {
                this.loading = false;
            }
        },

        async loadSummary() {
            this.summary = await this.fetchJSON('/v1/jobs/summary');
        },

        scheduleStreamRefresh() {
            this._streamRefreshGeneration += 1;
            // Invalidate a list request started before this delivery. Its rows
            // may describe a different server-side filter window.
            this._listGeneration += 1;
            this._streamRefreshRequested = true;
            if (this._streamRefreshPromise) return;
            if (this._refreshTimer) clearTimeout(this._refreshTimer);
            const generation = this._streamRefreshGeneration;
            this._refreshTimer = setTimeout(() => {
                this._refreshTimer = null;
                this._streamRefreshRequested = false;
                this._streamRefreshPromise = this.refreshStreamState(generation)
                    .catch(() => {})
                    .finally(() => {
                        this._streamRefreshPromise = null;
                        if (this._streamRefreshRequested) this.scheduleStreamRefresh();
                    });
            }, 150);
        },

        async refreshStreamState(generation) {
            if (this.detailId) return;
            const stateKey = JSON.stringify({ view: this.view, filters: this.filters });
            const listGeneration = this._listGeneration;
            const refreshHome = this.view === 'home' && !this.hasFilters();
            const requests = [this.fetchJSON('/v1/jobs/summary')];
            if (refreshHome) {
                const filterBase = { ...this.filters, dismissed: this.filters.dismissed ?? false };
                requests.push(Promise.all([
                    this.fetchJSON(buildJobListURL({ filters: filterBase, states: ATTENTION_STATES, limit: 6 })),
                    this.fetchJSON(buildJobListURL({ filters: filterBase, states: ACTIVE_STATES, limit: 6 })),
                    this.fetchJSON(buildJobListURL({ filters: filterBase, states: FINISHED_STATES, limit: 6 })),
                ]));
            }
            const [summary, homePages] = await Promise.all(requests);
            let allWindow = null;
            if (!refreshHome) {
                allWindow = await this.fetchLoadedAllWindow(generation, listGeneration, stateKey);
                if (!allWindow) return;
            }
            if (generation !== this._streamRefreshGeneration || listGeneration !== this._listGeneration || stateKey !== JSON.stringify({ view: this.view, filters: this.filters })) return;
            this.summary = summary;
            if (refreshHome && homePages) {
                this.sections = {
                    attention: preserveJobUIState(this.jobs, pageJobs(homePages[0])),
                    active: preserveJobUIState(this.jobs, pageJobs(homePages[1])),
                    finished: preserveJobUIState(this.jobs, pageJobs(homePages[2])),
                    other: [],
                };
                this.jobs = uniqueJobs(Object.values(this.sections).flat());
                this.nextCursor = null;
                this._allPageCursors = [];
            } else if (allWindow) {
                this.jobs = preserveJobUIState(this.jobs, uniqueJobs(allWindow.jobs));
                this.sections = splitJobSections(this.jobs);
                this._allPageCursors = allWindow.cursors;
                this.nextCursor = allWindow.nextCursor;
                if (this.view === 'all') this.replaceURL(allWindow.cursors.at(-1) ?? null);
            }
        },

        async fetchLoadedAllWindow(generation, listGeneration, stateKey) {
            const starts = this._allPageCursors.length ? [...this._allPageCursors] : [null];
            const filterBase = { ...this.filters, dismissed: this.filters.dismissed ?? false };
            const jobs = [];
            const cursors = [];
            let cursor = starts[0];
            let nextCursor = null;
            for (let page = 0; page < starts.length; page++) {
                if (page > 0 && !cursor) break;
                const pageCursor = cursor;
                const payload = await this.fetchJSON(buildJobListURL({ filters: filterBase, cursor: pageCursor, limit: 50 }));
                if (generation !== this._streamRefreshGeneration || listGeneration !== this._listGeneration || stateKey !== JSON.stringify({ view: this.view, filters: this.filters })) return null;
                jobs.push(...pageJobs(payload));
                cursors.push(pageCursor);
                nextCursor = payload.nextCursor || null;
                cursor = nextCursor;
            }
            return { jobs, cursors, nextCursor };
        },

        hasFilters() {
            return !!(
                this.filters.search || this.filters.kinds?.length || this.filters.states?.length || this.filters.origins?.length ||
                this.filters.command ||
                this.filters.ownerId || this.filters.actorId || this.filters.acceptedAfter || this.filters.acceptedBefore ||
                this.filters.relationship ||
                (this.filters.pinned !== null && this.filters.pinned !== undefined) ||
                (this.filters.dismissed !== null && this.filters.dismissed !== undefined)
            );
        },

        async loadHome() {
            const listGeneration = ++this._listGeneration;
            if (this._streamRefreshPromise) this._streamRefreshRequested = true;
            const filterBase = { ...this.filters, dismissed: this.filters.dismissed ?? false };
            const [attention, active, finished] = await Promise.all([
                this.fetchJSON(buildJobListURL({ filters: filterBase, states: ATTENTION_STATES, limit: 6 })),
                this.fetchJSON(buildJobListURL({ filters: filterBase, states: ACTIVE_STATES, limit: 6 })),
                this.fetchJSON(buildJobListURL({ filters: filterBase, states: FINISHED_STATES, limit: 6 })),
            ]);
            if (listGeneration !== this._listGeneration) return;
            const sections = {
                attention: preserveJobUIState(this.jobs, pageJobs(attention)),
                active: preserveJobUIState(this.jobs, pageJobs(active)),
                finished: preserveJobUIState(this.jobs, pageJobs(finished)),
                other: [],
            };
            this.sections = sections;
            this.jobs = uniqueJobs(Object.values(sections).flat());
            this.nextCursor = null;
            this._allPageCursors = [];
        },

        async loadAll(cursor = null, append = false) {
            const listGeneration = ++this._listGeneration;
            if (this._streamRefreshPromise) this._streamRefreshRequested = true;
            if (!append) this._allPageCursors = [cursor];
            const payload = await this.fetchJSON(buildJobListURL({
                filters: { ...this.filters, dismissed: this.filters.dismissed ?? false },
                cursor,
                limit: 50,
            }));
            if (listGeneration !== this._listGeneration) return false;
            const rows = preserveJobUIState(this.jobs, pageJobs(payload));
            this.jobs = append ? uniqueJobs([...this.jobs, ...rows]) : rows;
            this._allPageCursors = append
                ? [...this._allPageCursors, cursor]
                : [cursor];
            this.nextCursor = payload.nextCursor || null;
            this.sections = splitJobSections(this.jobs);
            return true;
        },

        async loadMore() {
            if (this.loadingMore || !this.nextCursor) return;
            const previous = this.nextCursor;
            this.loadingMore = true;
            this.error = '';
            try {
                if (await this.loadAll(previous, true)) this.replaceURL(previous);
            } catch (error) {
                this.error = error.message || 'Could not load the next page.';
            } finally {
                this.loadingMore = false;
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

        async detailFor(job) {
            if (advertisedCommands(job).length || advertisedOutputs(job).length || this.details[job.id]) {
                return this.details[job.id] || job;
            }
            const payload = await this.fetchJSON(`/v1/jobs/${encodeURIComponent(job.id)}`);
            const detail = payload.job || payload;
            this.details[job.id] = detail;
            this.updateJob(detail);
            return detail;
        },

        async toggleSelection(job, checked) {
            const selected = new Set(this.selectedIds);
            if (checked) selected.add(job.id);
            else selected.delete(job.id);
            this.selectedIds = selected;
            if (checked) {
                try { await this.detailFor(job); }
                catch (error) {
                    this.notice = error.message || 'Could not read the selected job commands.';
                    this._liveRegion?.announce(this.notice);
                }
            }
        },

        selectedJobs() {
            return [...Object.values(this.details), ...this.jobs].filter((job, index, rows) =>
                this.selectedIds.has(job.id) && rows.findIndex(candidate => candidate.id === job.id) === index,
            );
        },

        bulkCommands() {
            return selectedBulkCommands(this.selectedJobs(), this.selectedIds);
        },

        replaceURL(cursor = this.view === 'all' ? this.nextCursor : null) {
            const serialized = serializeJobCenterURL({ view: this.view, filters: this.filters, cursor });
            const nextURL = `${globalThis.location?.pathname || '/jobs'}${serialized ? `?${serialized}` : ''}`;
            globalThis.history?.replaceState({}, '', nextURL);
        },

        submitFilters(form) {
            const data = new FormData(form);
            const csv = key => String(data.get(key) || '').split(',').map(value => value.trim()).filter(Boolean);
            this.filters = {
                search: String(data.get('search') || '').trim(),
                command: String(data.get('command') || '').trim(),
                kinds: csv('kind'),
                states: csv('state'),
                origins: csv('origin'),
                ownerId: String(data.get('ownerId') || '').trim(),
                actorId: String(data.get('actorId') || '').trim(),
                acceptedAfter: String(data.get('acceptedAfter') || '').trim(),
                acceptedBefore: String(data.get('acceptedBefore') || '').trim(),
                relationship: String(data.get('relationship') || '').trim(),
                pinned: parseSelectBoolean(data.get('pinned')),
                dismissed: parseSelectBoolean(data.get('dismissed')),
            };
            this.view = 'all';
            this.nextCursor = null;
            this._allPageCursors = [];
            this.selectedIds = new Set();
            this.replaceURL(null);
            this.loading = true;
            this.error = '';
            return this.loadAll().catch(error => {
                this.error = error.message || 'Could not apply these filters.';
            }).finally(() => { this.loading = false; });
        },

        clearFilters() {
            this.filters = emptyFilters();
            this.view = 'home';
            this.nextCursor = null;
            this._allPageCursors = [];
            this.selectedIds = new Set();
            globalThis.history?.replaceState({}, '', '/jobs');
            return this.load();
        },

        async setView(view) {
            this.view = view === 'all' ? 'all' : 'home';
            this.nextCursor = null;
            this._allPageCursors = [];
            this.replaceURL(null);
            this.loading = true;
            this.error = '';
            try {
                return await (this.view === 'all' ? this.loadAll() : this.loadHome());
            } catch (error) {
                this.error = error.message || 'Could not change job views.';
                return null;
            } finally {
                this.loading = false;
            }
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
                const successorId = outcome.successorId || outcome.successorID || payload.successorId || payload.successorID;
                if (successorId) globalThis.location?.assign?.(`/job?id=${encodeURIComponent(successorId)}`);
                this.notice = outcome.message || payload.message || `${commandLabel(command)} requested.`;
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

        async runBulkCommand(command) {
            const jobs = this.selectedJobs();
            if (!jobs.length || this.bulkBusy) return [];
            const confirmation = commandConfirmation(command);
            if (confirmation) {
                const accepted = await globalThis.Alpine?.store('confirmDialog')?.ask(
                    `${confirmation} This applies to ${jobs.length} selected jobs.`,
                    { title: commandLabel(command), confirmLabel: commandLabel(command) },
                );
                if (!accepted) return [];
            }
            const key = idempotencyKey();
            this.bulkBusy = true;
            this.bulkOutcomes = [];
            try {
                const payload = await this.fetchJSON(`/v1/jobs/commands/${encodeURIComponent(command.key)}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
                    body: JSON.stringify({ jobIds: jobs.map(job => job.id), idempotencyKey: key }),
                });
                this.bulkOutcomes = payload.results || payload.outcomes || [];
                const succeeded = this.bulkOutcomes.filter(outcome => outcome.status === 'succeeded' || outcome.code === 'applied').length;
                this.notice = `${succeeded} of ${jobs.length} jobs ${commandLabel(command).toLowerCase()}.`;
                this._liveRegion?.announce(this.notice);
                await this.refreshCurrentView();
                return this.bulkOutcomes;
            } catch (error) {
                this.bulkOutcomes = error.payload?.results || error.payload?.outcomes || [];
                this.notice = error.message || 'The bulk command could not be completed.';
                this._liveRegion?.announce(this.notice);
                return this.bulkOutcomes;
            } finally {
                this.bulkBusy = false;
            }
        },

        async refreshCurrentView() {
            return Promise.all([
                this.loadSummary().catch(() => {}),
                this.view === 'all' || this.hasFilters() ? this.loadAll() : this.loadHome(),
            ]);
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
            const previousSequence = this.lastSequence;
            const result = reduceJobStreamEvent(this.jobs, message, this.lastSequence);
            this.lastSequence = result.lastSequence;
            if (this.lastSequence > previousSequence) this.scheduleStreamRefresh();
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
            if (this.view === 'home' && !this.hasFilters()) {
                this.sections = splitJobSections(this.jobs);
            } else if (this.view === 'all') {
                this.sections = splitJobSections(this.jobs);
            }
            if (result.announcement) this._liveRegion?.announce(result.announcement);
        },

        updateJob(job) {
            const result = reduceJobStreamEvent(this.jobs, { job }, this.lastSequence);
            this.applyStreamSnapshot(job, result);
        },

        applyStreamSnapshot(job, previousResult = null, announce = false) {
            if (!job?.id) return;
            const result = previousResult || reduceJobStreamEvent(this.jobs, { job }, this.lastSequence);
            if (result.changed) {
                this.jobs = result.jobs;
                this.sections = splitJobSections(this.jobs);
            }
            this.details[job.id] = { ...(this.details[job.id] || {}), ...job };
            if (this.detail?.id === job.id) this.detail = { ...this.detail, ...job };
            if (announce && result.announcement) this._liveRegion?.announce(result.announcement);
        },

        toggleExpanded(job) {
            job.uiExpanded = !job.uiExpanded;
        },

        progressText(job) { return progressText(job); },
        progressValue(job) { return progressValue(job); },
        progressAccessibleText(job) { return progressAccessibleText(job); },
        dateTimeLocalValue(value) { return dateTimeLocalValue(value); },
        stateLabel(job) { return stateLabel(job); },
        stateClass(job) { return classifyJobState(job); },
        commandLabel(command) { return commandLabel(command); },
        advertisedCommands(job) { return advertisedCommands(this.details[job.id] || job); },
        advertisedOutputs(job) { return advertisedOutputs(job); },
        warningEvents() { return warningEvents(this.timeline); },
        outputEndpoint(output) { return outputEndpoint(output); },
        get bulkBusy() { return this._bulkBusy || false; },
        set bulkBusy(value) { this._bulkBusy = value; },

        get isAllView() { return this.view === 'all'; },
        get selectedCount() { return this.selectedIds.size; },
        get displaySections() {
            if (this.view === 'all' || this.hasFilters()) return [{ key: 'all', title: 'All jobs', jobs: this.jobs }];
            return [
                { key: 'attention', title: 'Needs attention', jobs: this.sections.attention },
                { key: 'active', title: 'Active and scheduled', jobs: this.sections.active },
                { key: 'finished', title: 'Recent finished', jobs: this.sections.finished },
            ];
        },
    };
}


function emptyFilters() {
    return { search: '', command: '', kinds: [], states: [], origins: [], ownerId: '', actorId: '', acceptedAfter: '', acceptedBefore: '', relationship: '', pinned: null, dismissed: null };
}

function parseSelectBoolean(value) {
    if (value === 'true') return true;
    if (value === 'false') return false;
    return null;
}

function pageJobs(payload) {
    if (Array.isArray(payload)) return payload;
    return Array.isArray(payload?.jobs) ? payload.jobs : [];
}

function uniqueJobs(jobs) {
    const seen = new Map();
    for (const job of jobs) {
        if (!seen.has(job.id)) seen.set(job.id, job);
    }
    return [...seen.values()].sort((left, right) => String(right.acceptedAt || '').localeCompare(String(left.acceptedAt || '')) || String(right.id).localeCompare(String(left.id)));
}

function preserveJobUIState(previousJobs, incomingJobs) {
    const previousById = new Map((previousJobs || []).map(job => [job.id, job]));
    return (incomingJobs || []).map(job => {
        const previous = previousById.get(job.id);
        if (!previous) return job;
        return {
            ...job,
            uiExpanded: previous.uiExpanded ?? job.uiExpanded ?? false,
            uiSelected: previous.uiSelected ?? job.uiSelected ?? false,
        };
    });
}

function commandConfirmation(command) {
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
    if (command?.destructive) return `Run ${commandLabel(command)}?`;
    return '';
}
