{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data="jobCenter()" data-testid="job-detail" class="mx-auto max-w-5xl space-y-5">
    <p x-show="loading" x-cloak role="status" class="py-6 text-sm text-stone-600">Loading job…</p>
    <p x-show="error" x-cloak role="alert" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-800" x-text="error"></p>
    <template x-if="detail && !loading">
        <div class="space-y-6">
            <header class="flex flex-wrap items-start justify-between gap-3 border-b border-stone-300 pb-4">
                <div class="min-w-0">
                    <a href="/jobs" class="text-sm text-amber-900 underline decoration-amber-300 underline-offset-2">All jobs</a>
                    <h1 class="mt-2 break-words text-2xl font-semibold text-stone-900" x-text="detail.title || detail.kind || detail.id"></h1>
                    <div class="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-sm text-stone-600">
                        <span class="rounded border border-stone-300 px-2 py-0.5 font-mono" x-text="stateLabel(detail)"></span>
                        <span x-show="detail.pinned" x-cloak class="inline-flex items-center rounded border border-amber-400 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-900">Pinned by you</span>
                        <span x-text="detail.kind"></span>
                        <span x-show="detail.phase" x-text="detail.phase"></span>
                        <span class="font-mono text-xs" x-text="detail.id"></span>
                    </div>
                </div>
                <div class="flex flex-wrap gap-2" role="group" aria-label="Advertised job commands">
                    <template x-for="command in commandsFor(detail)" :key="command.key">
                        <button type="button" @click="runCommand(detail, command)" class="rounded border border-stone-400 bg-white px-3 py-2 text-sm font-medium text-stone-800 hover:bg-stone-50" x-text="command.label || command.key"></button>
                    </template>
                </div>
            </header>

            <section aria-labelledby="job-context-heading" class="rounded border border-stone-200 bg-white p-4">
                <h2 id="job-context-heading" class="font-mono text-sm font-semibold text-stone-800">Job context</h2>
                <dl class="mt-2 grid gap-2 text-sm sm:grid-cols-3">
                    <div x-show="detail.ownerUserId !== null && detail.ownerUserId !== undefined">
                        <dt class="text-xs text-stone-500">Owner</dt>
                        <dd class="break-all font-mono text-stone-800" x-text="detail.ownerUserId"></dd>
                    </div>
                    <div x-show="detail.actorUserId !== null && detail.actorUserId !== undefined">
                        <dt class="text-xs text-stone-500">Actor</dt>
                        <dd class="break-all font-mono text-stone-800" x-text="detail.actorUserId"></dd>
                    </div>
                    <div x-show="detail.origin">
                        <dt class="text-xs text-stone-500">Origin</dt>
                        <dd class="break-words text-stone-800" x-text="detail.origin"></dd>
                    </div>
                </dl>
            </section>

            <p x-show="notice" x-cloak class="rounded border border-stone-200 bg-white p-3 text-sm text-stone-800" x-text="notice"></p>

            <template x-if="detail.progress">
                <section aria-labelledby="job-progress-heading" class="rounded border border-stone-200 bg-white p-4">
                    <h2 id="job-progress-heading" class="font-mono text-sm font-semibold text-stone-800">Progress</h2>
                    <p class="mt-2 text-sm text-stone-700" x-text="progressText(detail)"></p>
                    <div class="mt-2 h-2 rounded bg-stone-200" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="progressValue(detail)" :aria-valuetext="progressAccessibleText(detail)" :aria-label="(detail.title || detail.kind || 'Job') + ' progress: ' + progressAccessibleText(detail)">
                        <div class="h-2 rounded bg-amber-800" :class="progressIndeterminate(detail) ? 'w-full animate-pulse' : ''" :style="progressIndeterminate(detail) ? '' : `width:${progressValue(detail) ?? 0}%`"></div>
                    </div>
                </section>
            </template>

            <section x-show="detail.failure?.message" x-cloak aria-labelledby="job-failure-heading" class="rounded border border-red-300 bg-red-50 p-4">
                <h2 id="job-failure-heading" class="font-mono text-sm font-semibold text-red-900">Failure</h2>
                <p class="mt-2 whitespace-pre-wrap break-words text-sm text-red-900" x-text="detail.failure?.message"></p>
                <p x-show="detail.failure?.class" class="mt-2 text-xs text-red-800" x-text="'Class: ' + detail.failure?.class"></p>
            </section>

            <section aria-labelledby="job-summary-heading" class="rounded border border-stone-200 bg-white p-4">
                <h2 id="job-summary-heading" class="font-mono text-sm font-semibold text-stone-800">Summary</h2>
                <p x-show="detail.summary === null || detail.summary === undefined || detail.summary === ''" class="mt-2 text-sm text-stone-500">No summary was provided.</p>
                <pre x-show="detail.summary !== null && detail.summary !== undefined && detail.summary !== ''" class="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded bg-stone-50 p-3 text-xs text-stone-800" x-text="typeof detail.summary === 'string' ? detail.summary : JSON.stringify(detail.summary, null, 2)"></pre>
            </section>

            <section aria-labelledby="job-outputs-heading" class="rounded border border-stone-200 bg-white p-4">
                <div class="flex items-center justify-between gap-2">
                    <h2 id="job-outputs-heading" class="font-mono text-sm font-semibold text-stone-800">Outputs</h2>
                    <span class="text-xs text-stone-500" x-text="`${advertisedOutputs(detail).length} available records`"></span>
                </div>
                <ul class="mt-3 divide-y divide-stone-200">
                    <template x-for="output in advertisedOutputs(detail)" :key="output.key">
                        <li class="flex flex-wrap items-center justify-between gap-2 py-3">
                            <div class="min-w-0">
                                <p class="break-words text-sm font-medium text-stone-800" x-text="output.label || output.key"></p>
                                <p class="mt-0.5 text-xs text-stone-500"><span x-text="output.type"></span><span> · </span><span x-text="output.availability"></span></p>
                                <time x-show="output.expiresAt" class="mt-0.5 block text-xs text-stone-500" :datetime="output.expiresAt" x-text="output.expiresAt ? 'Expires ' + new Date(output.expiresAt).toLocaleString() : ''"></time>
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
                <div class="mt-3 grid gap-4 sm:grid-cols-2">
                    <template x-for="relation in ['ancestors', 'successors', 'parents', 'children']" :key="relation">
                        <div x-show="detail.lineage?.[relation]?.length">
                            <h3 class="text-xs font-mono uppercase tracking-wide text-stone-500" x-text="relation"></h3>
                            <ul class="mt-1 space-y-1">
                                <template x-for="related in (detail.lineage?.[relation] || [])" :key="related.id">
                                    <li><a :href="detailURL(related)" class="text-sm text-amber-900 underline decoration-amber-300 underline-offset-2" x-text="related.title || related.kind || related.id"></a></li>
                                </template>
                            </ul>
                        </div>
                    </template>
                </div>
                <p x-show="!['ancestors', 'successors', 'parents', 'children'].some(key => detail.lineage?.[key]?.length)" class="mt-2 text-sm text-stone-500">No visible related jobs.</p>
            </section>

            <section aria-labelledby="job-timeline-heading" class="rounded border border-stone-200 bg-white p-4">
                <div class="flex items-baseline justify-between gap-2">
                    <h2 id="job-timeline-heading" class="font-mono text-sm font-semibold text-stone-800">Timeline</h2>
                    <span class="text-xs text-stone-500">Earlier events are not announced again.</span>
                </div>
                <p x-show="timelineError" class="mt-2 text-sm text-stone-600" x-text="timelineError"></p>
                <section x-show="warningEvents().length" x-cloak aria-labelledby="job-warnings-heading" class="mt-3 rounded border border-amber-300 bg-amber-50 p-3">
                    <h3 id="job-warnings-heading" class="font-mono text-sm font-semibold text-amber-950">Warnings</h3>
                    <ul class="mt-2 space-y-2">
                        <template x-for="event in warningEvents()" :key="event.id || event.sequence">
                            <li class="break-words text-sm text-amber-950">
                                <span class="font-medium" x-text="event.type === 'events-truncated' ? 'Earlier event history omitted' : 'Warning'"></span>
                                <time x-show="event.createdAt" class="ml-1 text-xs text-amber-900" :datetime="event.createdAt" x-text="event.createdAt ? new Date(event.createdAt).toLocaleString() : ''"></time>
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
                            <time class="text-xs text-stone-500" :datetime="event.createdAt" x-text="event.createdAt ? new Date(event.createdAt).toLocaleString() : ''"></time>
                            <pre x-show="event.detail" class="mt-1 whitespace-pre-wrap break-words text-xs text-stone-600" x-text="typeof event.detail === 'string' ? event.detail : JSON.stringify(event.detail)"></pre>
                        </li>
                    </template>
                    <li x-show="timeline.length === 0" class="text-sm text-stone-500">No timeline events are available.</li>
                </ol>
            </section>
        </div>
    </template>
</div>
{% endblock %}
