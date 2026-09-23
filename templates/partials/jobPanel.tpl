<div x-data="jobPanel()" data-testid="job-panel-root" class="relative">
    <button type="button" class="job-panel-trigger inline-flex items-center gap-2 rounded border border-stone-300 bg-white px-2 py-1.5 text-sm font-medium text-stone-800 hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-amber-700"
            @click="toggle($event)" aria-label="Open Jobs panel" aria-controls="job-center-panel"
            :aria-expanded="isOpen.toString()" title="Jobs (Control or Command + Shift + D)">
        <svg aria-hidden="true" class="h-5 w-5 text-stone-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="M4 6h16M4 12h10M4 18h16" />
            <circle cx="18" cy="12" r="2" stroke-width="1.7" />
        </svg>
        <span>Jobs</span>
        <span class="rounded-full bg-stone-100 px-1.5 py-0.5 text-xs text-stone-700" x-text="activeCount" :aria-label="'Active jobs: ' + activeCount"></span>
        <span x-show="attentionCount > 0" class="rounded-full border border-amber-700 px-1.5 py-0.5 text-xs font-medium text-amber-900" x-text="attentionCount" :aria-label="'Jobs needing attention: ' + attentionCount"></span>
    </button>

    <template x-if="isOpen">
        <template x-teleport=".overlays">
            <div class="fixed inset-0 z-40 overflow-hidden" data-testid="job-panel-overlay">
                <div class="fixed inset-0 bg-black/20" aria-hidden="true" @click="close()"></div>
                <section id="job-center-panel" x-ref="panel" role="dialog" aria-modal="true" aria-labelledby="job-center-panel-title"
                         class="fixed inset-x-0 bottom-0 z-[49] flex max-h-[85vh] w-full flex-col overflow-hidden rounded-t-lg border border-stone-300 bg-white shadow-xl sm:inset-auto sm:right-4 sm:top-14 sm:w-[min(28rem,calc(100vw-2rem))] sm:rounded-lg"
                         x-trap.noscroll.noreturn="isOpen" @keydown.escape="close()">
                <header class="flex items-start justify-between gap-3 border-b border-stone-200 bg-stone-50 p-4">
                    <div>
                        <h2 id="job-center-panel-title" class="text-lg font-semibold text-stone-900">Jobs</h2>
                        <p class="mt-1 text-xs text-stone-600" role="status" aria-live="polite" x-text="connectionStatus === 'connected' ? 'Live updates connected' : connectionStatus === 'reconnecting' ? 'Reconnecting' : 'Connecting to live updates'"></p>
                    </div>
                    <button type="button" @click="close()" class="rounded border border-stone-300 p-2 text-stone-700 hover:bg-white" aria-label="Close Jobs panel">
                        <svg aria-hidden="true" class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="m6 6 12 12M18 6 6 18" /></svg>
                    </button>
                </header>

                <div class="grid grid-cols-2 gap-2 border-b border-stone-200 p-3">
                    <div class="rounded bg-stone-50 px-3 py-2">
                        <span class="block text-xs font-mono uppercase tracking-wide text-stone-500">Active and scheduled</span>
                        <span class="text-lg font-semibold text-stone-900" x-text="activeCount"></span>
                    </div>
                    <div class="rounded bg-stone-50 px-3 py-2">
                        <span class="block text-xs font-mono uppercase tracking-wide text-stone-500">Needs attention</span>
                        <span class="text-lg font-semibold text-stone-900" x-text="attentionCount"></span>
                    </div>
                </div>

                <p x-show="error" x-cloak role="alert" class="mx-3 mt-3 rounded border border-red-300 bg-red-50 p-2 text-sm text-red-800" x-text="error"></p>
                <p x-show="notice" x-cloak class="mx-3 mt-3 rounded border border-stone-200 p-2 text-sm text-stone-800" x-text="notice"></p>

                <div class="min-h-0 flex-1 space-y-3 overflow-y-auto p-3" aria-label="Recent jobs">
                    <template x-for="job in jobs" :key="job.id">
                        <article class="rounded border border-stone-200 p-3">
                            <div class="flex items-start justify-between gap-3">
                                <div class="min-w-0">
                                    <a :href="detailURL(job)" class="block truncate text-sm font-medium text-amber-900 underline decoration-amber-300 underline-offset-2" x-text="job.title || job.kind || job.id"></a>
                                    <p class="mt-1 flex flex-wrap gap-x-2 text-xs text-stone-600"><span x-text="stateLabel(job)"></span><span aria-hidden="true">·</span><span x-text="job.kind"></span></p>
                                </div>
                                <a :href="detailURL(job)" :aria-label="'Details for ' + (job.title || job.kind || job.id)" class="shrink-0 text-xs font-medium text-stone-700 underline">Details</a>
                            </div>
                            <div class="mt-2 flex flex-wrap gap-2" role="group" aria-label="Advertised controls">
                                <template x-for="command in commandsFor(job)" :key="command.key">
                                    <button type="button" @click="runCommand(job, command)" class="rounded border border-stone-300 px-2 py-1 text-xs font-medium text-stone-800 hover:bg-stone-50" x-text="command.label || command.key"></button>
                                </template>
                            </div>
                        </article>
                    </template>
                    <p x-show="jobs.length === 0 && !error" class="rounded border border-dashed border-stone-300 p-5 text-center text-sm text-stone-600">No visible jobs yet.</p>
                </div>

                <footer class="flex flex-wrap items-center justify-between gap-2 border-t border-stone-200 bg-stone-50 p-3">
                    <button type="button" x-show="finishedCount > 0" @click="dismissFinished()" :disabled="busy" class="rounded border border-stone-400 bg-white px-3 py-2 text-sm font-medium text-stone-800 hover:bg-stone-100 disabled:opacity-50">Dismiss finished</button>
                    <a href="/jobs" class="ml-auto rounded bg-stone-800 px-3 py-2 text-sm font-medium text-white hover:bg-stone-900">All jobs</a>
                </footer>

                <ul x-show="outcomes.length" x-cloak class="max-h-32 overflow-y-auto border-t border-stone-200 bg-white px-3 py-2 text-xs" aria-label="Dismiss outcomes">
                    <template x-for="outcome in outcomes" :key="outcome.jobId">
                        <li class="flex flex-wrap gap-x-2 py-1"><span class="font-mono text-stone-600" x-text="outcome.jobId"></span><span x-text="outcome.message || outcome.code || outcome.status"></span></li>
                    </template>
                </ul>
                </section>
            </div>
        </template>
    </template>
</div>
