{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data="jobCenter()" data-testid="job-center" class="space-y-5">
    <p class="text-sm text-stone-600">
        Follow background work, open its outputs, and use the controls each job advertises.
        <span class="ml-2 inline-flex items-center gap-1" role="status" aria-live="polite">
            <span aria-hidden="true" class="inline-block h-2 w-2 rounded-full bg-stone-500"></span>
            <span x-text="connectionStatus === 'connected' ? 'Live updates connected' : connectionStatus === 'reconnecting' ? 'Reconnecting to live updates' : 'Loading live updates'"></span>
        </span>
    </p>

    <section aria-label="Job totals" class="grid grid-cols-2 gap-3 sm:max-w-xl">
        <div class="rounded border border-stone-200 bg-white p-3">
            <p class="text-xs font-mono uppercase tracking-wide text-stone-500">Active and scheduled</p>
            <p class="mt-1 text-2xl font-semibold text-stone-900" x-text="summary ? (Number(summary.byState?.scheduled || 0) + Number(summary.byState?.queued || 0) + Number(summary.byState?.running || 0) + Number(summary.byState?.paused || 0)) : '—'"></p>
        </div>
        <div class="rounded border border-stone-200 bg-white p-3">
            <p class="text-xs font-mono uppercase tracking-wide text-stone-500">Needs attention</p>
            <p class="mt-1 text-2xl font-semibold text-stone-900" x-text="summary ? (Number(summary.byState?.blocked || 0) + Number(summary.byState?.failed || 0) + Number(summary.byState?.interrupted || 0)) : '—'"></p>
        </div>
    </section>

    <details class="rounded border border-stone-200 bg-white">
        <summary class="cursor-pointer px-4 py-3 font-mono text-sm font-medium text-stone-800">Filter jobs</summary>
        <form class="grid grid-cols-1 gap-3 border-t border-stone-200 p-4 sm:grid-cols-2 lg:grid-cols-3" aria-label="Filter jobs" @submit.prevent="submitFilters($event.currentTarget)">
            <label class="text-sm text-stone-700 sm:col-span-2 lg:col-span-3">
                Search
                <input name="search" type="search" :value="filters.search" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" placeholder="Title, summary, failure, output" />
            </label>
            <label class="text-sm text-stone-700">Available command
                <input name="command" type="text" :value="filters.command" :disabled="!commandFilterEnabled" aria-describedby="job-command-filter-help" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700 disabled:cursor-not-allowed disabled:bg-stone-100" placeholder="retry, resume" />
                <span id="job-command-filter-help" class="mt-1 block text-xs text-stone-500">Show jobs currently offering this command.</span>
            </label>
            <label class="text-sm text-stone-700">Kinds, comma separated
                <input name="kind" type="text" :value="filters.kinds.join(', ')" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" />
            </label>
            <label class="text-sm text-stone-700">States, comma separated
                <input name="state" type="text" :value="filters.states.join(', ')" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" placeholder="queued, failed" />
            </label>
            <label class="text-sm text-stone-700">Origins, comma separated
                <input name="origin" type="text" :value="filters.origins.join(', ')" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" />
            </label>
            <label class="text-sm text-stone-700">Owner ID
                <input name="ownerId" type="text" inputmode="numeric" :value="filters.ownerId" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" />
            </label>
            <label class="text-sm text-stone-700">Actor ID
                <input name="actorId" type="text" inputmode="numeric" :value="filters.actorId" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" />
            </label>
            <label class="text-sm text-stone-700">Accepted after
                <input name="acceptedAfter" type="datetime-local" :value="dateTimeLocalValue(filters.acceptedAfter)" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" />
            </label>
            <label class="text-sm text-stone-700">Accepted before
                <input name="acceptedBefore" type="datetime-local" :value="dateTimeLocalValue(filters.acceptedBefore)" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700" />
            </label>
            <label class="text-sm text-stone-700">Relationship
                <select name="relationship" :value="filters.relationship" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700">
                    <option value="">Any relationship</option>
                    <option value="retry-of">Retry successor</option>
                    <option value="repeat-of">Repeat successor</option>
                    <option value="parent-child">Parent stage</option>
                </select>
            </label>
            <label class="text-sm text-stone-700">Pinned
                <select name="pinned" :value="filters.pinned === null ? '' : String(filters.pinned)" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700">
                    <option value="">Any</option><option value="true">Pinned</option><option value="false">Not pinned</option>
                </select>
            </label>
            <label class="text-sm text-stone-700">Dismissed
                <select name="dismissed" :value="filters.dismissed === null ? '' : String(filters.dismissed)" class="mt-1 block w-full rounded border-stone-300 text-sm focus:border-amber-700 focus:ring-amber-700">
                    <option value="">Any</option><option value="true">Dismissed</option><option value="false">Not dismissed</option>
                </select>
            </label>
            <div class="flex items-end gap-2 sm:col-span-2 lg:col-span-3">
                <button type="submit" class="rounded bg-amber-800 px-3 py-2 text-sm font-medium text-white hover:bg-amber-900">Apply filters</button>
                <button type="button" @click="clearFilters()" class="rounded border border-stone-300 px-3 py-2 text-sm text-stone-700 hover:bg-stone-50">Clear filters</button>
            </div>
        </form>
    </details>

    <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex gap-2" role="group" aria-label="Job views">
            <button type="button" @click="setView('home')" :aria-pressed="(!isAllView && !hasFilters()).toString()" class="rounded px-3 py-2 text-sm font-medium" :class="!isAllView && !hasFilters() ? 'bg-stone-800 text-white' : 'border border-stone-300 bg-white text-stone-700'">Overview</button>
            <button type="button" @click="setView('all')" :aria-pressed="isAllView.toString()" class="rounded px-3 py-2 text-sm font-medium" :class="isAllView ? 'bg-stone-800 text-white' : 'border border-stone-300 bg-white text-stone-700'">All jobs</button>
        </div>
        <p class="text-sm text-stone-600" x-text="notice"></p>
    </div>

    <div x-show="selectedCount > 0" x-cloak class="rounded border border-amber-300 bg-amber-50 p-3" data-testid="job-bulk-toolbar">
        <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-stone-800" x-text="`${selectedCount} selected`"></span>
            <template x-for="command in bulkCommands()" :key="command.key">
                <button type="button" @click="runBulkCommand(command)" :disabled="bulkBusy" class="rounded border border-amber-800 px-3 py-1.5 text-sm font-medium text-amber-900 hover:bg-amber-100 disabled:opacity-50" x-text="command.label || command.key"></button>
            </template>
            <button type="button" @click="selectedIds = new Set()" class="px-2 py-1.5 text-sm text-stone-700 underline">Clear selection</button>
        </div>
        <ul x-show="bulkOutcomes.length" class="mt-3 space-y-1 border-t border-amber-200 pt-2 text-sm" aria-label="Bulk command outcomes">
            <template x-for="outcome in bulkOutcomes" :key="outcome.jobId">
                <li class="flex flex-wrap gap-x-2"><span class="font-mono" x-text="outcome.jobId"></span><span x-text="outcome.message || outcome.code || outcome.status"></span></li>
            </template>
        </ul>
    </div>

    <p x-show="error" x-cloak role="alert" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-800" x-text="error"></p>
    <p x-show="loading" x-cloak role="status" class="py-6 text-sm text-stone-600">Loading jobs…</p>

    <div x-show="!loading" :aria-busy="loading.toString()" class="space-y-6">
        <template x-for="section in displaySections" :key="section.key">
            <section :aria-labelledby="'jobs-section-' + section.key" class="space-y-2">
                <div class="flex items-baseline justify-between gap-2 border-b border-stone-300 pb-2">
                    <h2 :id="'jobs-section-' + section.key" class="font-mono text-lg font-semibold text-stone-900" x-text="section.title"></h2>
                    <span class="text-xs text-stone-500" x-text="`${section.jobs.length} shown`"></span>
                </div>
                <div class="space-y-2">
                    <template x-for="job in section.jobs" :key="job.id">
                        <article class="min-w-0 rounded border border-stone-200 bg-white p-3 sm:p-4" :data-job-id="job.id">
                            <div class="flex items-start gap-3">
                                <input type="checkbox" class="mt-1 rounded border-stone-400 text-amber-800 focus:ring-amber-700" :checked="selectedIds.has(job.id)" @change="toggleSelection(job, $event.target.checked)" :aria-label="'Select ' + (job.title || job.kind || 'job')" />
                                <div class="min-w-0 flex-1">
                                    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                                        <a :href="detailURL(job)" :aria-label="'Open job ' + (job.title || job.kind || job.id)" class="min-w-0 truncate font-medium text-amber-900 underline decoration-amber-300 underline-offset-2 hover:decoration-amber-900" x-text="job.title || job.kind || job.id"></a>
                                        <span class="inline-flex shrink-0 items-center rounded border border-stone-300 px-2 py-0.5 text-xs font-mono text-stone-700" x-text="stateLabel(job)"></span>
                                    </div>
                                    <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-stone-500">
                                        <span x-text="job.kind"></span>
                                        <span x-show="job.phase" x-text="job.phase"></span>
                                        <time x-show="job.acceptedAt" :datetime="job.acceptedAt" x-text="job.acceptedAt ? new Date(job.acceptedAt).toLocaleString() : ''"></time>
                                    </div>
                                    <p x-show="job.summary" x-cloak class="mt-2 break-words text-sm text-stone-700" x-text="typeof job.summary === 'string' ? job.summary : JSON.stringify(job.summary)"></p>
                                    <template x-if="job.progress">
                                        <div class="mt-3 max-w-xl">
                                            <div class="mb-1 flex justify-between gap-2 text-xs text-stone-600"><span x-text="progressText(job)"></span><span x-text="progressValue(job) === null ? 'In progress' : `${progressValue(job)}%`"></span></div>
                                            <div class="h-2 rounded bg-stone-200" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="progressValue(job)" :aria-valuetext="progressText(job)" :aria-label="'Job progress: ' + progressText(job)">
                                                <div class="h-2 rounded bg-amber-800" :class="progressValue(job) === null ? 'w-full animate-pulse' : ''" :style="progressValue(job) === null ? '' : `width:${progressValue(job)}%`"></div>
                                            </div>
                                        </div>
                                    </template>
                                    <p x-show="job.failure?.message" x-cloak class="mt-2 break-words text-sm text-red-800" x-text="job.failure?.message"></p>
                                    <button type="button" @click="toggleExpanded(job)" class="mt-2 text-xs font-medium text-amber-900 underline decoration-amber-300 underline-offset-2"
                                            :aria-expanded="Boolean(job.uiExpanded).toString()" :aria-controls="'job-details-' + job.id"
                                            x-text="job.uiExpanded ? 'Hide details' : 'Show details'"></button>
                                    <div x-show="job.uiExpanded" x-cloak :id="'job-details-' + job.id" class="mt-2 grid gap-1 border-l-2 border-stone-300 pl-3 text-xs text-stone-600 sm:grid-cols-2">
                                        <p>Accepted <time :datetime="job.acceptedAt" x-text="job.acceptedAt ? new Date(job.acceptedAt).toLocaleString() : 'Unknown'"></time></p>
                                        <p>Started <time :datetime="job.startedAt" x-text="job.startedAt ? new Date(job.startedAt).toLocaleString() : 'Not started'"></time></p>
                                        <p x-show="job.finishedAt">Finished <time :datetime="job.finishedAt" x-text="job.finishedAt ? new Date(job.finishedAt).toLocaleString() : ''"></time></p>
                                        <p>Version <span class="font-mono" x-text="job.version"></span></p>
                                    </div>
                                </div>
                                <a :href="detailURL(job)" :aria-label="'Open job ' + (job.title || job.kind || job.id)" class="shrink-0 rounded border border-stone-300 px-2 py-1 text-xs font-medium text-stone-700 hover:bg-stone-50">Open</a>
                            </div>
                        </article>
                    </template>
                    <p x-show="section.jobs.length === 0" class="rounded border border-dashed border-stone-300 bg-white p-4 text-sm text-stone-600">No jobs in this section.</p>
                </div>
            </section>
        </template>
        <div x-show="isAllView && nextCursor" class="py-2 text-center">
            <button type="button" @click="loadMore()" :disabled="loadingMore" class="rounded border border-stone-300 bg-white px-4 py-2 text-sm font-medium text-stone-800 hover:bg-stone-50 disabled:opacity-50" x-text="loadingMore ? 'Loading…' : 'Load more jobs'"></button>
        </div>
    </div>
</div>
{% endblock %}
