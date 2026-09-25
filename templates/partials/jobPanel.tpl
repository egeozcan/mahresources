<div x-data="jobPanel()" data-testid="job-panel-root" class="relative">
    <button type="button" class="job-panel-trigger inline-flex items-center gap-2 rounded border border-stone-300 bg-white px-2 py-1.5 text-sm font-medium text-stone-800 hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-amber-700"
            @click="toggle($event)" aria-label="Open Jobs panel" aria-controls="job-center-panel"
            :aria-expanded="isOpen.toString()" title="Jobs (Control or Command + Shift + D)">
        <svg aria-hidden="true" class="h-5 w-5 text-stone-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="M4 6h16M4 12h10M4 18h16" />
            <circle cx="18" cy="12" r="2" stroke-width="1.7" />
        </svg>
        <span>Jobs</span>
        <span class="rounded-full bg-stone-100 px-1.5 py-0.5 text-xs text-stone-700" x-text="activeCount" :aria-label="'Active jobs shown: ' + activeCount"></span>
        <span x-show="attentionCount > 0" class="rounded-full border border-amber-700 px-1.5 py-0.5 text-xs font-medium text-amber-900" x-text="attentionCount" :aria-label="'Jobs needing attention shown: ' + attentionCount"></span>
    </button>

    {# x-if must stay the outer template: with x-teleport outside, x-ref="panel" #}
    {# never registers and focus never moves into the drawer. #}
    <template x-if="isOpen">
        <template x-teleport=".overlays">
            <div class="fixed inset-0 z-40 overflow-hidden" data-testid="job-panel-overlay">
                <div class="fixed inset-0 bg-black/20" aria-hidden="true" @click="close()"></div>
                {# A full-height drawer from the right, as the download cockpit was. #}
                {# The slide-in is a CSS animation (.job-drawer), because x-if does not #}
                {# run x-transition, and it is dropped under prefers-reduced-motion. #}
                <section id="job-center-panel" x-ref="panel" role="dialog" aria-modal="true" aria-labelledby="job-center-panel-title"
                         data-testid="job-panel-drawer"
                         class="job-drawer fixed inset-y-0 right-0 z-[49] flex w-full max-w-md flex-col overflow-hidden border-l border-stone-300 bg-white shadow-xl"
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
                        <span class="block text-xs font-mono uppercase tracking-wide text-stone-500">Active and scheduled jobs shown</span>
                        <span class="text-lg font-semibold text-stone-900" x-text="activeCount"></span>
                    </div>
                    <div class="rounded bg-stone-50 px-3 py-2">
                        <span class="block text-xs font-mono uppercase tracking-wide text-stone-500">Jobs needing attention shown</span>
                        <span class="text-lg font-semibold text-stone-900" x-text="attentionCount"></span>
                    </div>
                </div>

                <p x-show="error" x-cloak role="alert" class="mx-3 mt-3 rounded border border-red-300 bg-red-50 p-2 text-sm text-red-800" x-text="error"></p>
                <p x-show="notice" x-cloak class="mx-3 mt-3 rounded border border-stone-200 p-2 text-sm text-stone-800" x-text="notice"></p>

                <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-3" aria-label="Recent jobs">
                    <template x-for="group in groups" :key="group.key">
                        <section :aria-labelledby="'job-panel-group-' + group.key" class="space-y-2" :data-job-panel-group="group.key">
                            <h3 :id="'job-panel-group-' + group.key" class="flex items-baseline gap-2 text-xs font-mono font-semibold uppercase tracking-wide text-stone-600">
                                <span x-text="group.title"></span>
                                <span class="font-normal text-stone-500" x-text="'(' + group.jobs.length + ')'"></span>
                            </h3>
                            <template x-for="job in group.jobs" :key="job.id">
                                <article class="rounded border border-stone-200 p-3" data-job-panel-row :data-job-id="job.id" :aria-labelledby="'job-panel-title-' + job.id">
                                    <div class="flex items-start justify-between gap-3">
                                        <div class="min-w-0">
                                            <a :id="'job-panel-title-' + job.id" :href="detailURL(job)" class="block truncate text-sm font-medium text-amber-900 underline decoration-amber-300 underline-offset-2" x-text="job.title || job.kind || job.id"></a>
                                            <p class="mt-1 flex flex-wrap items-center gap-x-2 text-xs text-stone-600"><span x-text="stateLabel(job)"></span><span aria-hidden="true">·</span><span x-text="job.kind"></span><span x-show="job.pinned" x-cloak class="inline-flex items-center rounded border border-amber-400 bg-amber-50 px-1.5 py-0.5 font-medium text-amber-900">Pinned by you</span></p>
                                        </div>
                                        <div class="flex shrink-0 items-center gap-2">
                                            <template x-if="resultOutput(job)">
                                                <a :href="resultURL(job)" :aria-label="resultAccessibleLabel(job)" class="text-xs font-medium text-amber-900 underline decoration-amber-300 underline-offset-2" x-text="resultLinkLabel(job)"></a>
                                            </template>
                                            <a :href="detailURL(job)" :aria-label="'Details for ' + (job.title || job.kind || job.id)" class="text-xs font-medium text-stone-700 underline">Details</a>
                                        </div>
                                    </div>

                                    {# The reason a job failed, as its Kind recorded it; the /jobs detail page shows the same text. #}
                                    <p x-show="failureText(job)" x-cloak class="mt-2 break-words rounded border border-red-300 bg-red-50 px-2 py-1 text-xs text-red-900" data-job-panel-failure><span class="font-medium">Reason:</span> <span class="whitespace-pre-wrap" x-text="failureText(job)"></span></p>

                                    <template x-if="showsProgress(job)">
                                        <div class="mt-2" data-job-panel-progress>
                                            <div class="mb-1 flex justify-between gap-2 text-xs text-stone-700">
                                                <span class="min-w-0 truncate" x-text="progressLabel(job)"></span>
                                                <span class="shrink-0 tabular-nums" x-text="progressValue(job) === null ? (progressIndeterminate(job) ? 'In progress' : '') : progressValue(job) + '%'"></span>
                                            </div>
                                            <div class="h-2 overflow-hidden rounded bg-stone-200" role="progressbar" aria-valuemin="0" aria-valuemax="100"
                                                 :aria-valuenow="progressValue(job)" :aria-valuetext="progressValueText(job)"
                                                 :aria-label="(job.title || job.kind || 'Job') + ' progress'">
                                                <div class="h-2 rounded" :class="[job.state === 'paused' ? 'bg-stone-400' : 'bg-amber-800', progressIndeterminate(job) ? 'w-full motion-safe:animate-pulse' : '']"
                                                     :style="{ width: progressIndeterminate(job) ? '100%' : (progressValue(job) ?? 0) + '%' }"></div>
                                            </div>
                                        </div>
                                    </template>
                                    {# One line for both: the amount, speed and time left under a running bar, #}
                                    {# the average speed on a finished row that has no bar. #}
                                    <p x-show="statsText(job)" class="mt-1 text-xs tabular-nums text-stone-600" data-job-panel-stats x-text="statsText(job)"></p>

                                    <template x-if="metricsFor(job).length > 0">
                                        <dl class="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-xs" data-job-panel-metrics>
                                            <template x-for="metric in metricsFor(job)" :key="metric.key">
                                                <div class="flex min-w-0 justify-between gap-2">
                                                    <dt class="truncate text-stone-500" x-text="metric.label || metric.key"></dt>
                                                    <dd class="shrink-0 font-medium tabular-nums text-stone-800" x-text="metricText(metric)"></dd>
                                                </div>
                                            </template>
                                        </dl>
                                    </template>

                                    <template x-if="graphsFor(job).length > 0">
                                        <div class="mt-2 grid gap-2" :class="graphsFor(job).length > 1 ? 'grid-cols-2' : ''" data-job-panel-graphs>
                                            <template x-for="series in graphsFor(job)" :key="series.key">
                                                <figure class="rounded bg-stone-50 px-2 py-1" data-job-panel-graph :data-series="series.key">
                                                    <figcaption class="flex justify-between gap-2 text-xs text-stone-600">
                                                        <span class="truncate" x-text="series.label"></span>
                                                        <span class="shrink-0 tabular-nums" x-text="graphLatest(series)"></span>
                                                    </figcaption>
                                                    <svg viewBox="0 0 120 28" preserveAspectRatio="none" class="mt-0.5 h-7 w-full text-amber-800" role="img" :aria-label="graphLabel(series)">
                                                        <path :d="sparkline(series)" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
                                                    </svg>
                                                </figure>
                                            </template>
                                        </div>
                                    </template>

                                    <div class="mt-2 flex flex-wrap gap-2" role="group" aria-label="Advertised controls">
                                        <template x-for="command in commandsFor(job)" :key="command.key">
                                            <button type="button" @click="runCommand(job, command)" class="rounded border border-stone-300 px-2 py-1 text-xs font-medium text-stone-800 hover:bg-stone-50" x-text="command.label || command.key"></button>
                                        </template>
                                    </div>
                                </article>
                            </template>
                        </section>
                    </template>
                    <p x-show="jobs.length === 0 && !error" class="rounded border border-dashed border-stone-300 p-5 text-center text-sm text-stone-600">No visible jobs yet.</p>
                    <p x-show="finishedHasMore" x-cloak class="text-xs text-stone-600" data-job-panel-finished-more>Showing the newest <span x-text="finishedLimit"></span> finished jobs. Older ones are on All jobs.</p>
                </div>

                <footer class="flex flex-wrap items-center justify-between gap-2 border-t border-stone-200 bg-stone-50 p-3">
                    <button type="button" data-job-panel-dismiss-finished x-show="finishedCount > 0 || busy" @click="dismissFinished()" :aria-disabled="busy.toString()" x-text="busy ? 'Dismissing…' : 'Dismiss finished'" class="rounded border border-stone-400 bg-white px-3 py-2 text-sm font-medium text-stone-800 hover:bg-stone-100 aria-disabled:cursor-not-allowed aria-disabled:opacity-50">Dismiss finished</button>
                    <a href="/jobs" data-job-panel-all-jobs class="ml-auto rounded bg-stone-800 px-3 py-2 text-sm font-medium text-white hover:bg-stone-900">All jobs</a>
                </footer>

                </section>
            </div>
        </template>
    </template>
</div>
