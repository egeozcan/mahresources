import { createLiveRegion } from '../utils/ariaLiveRegion.js';

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

            // BH-036: pick up the retention window from the meta tag emitted by base.tpl.
            const metaEl = document.querySelector('meta[name="x-export-retention-ms"]');
            if (metaEl) {
                this.exportRetentionMs = parseInt(metaEl.getAttribute('content'), 10) || 0;
            }

            // Listen for jobs-panel-open event (e.g., from pluginActionModal)
            window.addEventListener('jobs-panel-open', () => { this.isOpen = true; });

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
                    this.$nextTick(() => {
                        const panel = this.$refs.panel;
                        if (!panel) return;
                        const firstFocusable = panel.querySelector(
                            'button:not([disabled]), [href], input:not([disabled]), [tabindex]:not([tabindex="-1"])'
                        );
                        (firstFocusable || panel).focus();
                    });
                } else if (this._lastTrigger) {
                    this._lastTrigger.focus();
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

        toggle(event) {
            if (!this.isOpen && event?.currentTarget) {
                this._lastTrigger = event.currentTarget;
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
                this.jobs.push(job);
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
                const existingJob = this.jobs.find(j => j.id === job.id);

                // Retain completed/failed/cancelled jobs (non-active)
                if (existingJob && !this.isActive(existingJob)) {
                    // Avoid duplicates
                    if (!this.retainedCompletedJobs.some(j => j.id === existingJob.id)) {
                        this.retainedCompletedJobs.unshift(existingJob);
                        // Keep only the last 5
                        if (this.retainedCompletedJobs.length > 5) {
                            this.retainedCompletedJobs = this.retainedCompletedJobs.slice(0, 5);
                        }
                    }
                }

                // Remove from main jobs array
                this.jobs = this.jobs.filter(j => j.id !== job.id);
            });

            this.eventSource.addEventListener('action_added', (e) => {
                const { job } = JSON.parse(e.data);
                job._isAction = true;
                this.jobs.push(job);
                this.isOpen = true;
                this.announce(`Action started: ${job.label}`);
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
                const existingJob = this.jobs.find(j => j.id === job.id);
                if (existingJob && (existingJob.status === 'completed' || existingJob.status === 'failed')) {
                    if (!this.retainedCompletedJobs.some(j => j.id === existingJob.id)) {
                        existingJob._isAction = true;
                        this.retainedCompletedJobs.unshift(existingJob);
                        if (this.retainedCompletedJobs.length > 5) {
                            this.retainedCompletedJobs = this.retainedCompletedJobs.slice(0, 5);
                        }
                    }
                }
                this.jobs = this.jobs.filter(j => j.id !== job.id);
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

        cancelJob(jobId) {
            fetch('/v1/jobs/cancel', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: `id=${encodeURIComponent(jobId)}`
            }).catch(err => console.error('Cancel failed:', err));
        },

        pauseJob(jobId) {
            fetch('/v1/jobs/pause', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: `id=${encodeURIComponent(jobId)}`
            }).catch(err => console.error('Pause failed:', err));
        },

        resumeJob(jobId) {
            fetch('/v1/jobs/resume', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: `id=${encodeURIComponent(jobId)}`
            }).catch(err => console.error('Resume failed:', err));
        },

        retryJob(jobId) {
            fetch('/v1/jobs/retry', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: `id=${encodeURIComponent(jobId)}`
            }).catch(err => console.error('Retry failed:', err));
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

        isActive(job) {
            return ['pending', 'downloading', 'processing', 'running'].includes(job.status);
        },

        canPause(job) {
            return ['pending', 'downloading'].includes(job.status);
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

        get displayJobs() {
            const jobIds = new Set(this.jobs.map(j => j.id));
            const unique = this.retainedCompletedJobs.filter(j => !jobIds.has(j.id));
            return [...this.jobs, ...unique];
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
