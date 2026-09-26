<div x-data="jobPanel()" data-testid="job-panel-root" class="relative">
    <button type="button" class="job-panel-trigger inline-flex items-center gap-2 rounded border border-stone-300 bg-white px-2 py-1.5 text-sm font-medium text-stone-800 hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-amber-700"
            @click="toggle($event)" aria-label="Open Jobs panel" :aria-controls="isOpen ? 'job-center-panel' : null"
            :aria-expanded="isOpen.toString()" aria-describedby="job-panel-trigger-counts" title="Jobs (Control or Command + Shift + D)">
        <svg aria-hidden="true" class="h-5 w-5 text-stone-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="M4 6h16M4 12h10M4 18h16" />
            <circle cx="18" cy="12" r="2" stroke-width="1.7" />
        </svg>
        <span>Jobs</span>
        {# The button's aria-label replaces its contents as its name, so the badges are read through its description instead. #}
        <span aria-hidden="true" class="rounded-full bg-stone-100 px-1.5 py-0.5 text-xs text-stone-700" data-job-panel-active-badge x-text="activeCount"></span>
        <span aria-hidden="true" x-show="attentionCount > 0" class="rounded-full border border-amber-700 px-1.5 py-0.5 text-xs font-medium text-amber-900" data-job-panel-attention-badge x-text="attentionCount"></span>
    </button>
    <span id="job-panel-trigger-counts" class="sr-only" x-text="countsText"></span>

    {# x-if must stay the outer template: with x-teleport outside, x-ref="panel" #}
    {# never registers and focus never moves into the drawer. #}
    <template x-if="isOpen">
        <template x-teleport=".overlays">
            <div class="fixed inset-0 z-40 overflow-hidden" data-testid="job-panel-overlay">
                <div class="fixed inset-0 bg-black/20" aria-hidden="true" @click="close()"></div>
                {# A full-height drawer from the right, as the download cockpit was. #}
                {# The slide-in is a CSS animation (.job-drawer), because x-if does not #}
                {# run x-transition, and it is dropped under prefers-reduced-motion. #}
                <section id="job-center-panel" x-ref="panel" role="dialog" aria-modal="true" aria-labelledby="job-center-panel-title" aria-describedby="job-center-panel-counts"
                         data-testid="job-panel-drawer"
                         class="job-drawer fixed inset-y-0 right-0 z-[49] flex w-full max-w-md flex-col overflow-hidden border-l border-stone-300 bg-white shadow-xl"
                         x-trap.noscroll.noreturn="isOpen" @keydown.escape="close()">
                {# The layout follows the download cockpit this drawer replaced: a flat, #}
                {# divided list with a status icon and pill per row, and quiet text #}
                {# controls, rather than a stack of bordered cards and buttons. #}
                <header class="flex items-center justify-between gap-3 border-b border-stone-200 bg-stone-50 px-4 py-3">
                    <div class="flex min-w-0 items-baseline gap-3">
                        <h2 id="job-center-panel-title" class="font-mono text-lg font-semibold text-stone-900">Jobs</h2>
                        {# The trigger's description is outside this aria-modal dialog, so the dialog carries the counts too, zeroes included. #}
                        <p id="job-center-panel-counts" class="sr-only" x-text="countsText"></p>
                        <p class="flex items-center gap-1.5 text-xs text-stone-600">
                            <span aria-hidden="true" class="h-2 w-2 shrink-0 rounded-full"
                                  :class="{ 'bg-green-600': connectionStatus === 'connected', 'bg-red-600': connectionStatus === 'reconnecting', 'bg-amber-500 motion-safe:animate-pulse': connectionStatus !== 'connected' && connectionStatus !== 'reconnecting' }"></span>
                            <span role="status" aria-live="polite" x-text="connectionStatus === 'connected' ? 'Live updates connected' : connectionStatus === 'reconnecting' ? 'Reconnecting' : 'Connecting to live updates'"></span>
                        </p>
                    </div>
                    <button type="button" @click="close()" class="rounded p-1 text-stone-500 hover:text-stone-800 focus:outline-none focus:ring-2 focus:ring-amber-700" aria-label="Close Jobs panel">
                        <svg aria-hidden="true" class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18 18 6M6 6l12 12" /></svg>
                    </button>
                </header>

                {# Announcements while the drawer is open: it is aria-modal, so the page's own live region may go unheard. #}
                <div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-job-panel-announcer></div>

                <p x-show="error" x-cloak role="alert" class="border-b border-red-200 bg-red-50 px-4 py-2 text-sm text-red-800" x-text="error"></p>
                <p x-show="notice" x-cloak class="border-b border-stone-200 px-4 py-2 text-sm text-stone-700" data-job-panel-notice x-text="notice"></p>

                <div class="min-h-0 flex-1 overflow-y-auto" aria-label="Recent jobs">
                    <template x-for="group in groups" :key="group.key">
                        <section :aria-labelledby="'job-panel-group-' + group.key" :data-job-panel-group="group.key">
                            <h3 :id="'job-panel-group-' + group.key" class="sticky top-0 z-10 flex items-baseline gap-2 border-b border-stone-200 bg-stone-50 px-4 py-1.5 font-mono text-xs font-semibold uppercase tracking-wide text-stone-600">
                                <span x-text="group.title"></span>
                                <span class="font-normal text-stone-500" x-text="'(' + group.jobs.length + ')'"></span>
                            </h3>
                            {# role="list": Safari drops list semantics from a list styled without markers. #}
                            <ul role="list" class="divide-y divide-stone-100 border-b border-stone-200">
                            <template x-for="job in group.jobs" :key="job.id">
                                <li class="px-4 py-3 hover:bg-stone-50">
                                <article data-job-panel-row :data-job-id="job.id" :aria-labelledby="'job-panel-title-' + job.id" class="flex items-start gap-3">
                                    {# The icon repeats the pill beside the title, so it is hidden from assistive technology. #}
                                    <span aria-hidden="true" class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full"
                                          :class="{
                                              'bg-amber-100 text-amber-800': stateTone(job) === 'working',
                                              'bg-stone-100 text-stone-600': stateTone(job) === 'waiting' || stateTone(job) === 'neutral',
                                              'bg-yellow-100 text-yellow-800': stateTone(job) === 'paused' || stateTone(job) === 'warning',
                                              'bg-green-100 text-green-800': stateTone(job) === 'done',
                                              'bg-red-100 text-red-700': stateTone(job) === 'failed'
                                          }">
                                        <svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" stroke-linecap="round" stroke-linejoin="round">
                                            <path x-show="stateTone(job) === 'working'" d="M12 4v12m0 0-4-4m4 4 4-4M5 20h14" />
                                            <path x-show="stateTone(job) === 'waiting'" d="M12 7v5l3 2m6-2a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
                                            <path x-show="stateTone(job) === 'paused'" d="M9 6v12M15 6v12" />
                                            <path x-show="stateTone(job) === 'done'" d="m5 13 4 4L19 7" />
                                            <path x-show="stateTone(job) === 'warning'" d="M12 8v5m0 3.5v.01M12 3 2 20h20L12 3Z" />
                                            <path x-show="stateTone(job) === 'failed'" d="M6 6l12 12M18 6 6 18" />
                                            <path x-show="stateTone(job) === 'neutral'" d="M6 12h12" />
                                        </svg>
                                    </span>

                                    <div class="min-w-0 flex-1">
                                        <div class="flex items-start justify-between gap-2">
                                            <a :id="'job-panel-title-' + job.id" :href="detailURL(job)" :title="job.title || job.kind || job.id"
                                               class="min-w-0 truncate text-sm font-medium text-stone-900 underline decoration-stone-300 underline-offset-2 hover:text-amber-900 hover:decoration-amber-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-700"
                                               x-text="job.title || job.kind || job.id"></a>
                                            <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
                                                  :class="{
                                                      'bg-amber-100 text-amber-900': stateTone(job) === 'working',
                                                      'bg-stone-100 text-stone-700': stateTone(job) === 'waiting' || stateTone(job) === 'neutral',
                                                      'bg-yellow-100 text-yellow-900': stateTone(job) === 'paused' || stateTone(job) === 'warning',
                                                      'bg-green-100 text-green-800': stateTone(job) === 'done',
                                                      'bg-red-100 text-red-800': stateTone(job) === 'failed'
                                                  }"
                                                  x-text="stateLabel(job)"></span>
                                        </div>
                                        <p class="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 text-xs text-stone-600">
                                            <span class="truncate" x-text="job.kind"></span>
                                            <span x-show="job.pinned" x-cloak class="inline-flex items-center rounded border border-amber-400 bg-amber-50 px-1.5 font-medium text-amber-900">Pinned by you</span>
                                        </p>

                                        {# The reason a job failed, as its Kind recorded it; the /jobs detail page shows the same text. #}
                                        <p x-show="failureText(job)" x-cloak class="mt-1 break-words text-xs text-red-800" data-job-panel-failure><span class="font-medium">Reason:</span> <span class="whitespace-pre-wrap" x-text="failureText(job)"></span></p>

                                        <template x-if="showsProgress(job)">
                                            <div class="mt-2" data-job-panel-progress>
                                                <div class="mb-1 flex justify-between gap-2 text-xs text-stone-600">
                                                    <span class="min-w-0 truncate" x-text="progressLabel(job)"></span>
                                                    <span class="shrink-0 font-medium tabular-nums text-amber-900" x-text="progressValue(job) === null ? (progressIndeterminate(job) ? 'In progress' : '') : progressValue(job) + '%'"></span>
                                                </div>
                                                <div class="h-2 overflow-hidden rounded-full bg-stone-200" role="progressbar" aria-valuemin="0" aria-valuemax="100"
                                                     :aria-valuenow="progressValue(job)" :aria-valuetext="progressValueText(job)"
                                                     :aria-label="(job.title || job.kind || 'Job') + ' progress'">
                                                    <div class="h-2 rounded-full transition-[width] duration-300 motion-reduce:transition-none" :class="[job.state === 'paused' ? 'bg-stone-400' : 'bg-amber-700', progressIndeterminate(job) ? 'w-full motion-safe:animate-pulse' : '']"
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

                                        {# Text controls, as the cockpit had. Each is at least 24px tall (WCAG 2.5.8); #}
                                        {# the negative margin lines the first one's text up with the row above. #}
                                        {# Pin, lineage and forget keep the record rather than act on the work, #}
                                        {# so they wait under More; opened, it takes a line of its own. #}
                                        <div x-show="resultOutput(job) || commandsFor(job).length > 0" class="-ml-1.5 mt-1 flex flex-wrap items-center gap-x-1" role="group" aria-label="Advertised controls">
                                            <template x-if="resultOutput(job)">
                                                <a :href="resultURL(job)" :aria-label="resultAccessibleLabel(job)" class="inline-flex min-h-6 items-center rounded px-1.5 text-xs font-medium text-amber-800 underline decoration-amber-300 underline-offset-2 hover:decoration-amber-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-700"><span x-text="resultLinkLabel(job)"></span>&nbsp;<span aria-hidden="true">&rarr;</span></a>
                                            </template>
                                            <template x-for="command in primaryCommandsFor(job)" :key="command.key">
                                                <button type="button" @click="runCommand(job, command)" :data-command-key="command.key"
                                                        class="inline-flex min-h-6 items-center rounded px-1.5 text-xs font-medium hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-700"
                                                        :class="command.key === 'cancel' || command.destructive ? 'text-red-700' : command.key === 'dismiss' ? 'text-stone-600' : 'text-amber-800'"
                                                        x-text="command.label || command.key"></button>
                                            </template>
                                            <details x-show="moreCommandsFor(job).length > 0" class="group open:basis-full">
                                                <summary class="inline-flex min-h-6 cursor-pointer list-none items-center gap-1 rounded px-1.5 text-xs font-medium text-stone-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-700 [&::-webkit-details-marker]:hidden">
                                                    More
                                                    <svg aria-hidden="true" class="h-3 w-3 transition-transform group-open:rotate-180 motion-reduce:transition-none" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="m6 9 6 6 6-6" /></svg>
                                                </summary>
                                                <div class="flex flex-wrap items-center gap-x-1">
                                                    <template x-for="command in moreCommandsFor(job)" :key="command.key">
                                                        <button type="button" @click="runCommand(job, command)" :data-command-key="command.key"
                                                                class="inline-flex min-h-6 items-center rounded px-1.5 text-xs font-medium text-stone-700 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-700"
                                                                x-text="command.label || command.key"></button>
                                                    </template>
                                                </div>
                                            </details>
                                        </div>
                                    </div>
                                </article>
                                </li>
                            </template>
                            </ul>
                        </section>
                    </template>
                    <div x-show="jobs.length === 0 && !error" class="flex flex-col items-center justify-center p-8 text-center text-stone-600">
                        <svg aria-hidden="true" class="mb-3 h-12 w-12 text-stone-300" fill="none" stroke="currentColor" stroke-width="1" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h10M4 18h16" /><circle cx="18" cy="12" r="2" /></svg>
                        <p class="text-sm">No visible jobs yet.</p>
                        <p class="mt-1 text-xs text-stone-500">Downloads, exports, imports and plugin work appear here while they run.</p>
                    </div>
                    <p x-show="finishedHasMore" x-cloak class="px-4 py-2 text-xs text-stone-600" data-job-panel-finished-more>Showing the newest <span x-text="finishedLimit"></span> finished jobs. Older ones are on All jobs.</p>
                </div>

                <footer class="border-t border-stone-200 text-xs text-stone-600">
                    <div class="flex items-center justify-between gap-2 px-4 py-2">
                        <button type="button" data-job-panel-dismiss-finished x-show="finishedCount > 0 || busy" @click="dismissFinished()" :aria-disabled="busy.toString()" x-text="busy ? 'Dismissing…' : 'Dismiss finished'" class="inline-flex min-h-6 items-center rounded text-stone-700 underline decoration-stone-300 underline-offset-2 hover:decoration-stone-700 focus:outline-none focus:ring-2 focus:ring-amber-700 aria-disabled:cursor-not-allowed aria-disabled:opacity-50">Dismiss finished</button>
                        <a href="/jobs" data-job-panel-all-jobs class="ml-auto inline-flex min-h-6 items-center rounded font-medium text-amber-800 underline decoration-amber-300 underline-offset-2 hover:decoration-amber-800 focus:outline-none focus:ring-2 focus:ring-amber-700">All jobs</a>
                    </div>
                    <p class="hidden border-t border-stone-200 bg-stone-50 px-4 py-2 text-center sm:block">
                        Press <kbd class="rounded border border-stone-300 bg-white px-1.5 py-0.5 font-mono">Esc</kbd> to close
                        or <kbd class="rounded border border-stone-300 bg-white px-1.5 py-0.5 font-mono"><span x-text="/Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'"></span>+Shift+D</kbd> to toggle
                    </p>
                </footer>

                </section>
            </div>
        </template>
    </template>
</div>
