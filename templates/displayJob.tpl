{% extends "/layouts/base.tpl" %}

{% block body %}
{# The page's one h1 is the layout's, naming the Job (job_template_context.go); this keeps the document title's state current. #}
<div x-data="jobCenter()" x-effect="syncDocumentTitle()" data-site-title="{{ title }}" data-testid="job-detail" class="mx-auto max-w-5xl space-y-5">
    <p x-show="loading" x-cloak role="status" class="py-6 text-sm text-stone-600">Loading job…</p>
    {# A failed read offers a way on: read again, or go back to the list. #}
    <div x-show="error" x-cloak class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-800" data-job-detail-error>
        <p role="alert" x-text="error"></p>
        <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1">
            <button type="button" @click="load()" class="inline-flex min-h-6 items-center rounded font-medium text-red-900 underline decoration-red-300 underline-offset-2 hover:decoration-red-800 focus:outline-hidden focus:ring-2 focus:ring-amber-700">Try again</button>
            <a href="/jobs?dismissed=false" class="inline-flex min-h-6 items-center rounded font-medium text-red-900 underline decoration-red-300 underline-offset-2 hover:decoration-red-800 focus:outline-hidden focus:ring-2 focus:ring-amber-700">All jobs</a>
        </div>
    </div>
    <template x-if="detail && !loading">
        <div class="space-y-6">
            <header class="flex flex-wrap items-start justify-between gap-3 border-b border-stone-300 pb-4">
                <div class="min-w-0">
                    <a href="/jobs?dismissed=false" class="text-sm text-amber-900 underline decoration-amber-300 underline-offset-2">All jobs</a>
                    <div class="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-sm text-stone-600">
                        <span class="rounded border border-stone-300 px-2 py-0.5 font-mono" x-text="stateLabel(detail)"></span>
                        <span x-show="detail.pinned" x-cloak class="inline-flex items-center rounded border border-amber-400 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-900">Pinned by you</span>
                        <span x-show="detail.dismissed" x-cloak data-job-dismissed class="inline-flex items-center rounded border border-stone-300 bg-stone-50 px-2 py-0.5 text-xs font-medium text-stone-700">Dismissed by you</span>
                        <span x-text="detail.kind"></span>
                        <span x-show="phaseText(detail)" x-text="phaseText(detail)"></span>
                        <span class="font-mono text-xs" x-text="detail.id"></span>
                    </div>
                    <p x-show="scheduledText(detail)" x-cloak class="mt-2 text-sm text-stone-800" data-job-scheduled-for x-text="scheduledText(detail)"></p>
                </div>
                <div class="flex flex-wrap gap-2" role="group" aria-label="Job actions" data-job-commands>
                    {# aria-disabled, not disabled, while a command runs: a disabled button drops the focus it holds. #}
                    <template x-for="command in commandsFor(detail)" :key="command.key">
                        <button type="button" @click="runCommand(detail, command)" :data-command-key="command.key" :aria-disabled="commandBusy ? 'true' : null"
                                class="rounded border bg-white px-3 py-2 text-sm font-medium hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-amber-700 aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
                                :class="command.destructive ? 'border-red-300 text-red-800' : 'border-stone-400 text-stone-800'"
                                x-text="command.label || command.key"></button>
                    </template>
                </div>
            </header>

            <section aria-labelledby="job-context-heading" class="rounded border border-stone-200 bg-white p-4">
                <h2 id="job-context-heading" class="font-mono text-sm font-semibold text-stone-800">Job context</h2>
                <dl class="mt-2 grid gap-2 text-sm sm:grid-cols-3">
                    <div x-show="accountText(detail, 'owner')" data-job-owner>
                        <dt class="text-xs text-stone-500">Owner</dt>
                        <dd class="break-words text-stone-800" x-text="accountText(detail, 'owner')"></dd>
                    </div>
                    <div x-show="accountText(detail, 'actor')" data-job-actor>
                        <dt class="text-xs text-stone-500">Actor</dt>
                        <dd class="break-words text-stone-800" x-text="accountText(detail, 'actor')"></dd>
                    </div>
                    <div x-show="detail.origin">
                        <dt class="text-xs text-stone-500">Origin</dt>
                        <dd class="break-words text-stone-800" x-text="detail.origin"></dd>
                    </div>
                </dl>
            </section>

            {# When the Job was accepted, ran and finished, how long it spent in each state, and how long its history is kept. #}
            <section aria-labelledby="job-times-heading" class="rounded border border-stone-200 bg-white p-4" data-job-times>
                <h2 id="job-times-heading" class="font-mono text-sm font-semibold text-stone-800">Times</h2>
                <p class="mt-1 text-xs text-stone-600" x-text="timeZoneText"></p>
                <dl class="mt-2 grid gap-2 text-sm sm:grid-cols-3">
                    <template x-for="row in timeRows(detail)" :key="row.key">
                        <div :data-job-time="row.key">
                            <dt class="text-xs text-stone-500" x-text="row.label"></dt>
                            <dd class="text-stone-800">
                                <time class="tabular-nums" :datetime="row.at" x-text="row.text"></time>
                                <span class="text-stone-600" x-text="'(' + row.relative + ')'"></span>
                            </dd>
                        </div>
                    </template>
                    <template x-for="row in durationRows(detail)" :key="row.key">
                        <div :data-job-duration="row.key">
                            <dt class="text-xs text-stone-500" x-text="row.label"></dt>
                            <dd class="tabular-nums text-stone-800" x-text="row.text"></dd>
                        </div>
                    </template>
                </dl>
                <p x-show="detail.pinned && detail.expiresAt" x-cloak class="mt-2 text-xs text-stone-600" data-job-pin-retention>Your pin keeps this job's history past that date. Its files keep their own expiry.</p>
            </section>

            <p x-show="noticeText" x-cloak data-job-notice class="rounded border border-stone-200 bg-white p-3 text-sm text-stone-800" x-text="noticeText"></p>

            <template x-if="showsProgress(detail)">
                <section aria-labelledby="job-progress-heading" class="rounded border border-stone-200 bg-white p-4">
                    <h2 id="job-progress-heading" class="font-mono text-sm font-semibold text-stone-800">Progress</h2>
                    <p class="mt-2 text-sm text-stone-700" x-text="progressText(detail)"></p>
                    <div class="mt-2 h-2 rounded bg-stone-200" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="progressValue(detail)" :aria-valuetext="progressAccessibleText(detail)" :aria-label="(detail.title || detail.kind || 'Job') + ' progress: ' + progressAccessibleText(detail)">
                        <div class="h-2 rounded bg-amber-800" :class="progressIndeterminate(detail) ? 'w-full motion-safe:animate-pulse' : ''" :style="progressIndeterminate(detail) ? '' : `width:${progressValue(detail) ?? 0}%`"></div>
                    </div>
                    <p x-show="statsText(detail)" class="mt-2 text-sm tabular-nums text-stone-700" data-job-stats x-text="statsText(detail)"></p>
                    <template x-if="metricsFor(detail).length > 0">
                        <dl class="mt-3 grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2" data-job-metrics>
                            <template x-for="metric in metricsFor(detail)" :key="metric.key">
                                <div class="flex justify-between gap-3 border-b border-stone-100 py-1">
                                    <dt class="text-stone-600" x-text="metric.label || metric.key"></dt>
                                    <dd class="font-medium tabular-nums text-stone-900" x-text="metricText(metric)"></dd>
                                </div>
                            </template>
                        </dl>
                    </template>
                    <template x-if="graphsFor(detail).length > 0">
                        <div class="mt-4 grid gap-4 sm:grid-cols-2" data-job-graphs>
                            <template x-for="series in graphsFor(detail)" :key="series.key">
                                <figure class="rounded border border-stone-200 p-3" data-job-graph :data-series="series.key">
                                    <figcaption class="flex justify-between gap-2 text-xs text-stone-600">
                                        <span x-text="series.label"></span>
                                        <span class="tabular-nums" x-text="graphLatest(series)"></span>
                                    </figcaption>
                                    <svg viewBox="0 0 480 80" preserveAspectRatio="none" class="mt-2 h-20 w-full text-amber-800" role="img" :aria-label="graphLabel(series)">
                                        <path :d="sparkline(series)" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
                                    </svg>
                                </figure>
                            </template>
                        </div>
                    </template>
                </section>
            </template>

            <section x-show="detail.failure?.message" x-cloak aria-labelledby="job-failure-heading" class="rounded border border-red-300 bg-red-50 p-4">
                <h2 id="job-failure-heading" class="font-mono text-sm font-semibold text-red-900">Failure</h2>
                <p class="mt-2 whitespace-pre-wrap break-words text-sm text-red-900" x-text="detail.failure?.message"></p>
                <p x-show="detail.failure?.class" class="mt-2 text-xs text-red-800" x-text="'Class: ' + detail.failure?.class"></p>
                <template x-if="failureOutput(detail)">
                    <p class="mt-3"><a :href="outputLinkURL(failureOutput(detail), advertisedOutputs(detail))" :aria-label="outputLinkAccessibleLabel(failureOutput(detail), advertisedOutputs(detail))" class="text-sm font-medium text-red-900 underline decoration-red-400 underline-offset-2 hover:decoration-red-900" x-text="outputLinkLabel(failureOutput(detail), advertisedOutputs(detail))"></a></p>
                </template>
            </section>

            <section aria-labelledby="job-summary-heading" class="rounded border border-stone-200 bg-white p-4">
                <h2 id="job-summary-heading" class="font-mono text-sm font-semibold text-stone-800">Summary</h2>
                <p x-show="detail.summary === null || detail.summary === undefined || detail.summary === ''" class="mt-2 text-sm text-stone-500">No summary was provided.</p>
                <pre x-show="detail.summary !== null && detail.summary !== undefined && detail.summary !== ''" class="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded bg-stone-50 p-3 text-xs text-stone-800" x-text="typeof detail.summary === 'string' ? detail.summary : JSON.stringify(detail.summary, null, 2)"></pre>
            </section>

            <section aria-labelledby="job-outputs-heading" class="rounded border border-stone-200 bg-white p-4">
                <div class="flex items-center justify-between gap-2">
                    <h2 id="job-outputs-heading" class="font-mono text-sm font-semibold text-stone-800">Outputs</h2>
                    <span class="text-xs text-stone-500" x-text="outputCountText(detail)"></span>
                </div>
                <ul class="mt-3 divide-y divide-stone-200">
                    <template x-for="output in advertisedOutputs(detail)" :key="output.key">
                        <li class="flex flex-wrap items-center justify-between gap-2 py-3">
                            <div class="min-w-0">
                                <p class="break-words text-sm font-medium text-stone-800" x-text="output.label || output.key"></p>
                                <p class="mt-0.5 text-xs text-stone-500"><span x-text="output.type"></span><span> · </span><span x-text="output.availability"></span></p>
                                <p x-show="output.expiresAt" class="mt-0.5 text-xs text-stone-500">Expires <time class="tabular-nums" :datetime="output.expiresAt" x-text="timeText(output.expiresAt)"></time> <span x-text="'(' + relativeText(output.expiresAt) + ')'"></span></p>
                            </div>
                            <template x-if="outputLinkURL(output, advertisedOutputs(detail)) && output.availability === 'available'">
                                <a :href="outputLinkURL(output, advertisedOutputs(detail))" :aria-label="outputLinkAccessibleLabel(output, advertisedOutputs(detail))" class="rounded border border-stone-300 px-3 py-1.5 text-sm text-amber-900 underline decoration-amber-300 underline-offset-2 hover:decoration-amber-900" x-text="outputLinkLabel(output, advertisedOutputs(detail))"></a>
                            </template>
                            <template x-if="outputJSONLinkURL(output, advertisedOutputs(detail)) && output.availability === 'available'">
                                <a :href="outputJSONLinkURL(output, advertisedOutputs(detail))" aria-label="View JSON result" class="rounded border border-stone-300 px-3 py-1.5 text-sm text-amber-900 underline decoration-amber-300 underline-offset-2 hover:decoration-amber-900">View JSON result</a>
                            </template>
                            <span x-show="output.availability !== 'available'" class="text-xs text-stone-600" x-text="output.availability === 'expired' ? 'Expired' : 'Unavailable'"></span>
                        </li>
                    </template>
                    <li x-show="advertisedOutputs(detail).length === 0" class="py-3 text-sm text-stone-500">This job has no published outputs.</li>
                </ul>
            </section>

            <section x-show="detail.lineage" x-cloak aria-labelledby="job-lineage-heading" class="rounded border border-stone-200 bg-white p-4">
                <h2 id="job-lineage-heading" class="font-mono text-sm font-semibold text-stone-800">Related jobs</h2>
                {# A retry chain is Jobs of one title, so each entry says how it is related, its state and when it was accepted. #}
                <div class="mt-3 grid gap-4 sm:grid-cols-2">
                    <template x-for="group in lineageGroups(detail)" :key="group.key">
                        <div :data-job-lineage="group.key">
                            <h3 class="text-xs font-semibold text-stone-600" x-text="group.heading"></h3>
                            <ul class="mt-1 space-y-2">
                                <template x-for="related in group.entries" :key="related.id">
                                    <li class="text-sm text-stone-800">
                                        <span x-show="related.relation" x-text="related.relation + ' '"></span><a :href="detailURL(related)" class="break-words text-amber-900 underline decoration-amber-300 underline-offset-2" x-text="related.name"></a>
                                        <span class="block text-xs text-stone-600"><span x-text="related.state"></span><template x-if="related.accepted"><span> · accepted <time class="tabular-nums" :datetime="related.acceptedAt" x-text="related.accepted"></time></span></template></span>
                                    </li>
                                </template>
                            </ul>
                        </div>
                    </template>
                </div>
                <p x-show="detail.lineage?.retriedElsewhere" x-cloak class="mt-2 text-sm text-stone-700" data-job-retried-elsewhere>Another account has retried this job, so it cannot be retried again. That retry is not visible to you.</p>
                <p x-show="!detail.lineage?.retriedElsewhere && lineageGroups(detail).length === 0" class="mt-2 text-sm text-stone-500">No visible related jobs.</p>
            </section>

            <section aria-labelledby="job-timeline-heading" class="rounded border border-stone-200 bg-white p-4">
                <h2 id="job-timeline-heading" class="font-mono text-sm font-semibold text-stone-800">Timeline</h2>
                <p x-show="timelineError" class="mt-2 text-sm text-stone-600" x-text="timelineError"></p>
                <section x-show="warningEvents().length" x-cloak aria-labelledby="job-warnings-heading" class="mt-3 rounded border border-amber-300 bg-amber-50 p-3">
                    <h3 id="job-warnings-heading" class="font-mono text-sm font-semibold text-amber-950">Warnings</h3>
                    <ul class="mt-2 space-y-2">
                        <template x-for="event in warningEvents()" :key="event.id || event.sequence">
                            <li class="break-words text-sm text-amber-950">
                                <span class="font-medium" x-text="event.type === 'events-truncated' ? 'Earlier event history omitted' : 'Warning'"></span>
                                <time x-show="event.createdAt" class="ml-1 text-xs tabular-nums text-amber-900" :datetime="event.createdAt" x-text="timeText(event.createdAt)"></time>
                                <pre x-show="event.detail" class="mt-1 whitespace-pre-wrap break-words text-xs text-amber-950" x-text="typeof event.detail === 'string' ? event.detail : JSON.stringify(event.detail)"></pre>
                            </li>
                        </template>
                    </ul>
                </section>
                <ol class="mt-3 space-y-3 border-l border-stone-300 pl-4" data-testid="job-timeline">
                    <template x-for="event in timeline" :key="event.id || event.sequence">
                        <li class="relative">
                            <span aria-hidden="true" class="absolute -left-[1.33rem] top-1 h-2 w-2 rounded-full border border-stone-600 bg-white"></span>
                            <p class="text-sm font-medium text-stone-800" x-text="event.type"></p>
                            <time class="text-xs tabular-nums text-stone-500" :datetime="event.createdAt" x-text="timeText(event.createdAt)"></time>
                            <pre x-show="event.detail" class="mt-1 whitespace-pre-wrap break-words text-xs text-stone-600" x-text="typeof event.detail === 'string' ? event.detail : JSON.stringify(event.detail)"></pre>
                        </li>
                    </template>
                    <li x-show="timeline.length === 0" class="text-sm text-stone-500">No timeline events are available.</li>
                </ol>
                {# A Job with more events than one read brings reads on when asked; the latest events are the ones after these. #}
                <div x-show="timelineMore" x-cloak class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-stone-700" data-job-timeline-more>
                    <p>This job has more events than are shown.</p>
                    <button type="button" @click="loadLaterEvents()" class="inline-flex min-h-6 items-center rounded font-medium text-amber-900 underline decoration-amber-300 underline-offset-2 hover:decoration-amber-800 focus:outline-hidden focus:ring-2 focus:ring-amber-700">Show later events</button>
                </div>
            </section>
        </div>
    </template>
</div>
{% endblock %}
