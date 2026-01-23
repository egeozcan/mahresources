export function downloadCockpit() {
    return {
        isOpen: false,
        jobs: [],
        retainedCompletedJobs: [],  // Completed jobs retained for display after backend removal
        eventSource: null,
        connectionStatus: 'disconnected', // 'connected', 'disconnected', 'connecting'
        liveRegion: null,
        announceTimeout: null,
        speedTracking: {},  // Track progress for speed calculation: { jobId: { lastProgress, lastTime, speed } }

        statusIcons: {
            pending: '\u23F3',      // Hourglass
            downloading: '\u2B07',  // Down arrow
            processing: '\u2699',   // Gear
            completed: '\u2705',    // Check mark
            failed: '\u274C',       // X mark
            cancelled: '\u26D4'     // No entry
        },

        statusLabels: {
            pending: 'Pending',
            downloading: 'Downloading',
            processing: 'Processing',
            completed: 'Completed',
            failed: 'Failed',
            cancelled: 'Cancelled'
        },

        init() {
            // Create ARIA live region for screen reader announcements
            this.liveRegion = document.createElement('div');
            this.liveRegion.setAttribute('role', 'status');
            this.liveRegion.setAttribute('aria-live', 'polite');
            this.liveRegion.setAttribute('aria-atomic', 'true');
            Object.assign(this.liveRegion.style, {
                position: 'absolute',
                width: '1px',
                height: '1px',
                padding: '0',
                margin: '-1px',
                overflow: 'hidden',
                clip: 'rect(0, 0, 0, 0)',
                whiteSpace: 'nowrap',
                border: '0'
            });
            document.body.appendChild(this.liveRegion);

            // Listen for keyboard shortcut: Cmd/Ctrl+Shift+D
            document.addEventListener('keydown', (e) => {
                if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'd') {
                    e.preventDefault();
                    this.toggle();
                }
            });

            // Always connect to SSE to track download completions
            this.connect();

            this.$watch('isOpen', (value) => {
                if (value) {
                    this.announce('Download cockpit opened. Shows background download progress.');
                }
            });
        },

        announce(message) {
            if (this.liveRegion) {
                if (this.announceTimeout) {
                    clearTimeout(this.announceTimeout);
                }
                this.liveRegion.textContent = '';
                this.announceTimeout = setTimeout(() => {
                    this.liveRegion.textContent = message;
                }, 50);
            }
        },

        destroy() {
            if (this.liveRegion && this.liveRegion.parentNode) {
                this.liveRegion.parentNode.removeChild(this.liveRegion);
            }
            if (this.announceTimeout) {
                clearTimeout(this.announceTimeout);
            }
            this.disconnect();
        },

        toggle() {
            this.isOpen = !this.isOpen;
        },

        close() {
            this.isOpen = false;
        },

        connect() {
            if (this.eventSource) return;

            this.connectionStatus = 'connecting';
            this.eventSource = new EventSource('/v1/download/events');

            this.eventSource.addEventListener('init', (e) => {
                const data = JSON.parse(e.data);
                this.jobs = data.jobs || [];
                this.retainedCompletedJobs = [];  // Clear retained on reconnect
                this.connectionStatus = 'connected';
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

            this.eventSource.onerror = () => {
                this.connectionStatus = 'disconnected';
                this.disconnect();
                // Reconnect after 3 seconds
                setTimeout(() => {
                    if (this.isOpen) this.connect();
                }, 3000);
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
            fetch('/v1/download/cancel', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: `id=${encodeURIComponent(jobId)}`
            }).catch(err => console.error('Cancel failed:', err));
        },

        formatProgress(job) {
            if (job.totalSize > 0) {
                const downloaded = this.formatBytes(job.progress);
                const total = this.formatBytes(job.totalSize);
                const percent = job.progressPercent.toFixed(1);
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

        getProgressPercent(job) {
            if (job.totalSize > 0 && job.progress > 0) {
                return Math.min(100, (job.progress / job.totalSize) * 100);
            }
            return 0;
        },

        isActive(job) {
            return ['pending', 'downloading', 'processing'].includes(job.status);
        },

        get activeCount() {
            return this.jobs.filter(j => this.isActive(j)).length;
        },

        get hasActiveJobs() {
            return this.activeCount > 0;
        },

        get displayJobs() {
            return [...this.jobs, ...this.retainedCompletedJobs];
        },

        truncateUrl(url, maxLength = 40) {
            if (!url) return '';
            if (url.length <= maxLength) return url;
            return url.substring(0, maxLength - 3) + '...';
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
