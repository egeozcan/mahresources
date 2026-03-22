{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data="adminOverview()" x-init="init()" @destroy="destroy()" class="space-y-6">

    {# ── 1. Server Health ──────────────────────────────────────────── #}
    <section
        aria-label="Server health"
        aria-live="polite"
        aria-atomic="true"
        class="rounded-lg bg-amber-50 border border-amber-200 p-5"
    >
        <h2 class="text-base font-semibold font-mono text-amber-900 mb-4">Server Health</h2>

        <template x-if="serverStatsLoading && !serverStats">
            <p class="text-sm text-amber-700 font-mono" role="status">Loading server stats&hellip;</p>
        </template>

        <template x-if="serverStatsError && !serverStats">
            <p class="text-sm text-red-700 font-mono" role="alert" x-text="serverStatsError"></p>
        </template>

        <template x-if="serverStats">
            <dl class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-x-6 gap-y-3 text-sm font-mono">
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Uptime</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.uptime"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Heap Alloc</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.heapAllocFmt"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Sys Memory</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.sysFmt"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">GC Runs</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.numGC"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Goroutines</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.goroutines"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Go Version</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.goVersion"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">DB Type</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.dbType"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">DB Size</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.dbFileSizeFmt || '—'"></dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">DB Connections</dt>
                    <dd class="text-stone-900 font-medium mt-0.5">
                        <span x-text="serverStats.dbInUse"></span> in use /
                        <span x-text="serverStats.dbOpenConns"></span> open
                    </dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Hash Workers</dt>
                    <dd class="text-stone-900 font-medium mt-0.5">
                        <template x-if="serverStats.hashWorkerEnabled">
                            <span x-text="serverStats.hashWorkerCount + ' active'"></span>
                        </template>
                        <template x-if="!serverStats.hashWorkerEnabled">
                            <span class="text-stone-500">Disabled</span>
                        </template>
                    </dd>
                </div>
                <div>
                    <dt class="text-amber-700 text-xs uppercase tracking-wider">Downloads Queued</dt>
                    <dd class="text-stone-900 font-medium mt-0.5" x-text="serverStats.downloadQueueLength"></dd>
                </div>
            </dl>
        </template>
    </section>

    {# ── 2. Configuration ──────────────────────────────────────────── #}
    <section aria-label="Configuration" class="rounded-lg bg-white border border-stone-200 p-5">
        <h2 class="text-base font-semibold font-mono text-stone-800 mb-4">Configuration</h2>

        <template x-if="dataStatsLoading && !dataStats">
            <p class="text-sm text-stone-500 font-mono" role="status">Loading configuration&hellip;</p>
        </template>

        <template x-if="dataStatsError && !dataStats">
            <p class="text-sm text-red-700 font-mono" role="alert" x-text="dataStatsError"></p>
        </template>

        <template x-if="dataStats && dataStats.config">
            <dl class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-8 gap-y-3 text-sm font-mono">
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Bind Address</dt>
                    <dd class="text-stone-900 font-medium mt-0.5 break-all" x-text="dataStats.config.bindAddress || '—'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Storage Path</dt>
                    <dd class="text-stone-900 font-medium mt-0.5 break-all" x-text="dataStats.config.fileSavePath || '—'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Database</dt>
                    <dd class="text-stone-900 font-medium mt-0.5 break-all">
                        <span x-text="dataStats.config.dbType"></span>
                        <template x-if="dataStats.config.dbDsn">
                            <span class="text-stone-500"> — <span x-text="dataStats.config.dbDsn"></span></span>
                        </template>
                    </dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Read-only DB</dt>
                    <dd class="mt-0.5" :class="dataStats.config.hasReadOnlyDB ? 'text-green-700' : 'text-stone-500'"
                        x-text="dataStats.config.hasReadOnlyDB ? 'Enabled' : 'Disabled'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">FFmpeg</dt>
                    <dd class="mt-0.5" :class="dataStats.config.ffmpegAvailable ? 'text-green-700' : 'text-stone-500'"
                        x-text="dataStats.config.ffmpegAvailable ? 'Enabled' : 'Disabled'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">LibreOffice</dt>
                    <dd class="mt-0.5" :class="dataStats.config.libreOfficeAvailable ? 'text-green-700' : 'text-stone-500'"
                        x-text="dataStats.config.libreOfficeAvailable ? 'Enabled' : 'Disabled'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Full-text Search</dt>
                    <dd class="mt-0.5" :class="dataStats.config.ftsEnabled ? 'text-green-700' : 'text-stone-500'"
                        x-text="dataStats.config.ftsEnabled ? 'Enabled' : 'Disabled'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Hash Workers</dt>
                    <dd class="mt-0.5" :class="dataStats.config.hashWorkerEnabled ? 'text-green-700' : 'text-stone-500'">
                        <span x-text="dataStats.config.hashWorkerEnabled ? 'Enabled' : 'Disabled'"></span>
                        <template x-if="dataStats.config.hashWorkerEnabled">
                            <span class="text-stone-500"> (<span x-text="dataStats.config.hashWorkerCount"></span> workers)</span>
                        </template>
                    </dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Alt Filesystems</dt>
                    <dd class="text-stone-900 font-medium mt-0.5">
                        <template x-if="dataStats.config.altFileSystems && dataStats.config.altFileSystems.length > 0">
                            <span x-text="dataStats.config.altFileSystems.join(', ')"></span>
                        </template>
                        <template x-if="!dataStats.config.altFileSystems || dataStats.config.altFileSystems.length === 0">
                            <span class="text-stone-500">None</span>
                        </template>
                    </dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Ephemeral</dt>
                    <dd class="mt-0.5" :class="dataStats.config.ephemeralMode ? 'text-amber-700' : 'text-stone-500'"
                        x-text="dataStats.config.ephemeralMode ? 'Enabled' : 'Disabled'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Memory DB</dt>
                    <dd class="mt-0.5" :class="dataStats.config.memoryDb ? 'text-amber-700' : 'text-stone-500'"
                        x-text="dataStats.config.memoryDb ? 'Enabled' : 'Disabled'"></dd>
                </div>
                <div>
                    <dt class="text-stone-500 text-xs uppercase tracking-wider">Memory FS</dt>
                    <dd class="mt-0.5" :class="dataStats.config.memoryFs ? 'text-amber-700' : 'text-stone-500'"
                        x-text="dataStats.config.memoryFs ? 'Enabled' : 'Disabled'"></dd>
                </div>
            </dl>
        </template>
    </section>

    {# ── 3. Data Overview ──────────────────────────────────────────── #}
    <section aria-label="Data overview" class="rounded-lg bg-white border border-stone-200 p-5">
        <h2 class="text-base font-semibold font-mono text-stone-800 mb-4">Data Overview</h2>

        <template x-if="dataStatsLoading && !dataStats">
            <p class="text-sm text-stone-500 font-mono" role="status">Loading data overview&hellip;</p>
        </template>

        <template x-if="dataStatsError && !dataStats">
            <p class="text-sm text-red-700 font-mono" role="alert" x-text="dataStatsError"></p>
        </template>

        <template x-if="dataStats">
            <div class="space-y-5">
                {# Storage summary cards #}
                <div class="grid grid-cols-2 gap-4">
                    <div class="rounded-md bg-stone-50 border border-stone-200 p-4">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Total Storage</p>
                        <p class="text-2xl font-bold font-mono text-stone-900 mt-1" x-text="dataStats.storageTotalFmt"></p>
                    </div>
                    <div class="rounded-md bg-stone-50 border border-stone-200 p-4">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Version Storage</p>
                        <p class="text-2xl font-bold font-mono text-stone-900 mt-1" x-text="dataStats.totalVersionStorageFormatted"></p>
                    </div>
                </div>

                {# Entity count cards #}
                <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3" aria-label="Entity counts">
                    <a href="/resources" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Resources</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.resources)"></p>
                        <template x-if="dataStats.growth && dataStats.growth.last7Days">
                            <p class="text-xs text-stone-500 mt-0.5">+<span x-text="formatNumber(dataStats.growth.last7Days.resources)"></span> this week</p>
                        </template>
                    </a>
                    <a href="/notes" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Notes</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.notes)"></p>
                        <template x-if="dataStats.growth && dataStats.growth.last7Days">
                            <p class="text-xs text-stone-500 mt-0.5">+<span x-text="formatNumber(dataStats.growth.last7Days.notes)"></span> this week</p>
                        </template>
                    </a>
                    <a href="/groups" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Groups</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.groups)"></p>
                        <template x-if="dataStats.growth && dataStats.growth.last7Days">
                            <p class="text-xs text-stone-500 mt-0.5">+<span x-text="formatNumber(dataStats.growth.last7Days.groups)"></span> this week</p>
                        </template>
                    </a>
                    <a href="/tags" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Tags</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.tags)"></p>
                    </a>
                    <a href="/categories" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Categories</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.categories)"></p>
                    </a>
                    <a href="/resource-categories" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Resource Categories</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.resourceCategories)"></p>
                    </a>
                    <a href="/note-types" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Note Types</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.noteTypes)"></p>
                    </a>
                    <a href="/queries" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Queries</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.queries)"></p>
                    </a>
                    <a href="/relations" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Relations</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.relations)"></p>
                    </a>
                    <a href="/relation-types" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Relation Types</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.relationTypes)"></p>
                    </a>
                    <a href="/logs" class="rounded-md bg-stone-50 border border-stone-200 p-3 hover:bg-amber-50 hover:border-amber-300 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Log Entries</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.logEntries)"></p>
                    </a>
                    <div class="rounded-md bg-stone-50 border border-stone-200 p-3">
                        <p class="text-xs font-mono text-stone-500 uppercase tracking-wider">Resource Versions</p>
                        <p class="text-xl font-bold font-mono text-stone-900 mt-0.5" x-text="formatNumber(dataStats.entities.resourceVersions)"></p>
                    </div>
                </div>
            </div>
        </template>
    </section>

    {# ── 4. Detailed Statistics ─────────────────────────────────────── #}
    <section aria-label="Detailed statistics" class="rounded-lg bg-white border border-stone-200 p-5">
        <h2 class="text-base font-semibold font-mono text-stone-800 mb-4">Detailed Statistics</h2>

        <template x-if="expensiveStatsLoading && !expensiveStats">
            <div class="flex items-center gap-2 text-sm text-stone-500 font-mono" role="status">
                <svg class="animate-spin h-4 w-4 text-amber-600" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" aria-hidden="true">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                </svg>
                Computing detailed statistics&hellip;
            </div>
        </template>

        <template x-if="expensiveStatsError && !expensiveStats">
            <p class="text-sm text-red-700 font-mono" role="alert" x-text="expensiveStatsError"></p>
        </template>

        <template x-if="expensiveStats">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">

                {# Storage by Content Type #}
                <div>
                    <h3 class="text-sm font-semibold font-mono text-stone-700 mb-2">Storage by Content Type</h3>
                    <template x-if="expensiveStats.storageByContentType && expensiveStats.storageByContentType.length > 0">
                        <div class="overflow-x-auto">
                            <table class="min-w-full text-sm font-mono">
                                <thead>
                                    <tr class="border-b border-stone-200">
                                        <th class="py-1 pr-4 text-left text-xs text-stone-500 uppercase tracking-wider font-medium" scope="col">Type</th>
                                        <th class="py-1 pr-4 text-right text-xs text-stone-500 uppercase tracking-wider font-medium" scope="col">Size</th>
                                        <th class="py-1 text-right text-xs text-stone-500 uppercase tracking-wider font-medium" scope="col">Count</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <template x-for="row in expensiveStats.storageByContentType" :key="row.contentType">
                                        <tr class="border-b border-stone-100">
                                            <td class="py-1 pr-4 text-stone-700 break-all" x-text="row.contentType || '(none)'"></td>
                                            <td class="py-1 pr-4 text-right text-stone-900 whitespace-nowrap" x-text="row.totalFmt"></td>
                                            <td class="py-1 text-right text-stone-500" x-text="formatNumber(row.count)"></td>
                                        </tr>
                                    </template>
                                </tbody>
                            </table>
                        </div>
                    </template>
                    <template x-if="!expensiveStats.storageByContentType || expensiveStats.storageByContentType.length === 0">
                        <p class="text-sm text-stone-500 font-mono">No data.</p>
                    </template>
                </div>

                {# Top Tags #}
                <div>
                    <h3 class="text-sm font-semibold font-mono text-stone-700 mb-2">Top Tags</h3>
                    <template x-if="expensiveStats.topTags && expensiveStats.topTags.length > 0">
                        <ol class="space-y-1">
                            <template x-for="tag in expensiveStats.topTags" :key="tag.id">
                                <li class="flex items-center justify-between text-sm font-mono">
                                    <a :href="'/tag?id=' + tag.id" class="text-amber-700 hover:underline truncate mr-2" x-text="tag.name"></a>
                                    <span class="text-stone-500 whitespace-nowrap" x-text="formatNumber(tag.count) + ' resources'"></span>
                                </li>
                            </template>
                        </ol>
                    </template>
                    <template x-if="!expensiveStats.topTags || expensiveStats.topTags.length === 0">
                        <p class="text-sm text-stone-500 font-mono">No tags.</p>
                    </template>
                </div>

                {# Top Categories #}
                <div>
                    <h3 class="text-sm font-semibold font-mono text-stone-700 mb-2">Top Categories</h3>
                    <template x-if="expensiveStats.topCategories && expensiveStats.topCategories.length > 0">
                        <ol class="space-y-1">
                            <template x-for="cat in expensiveStats.topCategories" :key="cat.id">
                                <li class="flex items-center justify-between text-sm font-mono">
                                    <a :href="'/category?id=' + cat.id" class="text-amber-700 hover:underline truncate mr-2" x-text="cat.name"></a>
                                    <span class="text-stone-500 whitespace-nowrap" x-text="formatNumber(cat.count) + ' groups'"></span>
                                </li>
                            </template>
                        </ol>
                    </template>
                    <template x-if="!expensiveStats.topCategories || expensiveStats.topCategories.length === 0">
                        <p class="text-sm text-stone-500 font-mono">No categories.</p>
                    </template>
                </div>

                {# Orphaned Resources #}
                <div>
                    <h3 class="text-sm font-semibold font-mono text-stone-700 mb-2">Orphaned Resources</h3>
                    <template x-if="expensiveStats.orphans">
                        <dl class="space-y-2 text-sm font-mono">
                            <div class="flex items-center justify-between">
                                <dt class="text-stone-500">Without tags</dt>
                                <dd role="status" :class="expensiveStats.orphans.withoutTags > 0 ? 'text-amber-700 font-semibold' : 'text-stone-900'"
                                    x-text="formatNumber(expensiveStats.orphans.withoutTags)"></dd>
                            </div>
                            <div class="flex items-center justify-between">
                                <dt class="text-stone-500">Without groups</dt>
                                <dd role="status" :class="expensiveStats.orphans.withoutGroups > 0 ? 'text-amber-700 font-semibold' : 'text-stone-900'"
                                    x-text="formatNumber(expensiveStats.orphans.withoutGroups)"></dd>
                            </div>
                        </dl>
                    </template>
                </div>

                {# Similarity Detection #}
                <div>
                    <h3 class="text-sm font-semibold font-mono text-stone-700 mb-2">Similarity Detection</h3>
                    <template x-if="expensiveStats.similarity">
                        <dl class="space-y-2 text-sm font-mono">
                            <div class="flex items-center justify-between">
                                <dt class="text-stone-500">Hashed images</dt>
                                <dd class="text-stone-900" x-text="formatNumber(expensiveStats.similarity.totalHashes)"></dd>
                            </div>
                            <div class="flex items-center justify-between">
                                <dt class="text-stone-500">Similar pairs found</dt>
                                <dd class="text-stone-900" x-text="formatNumber(expensiveStats.similarity.similarPairsFound)"></dd>
                            </div>
                        </dl>
                    </template>
                </div>

                {# Log Statistics #}
                <div>
                    <h3 class="text-sm font-semibold font-mono text-stone-700 mb-2">Log Statistics</h3>
                    <template x-if="expensiveStats.logStats">
                        <dl class="space-y-2 text-sm font-mono">
                            <div class="flex items-center justify-between">
                                <dt class="text-stone-500">Total entries</dt>
                                <dd class="text-stone-900" x-text="formatNumber(expensiveStats.logStats.totalEntries)"></dd>
                            </div>
                            <template x-if="expensiveStats.logStats.byLevel">
                                <template x-for="[level, count] in Object.entries(expensiveStats.logStats.byLevel)" :key="level">
                                    <div class="flex items-center justify-between">
                                        <dt class="text-stone-500" x-text="level"></dt>
                                        <dd class="text-stone-900" x-text="formatNumber(count)"></dd>
                                    </div>
                                </template>
                            </template>
                            <div class="flex items-center justify-between border-t border-stone-100 pt-2 mt-1">
                                <dt class="text-stone-500">Errors (last 24h)</dt>
                                <dd role="status" :class="expensiveStats.logStats.recentErrors > 0 ? 'text-red-700 font-semibold' : 'text-stone-900'"
                                    x-text="formatNumber(expensiveStats.logStats.recentErrors)"></dd>
                            </div>
                        </dl>
                    </template>
                </div>

            </div>
        </template>
    </section>

</div>
{% endblock %}
