import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { captureTrigger, focusedElement, focusFirstIn, restoreFocus } from '../utils/focus.js';

export function downloadCockpit() {
    return {
        isOpen: false,
        jobs: [],
        retainedCompletedJobs: [],  // Completed jobs retained for display after backend removal
        eventSource: null,
        connectionStatus: 'disconnected', // 'connected', 'disconnected', 'connecting'
        _liveRegion: null,
        _keydownHandler: null,
        _reconnectDelay: 1000,  // Initial reconnect delay (ms)
        _reconnectAttempts: 0,
        _maxReconnectAttempts: 10,
        _maxReconnectDelay: 60000,  // Max 60 seconds
        speedTracking: {},  // Track progress for speed calculation: { jobId: { lastProgress, lastTime, speed } }
        // Finding 40: ids the user cleared. Consulted by the `removed` handler, whose
        // job is otherwise to retain finished jobs for display — it would put every
        // cleared row straight back, because the removal events arrive after the
        // clear request resolves.
        _dismissedIds: new Set(),
        // BH-036: retention window (ms). Read from the meta tag emitted by base.tpl.
        exportRetentionMs: 0,

        statusIcons: {
            pending: '\u23F3',      // Hourglass
            downloading: '\u2B07',  // Down arrow
            processing: '\u2699',   // Gear
            running: '\u2699',      // Gear (same as processing)
            completed: '\u2705',    // Check mark
            failed: '\u274C',       // X mark
            cancelled: '\u26D4',    // No entry
            paused: '\u23F8'        // Pause symbol
        },

        statusLabels: {
            pending: 'Pending',
            downloading: 'Downloading',
            processing: 'Processing',
            running: 'Running',
            completed: 'Completed',
            failed: 'Failed',
            cancelled: 'Cancelled',
            paused: 'Paused'
        },

        init() {
            this._liveRegion = createLiveRegion();
            this._lastTrigger = null;
            // The floor for focus restore, captured here because $el in init() is the
            // component root — in a method it would be whichever element called it.
            // The trigger always exists (it is header chrome, not inside an x-if), so
            // it is somewhere real to land whenever there was no trigger to capture.
            this._trigger = this.$el.querySelector('.cockpit-trigger');
            // Captured for the same reason: blockingModal() sweeps the whole document
            // for open dialogs, and this panel is one of them.
            this._root = this.$el;

            // BH-036: pick up the retention window from the meta tag emitted by base.tpl.
            const metaEl = document.querySelector('meta[name="x-export-retention-ms"]');
            if (metaEl) {
                this.exportRetentionMs = parseInt(metaEl.getAttribute('content'), 10) || 0;
            }

            // Listen for jobs-panel-open event (e.g., from pluginActionModal)
            window.addEventListener('jobs-panel-open', (e) => this.openFromEvent(e.detail));

            // Listen for keyboard shortcut: Cmd/Ctrl+Shift+D
            this._keydownHandler = (e) => {
                if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'd') {
                    e.preventDefault();
                    this.toggle();
                }
            };
            document.addEventListener('keydown', this._keydownHandler);

            // Always connect to SSE to track download completions
            this.connect();

            // BH-028: Focus trap — move focus into panel on open, restore on close.
            this.$watch('isOpen', (value) => {
                if (value) {
                    this.announce('Jobs panel opened. Shows background job progress.');
                    this.$nextTick(() => focusFirstIn(this.$refs.panel));
                } else {
                    // Unconditionally, which is review remediation finding 4: this was
                    // gated on _lastTrigger, and the panel opens three ways that leave
                    // it null — the Cmd/Ctrl+Shift+D shortcut (toggle() with no
                    // event), the `jobs-panel-open` window event, and an incoming
                    // plugin action job. The panel is x-trap.noreturn and its contents
                    // are torn down by the x-if, so nothing else was standing between
                    // the reader and <body>.
                    restoreFocus(this._lastTrigger, this._trigger);
                    this._lastTrigger = null;
                }
            });
        },

        announce(message) {
            this._liveRegion?.announce(message);
        },

        destroy() {
            this._liveRegion?.destroy();
            if (this._keydownHandler) {
                document.removeEventListener('keydown', this._keydownHandler);
            }
            this.disconnect();
        },

        /**
         * The dialog, if any, that is already open when the panel is asked to open.
         *
         * The panel is header chrome: `.header` is a stacking context at z-index 40,
         * so the panel's own z-[60] only orders it against its *header* siblings. The
         * app's true modals live in `.overlays` at z-index 41 in the root stacking
         * context, above the whole header layer, so the panel opens behind one, moves
         * focus into itself and x-trap holds it there — inside a dialog nobody can
         * see, with Escape going to whichever component hears it first.
         *
         * The query is every `aria-modal` in the document and not just `.overlays`.
         * Scoping it to that layer was wrong twice over: the global search dialog is a
         * *header sibling* of this panel, so two dialogs at the same z-index fight on
         * DOM order alone, and six more live elsewhere (mrql.tpl, json.tpl, menu.tpl,
         * blockEditor.tpl, schemaEditorModal.tpl, globalSearch.tpl). A guard that
         * covers four of ten dialogs recreates the defect it exists to prevent, for
         * the other six.
         *
         * Two aria-modal dialogs open at once is the defect whichever way it paints,
         * so the panel declines rather than fighting for the top.
         */
        blockingModal() {
            for (const el of document.querySelectorAll('[aria-modal="true"]')) {
                // Never this panel: on the paths that can be reached while it is open,
                // finding itself would make it refuse to do anything.
                if (this._root?.contains?.(el)) continue;
                if (this.isRendered(el)) return el;
            }
            return null;
        },

        /**
         * Whether an element is actually painted. Three of the four overlays use
         * x-show, so they stay in the document with `display: none` and querying for
         * them is not enough on its own.
         */
        isRendered(el) {
            if (!el || !el.isConnected) return false;
            if (typeof el.checkVisibility === 'function') return el.checkVisibility();
            return !!(el.offsetWidth || el.offsetHeight || el.getClientRects?.().length);
        },

        /**
         * Open the panel from something that is not the trigger: the
         * `jobs-panel-open` window event, or a plugin action job arriving over SSE.
         *
         * These set isOpen directly, so they used to leave nothing to return focus to
         * and the reader came back to the panel's trigger instead of the control they
         * were actually on. Where they were is knowable — read it before the panel
         * mounts and moves focus inside.
         *
         * `returnFocusTo` is for the one caller that knows better than
         * `focusedElement()` can: pluginActionModal closes itself and then asks for
         * the panel, so what has focus at that moment is the modal's own Run button,
         * which is about to be torn down by its x-if. Restoring to a detached node
         * lands on <body>, so the fallback took over and the reader was returned to
         * the header trigger rather than to the action button they pressed.
         */
        openFromEvent(detail = null) {
            if (!this.isOpen) {
                if (this.blockingModal()) {
                    this.announce('A dialog is open, so the jobs panel was not opened. Close the dialog, then press Control or Command, Shift and D.');
                    return;
                }
                const requested = detail?.returnFocusTo;
                this._lastTrigger = (this.isRendered(requested) ? requested : null) ?? focusedElement() ?? this._trigger;
            }
            this.isOpen = true;
        },

        toggle(event) {
            if (!this.isOpen && this.blockingModal()) {
                this.announce('A dialog is open, so the jobs panel was not opened. Close the dialog first.');
                return;
            }
            if (!this.isOpen) {
                // Read before the flip, because opening moves focus into the panel.
                // The keyboard shortcut calls this with no event, so there is no
                // currentTarget to return to — whatever had focus when it was pressed
                // is the honest answer, and the trigger is the floor when focus was
                // nowhere (review remediation finding 4). The floor is the trigger and
                // not a trigger captured on some earlier open: that one may be long
                // gone from the page, while this is header chrome that always exists.
                this._lastTrigger = captureTrigger(event) ?? focusedElement() ?? this._trigger;
            }
            this.isOpen = !this.isOpen;
        },

        close() {
            this.isOpen = false;
        },

        connect() {
            if (this.eventSource) return;

            this.connectionStatus = 'connecting';
            this.eventSource = new EventSource('/v1/jobs/events');

            this.eventSource.addEventListener('init', (e) => {
                const data = JSON.parse(e.data);
                this.jobs = data.jobs || [];
                // Load action jobs into the same array with source marker
                const actionJobs = (data.actionJobs || []).map(j => ({ ...j, _isAction: true }));
                this.jobs = [...this.jobs, ...actionJobs];
                this.retainedCompletedJobs = [];  // Clear retained on reconnect
                this.connectionStatus = 'connected';
                // Reset backoff on successful connection
                this._reconnectDelay = 1000;
                this._reconnectAttempts = 0;
            });

            this.eventSource.addEventListener('added', (e) => {
                const { job } = JSON.parse(e.data);
                if (!this.upsertJob(job)) return;
                this.announce(`Download queued: ${this.truncateUrl(job.url, 30)}`);
            });

            this.eventSource.addEventListener('updated', (e) => {
                const { job } = JSON.parse(e.data);
                const index = this.jobs.findIndex(j => j.id === job.id);
                if (index !== -1) {
                    this.jobs[index] = job;
                }

                // Calculate download speed
                if (job.status === 'downloading' && job.progress > 0) {
                    const now = Date.now();
                    const tracking = this.speedTracking[job.id];
                    if (tracking && tracking.lastProgress < job.progress) {
                        const timeDelta = (now - tracking.lastTime) / 1000; // seconds
                        const bytesDelta = job.progress - tracking.lastProgress;
                        if (timeDelta > 0) {
                            tracking.speed = bytesDelta / timeDelta;
                        }
                        tracking.lastProgress = job.progress;
                        tracking.lastTime = now;
                    } else if (!tracking) {
                        this.speedTracking[job.id] = {
                            lastProgress: job.progress,
                            lastTime: now,
                            speed: 0
                        };
                    }
                }

                if (job.status === 'completed') {
                    delete this.speedTracking[job.id];
                    this.announce(`Download completed: ${this.truncateUrl(job.url, 30)}`);
                    // Dispatch global event for resource lists to reload
                    window.dispatchEvent(new CustomEvent('download-completed', { detail: job }));
                } else if (job.status === 'failed') {
                    delete this.speedTracking[job.id];
                    this.announce(`Download failed: ${this.truncateUrl(job.url, 30)}`);
                } else if (job.status === 'paused') {
                    delete this.speedTracking[job.id];
                    this.announce(`Download paused: ${this.truncateUrl(job.url, 30)}`);
                }
            });

            this.eventSource.addEventListener('removed', (e) => {
                const { job } = JSON.parse(e.data);
                this.handleJobRemoved(job);
            });

            this.eventSource.addEventListener('action_added', (e) => {
                const { job } = JSON.parse(e.data);
                job._isAction = true;
                const isNew = this.upsertJob(job);
                this.openFromEvent();
                if (isNew) this.announce(`Action started: ${job.label}`);
            });

            this.eventSource.addEventListener('action_updated', (e) => {
                const { job } = JSON.parse(e.data);
                job._isAction = true;
                const index = this.jobs.findIndex(j => j.id === job.id);
                if (index !== -1) {
                    this.jobs[index] = job;
                }
                if (job.status === 'completed') {
                    this.announce(`Action completed: ${job.label}`);
                    window.dispatchEvent(new CustomEvent('plugin-action-completed', { detail: job }));
                } else if (job.status === 'failed') {
                    this.announce(`Action failed: ${job.label}`);
                }
            });

            this.eventSource.addEventListener('action_removed', (e) => {
                const { job } = JSON.parse(e.data);
                this.handleJobRemoved(job, { isAction: true });
            });

            this.eventSource.onerror = () => {
                this.connectionStatus = 'disconnected';
                this.disconnect();
                this._reconnectAttempts++;
                if (this._reconnectAttempts <= this._maxReconnectAttempts) {
                    setTimeout(() => {
                        this.connect();
                    }, this._reconnectDelay);
                    // Exponential backoff: double delay each attempt, capped at max
                    this._reconnectDelay = Math.min(this._reconnectDelay * 2, this._maxReconnectDelay);
                }
            };
        },

        disconnect() {
            if (this.eventSource) {
                this.eventSource.close();
                this.eventSource = null;
            }
            this.connectionStatus = 'disconnected';
        },

        /**
         * Add a job the stream has just announced, or fold it into the row that is
         * already there. Returns whether the row is new, so an announcement is only
         * made for a job the reader has not been told about.
         *
         * Push alone produced two rows for one job. The SSE handler subscribes first
         * and *then* lists the queue — correct, or a job created between the two would
         * be missed entirely — so a job submitted in that window appears both in the
         * buffered `added` event and in the `init` listing. The second row was never
         * updated afterwards, because `updated` resolves a job by the first id it
         * matches, so it sat at "Pending" for the rest of the session with live
         * controls on it.
         */
        upsertJob(job) {
            const index = this.jobs.findIndex(j => j.id === job.id);
            if (index !== -1) {
                this.jobs[index] = { ...this.jobs[index], ...job };
                return false;
            }
            this.jobs.push(job);
            return true;
        },

        /**
         * Ask the server to change a job's state, and say so when it refuses.
         *
         * All four controls were `fetch(...).catch(console.error)`, and fetch does not
         * reject on 4xx — so a refusal was discarded in silence. Every refusal these
         * endpoints produce is one the reader can provoke: the row's state is a
         * snapshot from the last SSE event, so pressing Cancel on a download that
         * finished a moment ago answers 409, and pressing anything on a row the
         * retention sweep has removed answers 404. Nothing moved and nothing was said,
         * which for a screen-reader user is indistinguishable from a dead button.
         */
        async control(path, jobId, refusal) {
            try {
                const res = await fetch(path, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded',
                        'Accept': 'application/json',
                    },
                    body: `id=${encodeURIComponent(jobId)}`
                });
                if (res.ok) return true;
                const payload = await res.json().catch(() => null);
                this.announce(payload?.error || refusal);
                return false;
            } catch (err) {
                console.error(`${path} failed:`, err);
                this.announce(refusal);
                return false;
            }
        },

        cancelJob(jobId) {
            return this.control('/v1/jobs/cancel', jobId, 'That job could not be cancelled.');
        },

        /**
         * Dismiss every finished job (finding 40).
         *
         * The server removes them, which is what makes this durable: a purely local
         * hide would be undone by the next `init` event, since the SSE stream
         * replays the whole queue on connect.
         *
         * The ids are remembered in `_dismissedIds` because the `removed` events
         * the clear provokes arrive *after* this resolves, and the `removed`
         * handler's job is to retain finished jobs for display — it would put every
         * row straight back. Nothing is dismissed unless the server said it went,
         * so a rejected clear leaves the panel honest.
         *
         * The dismissed ids come from the *response*, not from the snapshot taken
         * before the request (review remediation finding 2). Which rows are finished
         * is decided server-side at handling time, so a job that crossed into a
         * terminal state while the request was in flight was cleared there, was
         * missing from this set, and came back as a phantom finished row that
         * survived until the next reconnect.
         */
        async clearCompleted() {
            const snapshot = this.displayJobs.filter(j => this.isFinished(j)).map(j => j.id);
            if (snapshot.length === 0) return;

            let cleared = null;
            try {
                const res = await fetch('/v1/jobs/clearCompleted', { method: 'POST' });
                if (!res.ok) {
                    this.announce('Could not clear finished jobs.');
                    return;
                }
                const payload = await res.json().catch(() => null);
                cleared = Array.isArray(payload?.ids) ? payload.ids : null;
            } catch (err) {
                console.error('Clear completed failed:', err);
                this.announce('Could not clear finished jobs.');
                return;
            }

            // A 2xx means the server did clear them, so an unreadable body must not
            // leave the panel showing rows that are gone; the snapshot is the best
            // guess left in that case.
            const dismissed = cleared ?? snapshot;
            dismissed.forEach(id => this._dismissedIds.add(id));
            this.jobs = this.jobs.filter(j => !this._dismissedIds.has(j.id));
            this.retainedCompletedJobs = this.retainedCompletedJobs.filter(j => !this._dismissedIds.has(j.id));
            this.announce(`Cleared ${dismissed.length} finished job${dismissed.length === 1 ? '' : 's'}.`);
        },

        /**
         * A job the server removed: keep it on screen if it finished, drop it if the
         * user dismissed it.
         *
         * Jobs leave the queue for three reasons — eviction, the retention sweep,
         * and "Clear completed" — and only the last is the user asking. So a finished
         * row is retained for display unless its id is in `_dismissedIds`.
         *
         * One method for both `removed` and `action_removed` because the dismissal
         * check was missing from the action branch entirely: clearing a finished
         * plugin action row left it on screen (review remediation finding 2).
         *
         * "Finished" and not "not active", which is what the code said while this
         * comment said the other thing. `paused` is deliberately neither, so a paused
         * download the 24-hour retention sweep removed was kept on screen as a row
         * whose Resume and Cancel buttons addressed a job the server no longer had —
         * and both answered 404 in silence until the controls learned to say so.
         */
        handleJobRemoved(job, { isAction = false } = {}) {
            const existingJob = this.jobs.find(j => j.id === job.id);
            // The removal event is the server's last word about this job, and it
            // replaces the local row rather than being merged into it. Deciding from
            // the local copy alone assumes the panel saw every update, and it does
            // not: notifySubscribers drops an event rather than blocking on a slow
            // subscriber, so a missed terminal `updated` left the row reading
            // "downloading" — not finished, therefore not retained — and the
            // completion, its warnings and an export's download link vanished at the
            // moment they became useful.
            //
            // Replaces rather than merges, because a snapshot's *absent* fields are
            // meaningful: `error`, `warnings`, `resultPath` and the rest are
            // `omitempty`, so a job whose retry cleared its error simply has no
            // `error` key. Spreading the event over the old row would keep the failed
            // attempt's message and show "Completed" beside "boom".
            const authoritative = job && job.status !== undefined;
            const finalJob = authoritative ? job : existingJob;

            if (existingJob && this.isFinished(finalJob) && !this._dismissedIds.has(job.id)) {
                if (isAction) finalJob._isAction = true;
                // Avoid duplicates
                if (!this.retainedCompletedJobs.some(j => j.id === finalJob.id)) {
                    this.retainedCompletedJobs.unshift(finalJob);
                    // Keep only the last 5
                    if (this.retainedCompletedJobs.length > 5) {
                        this.retainedCompletedJobs = this.retainedCompletedJobs.slice(0, 5);
                    }
                }
            }

            this.jobs = this.jobs.filter(j => j.id !== job.id);
        },

        pauseJob(jobId) {
            return this.control('/v1/jobs/pause', jobId, 'That job could not be paused.');
        },

        resumeJob(jobId) {
            return this.control('/v1/jobs/resume', jobId, 'That job could not be resumed.');
        },

        retryJob(jobId) {
            return this.control('/v1/jobs/retry', jobId, 'That job could not be retried.');
        },

        formatProgress(job) {
            if (job.totalSize > 0) {
                const downloaded = this.formatBytes(job.progress);
                const total = this.formatBytes(job.totalSize);
                // BH-015: cap label at 100 — totalSize estimate sometimes understates
                // tar overhead so raw progressPercent can overshoot.
                const percent = Math.min(100, job.progressPercent).toFixed(1);
                return `${downloaded} / ${total} (${percent}%)`;
            } else if (job.progress > 0) {
                return `${this.formatBytes(job.progress)} downloaded`;
            }
            return '';
        },

        formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
        },

        getSpeed(job) {
            const tracking = this.speedTracking[job.id];
            return tracking ? tracking.speed : 0;
        },

        formatSpeed(job) {
            const speed = this.getSpeed(job);
            if (speed <= 0) return '';
            return this.formatBytes(speed) + '/s';
        },

        // BH-036: human-readable relative time for a future timestamp (ms epoch).
        // Used to render the "Expires in X" label on completed group-export rows.
        formatRelativeTime(epochMs) {
            const now = Date.now();
            const diff = epochMs - now;
            if (diff <= 0) return 'now (tar may already be gone)';
            const mins = Math.floor(diff / 60_000);
            if (mins < 60) return `in ${mins} min`;
            const hours = Math.floor(mins / 60);
            if (hours < 24) return `in ${hours} h ${mins % 60} min`;
            const days = Math.floor(hours / 24);
            return `in ${days} day${days !== 1 ? 's' : ''}`;
        },

        getProgressPercent(job) {
            if (job.totalSize > 0 && job.progress > 0) {
                return Math.min(100, (job.progress / job.totalSize) * 100);
            }
            return 0;
        },

        /**
         * The progress bar's accessible name (finding 113).
         *
         * It used to be `'Download progress: ' + formatProgress(job)`, and
         * formatProgress returns `''` whenever the total size is unknown and nothing
         * has arrived yet — which is exactly the state a fresh remote download is in
         * (`totalSize: -1`). A screen reader announced the bare prefix
         * "Download progress:" with nothing after the colon.
         *
         * Named after the job, so several bars in one panel are told apart.
         */
        progressLabel(job) {
            const name = this.getJobTitle(job) || 'download';
            const detail = this.formatProgress(job);
            if (job.status === 'paused') {
                return detail
                    ? `Download progress: ${name}, paused at ${detail}`
                    : `Download progress: ${name}, paused`;
            }
            return detail
                ? `Download progress: ${name}, ${detail}`
                : `Download progress: ${name}, size unknown`;
        },

        /**
         * aria-valuenow, or null for an indeterminate bar.
         *
         * ARIA says an indeterminate progressbar omits aria-valuenow; it used to be
         * pinned at 0 for the whole transfer of an unknown-size download, which
         * announces "0 percent" over and over while bytes are arriving. Alpine
         * removes an attribute bound to null, the same idiom autocompleter.tpl uses
         * for aria-invalid.
         */
        progressValueNow(job) {
            if (!(job.totalSize > 0)) return null;
            return Math.round(this.getProgressPercent(job));
        },

        /** Describes an indeterminate bar, where a percentage would be a lie. */
        progressValueText(job) {
            if (job.totalSize > 0) return null;
            return this.formatProgress(job) || 'Waiting for the first bytes';
        },

        isActive(job) {
            return ['pending', 'downloading', 'processing', 'running'].includes(job.status);
        },

        /**
         * Finished means terminal: nothing more will happen to this job.
         *
         * `paused` is deliberately neither active nor finished — that distinction is
         * the whole of finding 2. It keeps its Cancel button and it survives
         * "Clear completed".
         */
        isFinished(job) {
            return ['completed', 'failed', 'cancelled'].includes(job.status);
        },

        canPause(job) {
            return ['pending', 'downloading'].includes(job.status);
        },

        /**
         * Finding 2: the Cancel button was gated on isActive(), which excludes
         * `paused`, so a paused download offered only Resume — and the API refused
         * a cancel with 404 "already finished" even if you found it. Both halves are
         * fixed; this is the UI half.
         */
        canCancel(job) {
            return this.isActive(job) || job.status === 'paused';
        },

        /**
         * Whether the row shows a progress readout. Finding 41: this was
         * `status === 'downloading'` in the template, so pausing threw away the
         * bytes, the percentage and the bar — the server keeps all three (measured:
         * a paused job still reports progress 196608 of 52428800), only the panel
         * stopped rendering them.
         */
        showsProgress(job) {
            return job.status === 'downloading' || job.status === 'paused';
        },

        canResume(job) {
            return job.status === 'paused';
        },

        canRetry(job) {
            return ['failed', 'cancelled'].includes(job.status);
        },

        get activeCount() {
            return this.jobs.filter(j => this.isActive(j)).length;
        },

        get hasActiveJobs() {
            return this.activeCount > 0;
        },

        /**
         * Newest first (finding 40).
         *
         * The panel rendered insertion order and opened scrolled to the top, so a
         * download you had just started was the *last* row — measured ~1340px below
         * the fold, with 20 older jobs above it and no hint that an active one
         * existed further down.
         *
         * Sorted on createdAt, which both job kinds carry (DownloadJob.createdAt and
         * ActionJob.createdAt), with the id as the tie-breaker so the order is
         * stable rather than dependent on Array#sort's treatment of equal keys —
         * jobs submitted in one SubmitMultiple call share a timestamp to the
         * microsecond.
         */
        get displayJobs() {
            const jobIds = new Set(this.jobs.map(j => j.id));
            const unique = this.retainedCompletedJobs.filter(j => !jobIds.has(j.id));
            const all = [...this.jobs, ...unique].filter(j => !this._dismissedIds.has(j.id));
            return all.sort((a, b) => {
                const at = Date.parse(a.createdAt) || 0;
                const bt = Date.parse(b.createdAt) || 0;
                if (at !== bt) return bt - at;
                return String(b.id).localeCompare(String(a.id));
            });
        },

        get hasFinishedJobs() {
            return this.displayJobs.some(j => this.isFinished(j));
        },

        truncateUrl(url, maxLength = 40) {
            if (!url) return '';
            if (url.length <= maxLength) return url;
            return url.substring(0, maxLength - 3) + '...';
        },

        getJobTitle(job) {
            if (job._isAction) {
                return job.label || job.actionId;
            }
            // BH-026: group-export jobs don't have a URL; derive a useful title.
            if (job.source === 'group-export') {
                return job.name || 'Group export';
            }
            return this.getFilename(job.url) || job.name || 'Download';
        },

        getJobSubtitle(job) {
            if (job._isAction) {
                return job.message || '';
            }
            return this.truncateUrl(job.url, 50);
        },

        getFilename(url) {
            if (!url) return '';
            try {
                const pathname = new URL(url).pathname;
                const filename = pathname.split('/').pop();
                return filename || url;
            } catch {
                return url.split('/').pop() || url;
            }
        }
    };
}
