{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data="mrqlEditor()" x-cloak class="space-y-4">

    {# ── Editor Section ─────────────────────────────────────────────── #}
    <section aria-label="MRQL query editor">
        <div class="flex items-center justify-between mb-2">
            <h2 class="text-base font-semibold font-mono text-stone-800">Query</h2>
            <div class="flex items-center gap-2">
                <button type="button"
                        @click="showDocs = !showDocs"
                        class="text-sm text-amber-700 hover:text-amber-900 font-mono flex items-center gap-1 cursor-pointer"
                        :aria-expanded="showDocs.toString()"
                        aria-controls="mrql-docs-panel">
                    <svg :class="showDocs && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                    </svg>
                    Docs
                </button>
            </div>
        </div>

        {# Syntax help panel #}
        <div x-show="showDocs" x-collapse id="mrql-docs-panel"
             class="mb-4 text-sm text-stone-600 bg-stone-50 border border-stone-200 rounded-md p-4 space-y-3 font-sans">
            <div>
                <h3 class="font-semibold text-stone-700">Syntax Overview</h3>
                <p>MRQL queries filter entities using field-value conditions connected with <code class="bg-stone-200 px-1 rounded">AND</code> / <code class="bg-stone-200 px-1 rounded">OR</code>.</p>
                <pre class="bg-stone-100 p-2 rounded mt-1 overflow-x-auto">name ~ "search term" AND tags = "important" ORDER BY created DESC LIMIT 20</pre>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Entity Types</h3>
                <p>Filter by entity type with <code class="bg-stone-200 px-1 rounded">type = resource|note|group</code>. Omit for cross-entity search.</p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Common Fields</h3>
                <p class="text-xs">
                    <code class="bg-stone-200 px-1 rounded">id</code>,
                    <code class="bg-stone-200 px-1 rounded">name</code>,
                    <code class="bg-stone-200 px-1 rounded">description</code>,
                    <code class="bg-stone-200 px-1 rounded">created</code>,
                    <code class="bg-stone-200 px-1 rounded">updated</code>,
                    <code class="bg-stone-200 px-1 rounded">tags</code>,
                    <code class="bg-stone-200 px-1 rounded">meta.*</code>
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Resource Fields</h3>
                <p class="text-xs">
                    <code class="bg-stone-200 px-1 rounded">groups</code>,
                    <code class="bg-stone-200 px-1 rounded">category</code>,
                    <code class="bg-stone-200 px-1 rounded">contentType</code>,
                    <code class="bg-stone-200 px-1 rounded">fileSize</code>,
                    <code class="bg-stone-200 px-1 rounded">width</code>,
                    <code class="bg-stone-200 px-1 rounded">height</code>,
                    <code class="bg-stone-200 px-1 rounded">originalName</code>,
                    <code class="bg-stone-200 px-1 rounded">hash</code>
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Note Fields</h3>
                <p class="text-xs">
                    <code class="bg-stone-200 px-1 rounded">groups</code>,
                    <code class="bg-stone-200 px-1 rounded">noteType</code>
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Group Fields</h3>
                <p class="text-xs">
                    <code class="bg-stone-200 px-1 rounded">category</code>,
                    <code class="bg-stone-200 px-1 rounded">parent</code>,
                    <code class="bg-stone-200 px-1 rounded">children</code>
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Operators</h3>
                <p class="text-xs">
                    <code class="bg-stone-200 px-1 rounded">=</code> equal,
                    <code class="bg-stone-200 px-1 rounded">!=</code> not equal,
                    <code class="bg-stone-200 px-1 rounded">~</code> contains,
                    <code class="bg-stone-200 px-1 rounded">!~</code> not contains,
                    <code class="bg-stone-200 px-1 rounded">&gt;</code> <code class="bg-stone-200 px-1 rounded">&gt;=</code> <code class="bg-stone-200 px-1 rounded">&lt;</code> <code class="bg-stone-200 px-1 rounded">&lt;=</code> comparison,
                    <code class="bg-stone-200 px-1 rounded">IS EMPTY</code>,
                    <code class="bg-stone-200 px-1 rounded">IS NULL</code>,
                    <code class="bg-stone-200 px-1 rounded">IN ("a", "b")</code>
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Examples</h3>
                <pre class="bg-stone-100 p-2 rounded overflow-x-auto">type = resource AND contentType ~ "image" AND fileSize > 1MB</pre>
                <pre class="bg-stone-100 p-2 rounded overflow-x-auto mt-1">type = note AND tags = "todo" ORDER BY updated DESC</pre>
                <pre class="bg-stone-100 p-2 rounded overflow-x-auto mt-1">name ~ "project" AND created > -30d</pre>
            </div>
        </div>

        {# Editor container #}
        <div x-ref="editorContainer"
             class="border border-stone-300 rounded-md overflow-hidden bg-white"></div>

        {# Validation status #}
        <div class="mt-1 min-h-[1.25rem]">
            <template x-if="validationError">
                <p class="text-sm text-red-600 font-mono" role="alert" x-text="validationError"></p>
            </template>
        </div>

        {# Action buttons #}
        <div class="flex items-center gap-2 mt-2">
            <button type="button"
                    @click="execute()"
                    :disabled="executing"
                    class="inline-flex items-center px-4 py-2 border border-stone-300 rounded-md shadow-sm text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer">
                <template x-if="executing">
                    <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" aria-hidden="true">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                    </svg>
                </template>
                <span x-text="executing ? 'Running...' : 'Run'"></span>
                <kbd class="ml-2 text-xs" aria-hidden="true" x-text="navigator.platform.indexOf('Mac') > -1 ? '⌘↵' : 'Ctrl+Enter'"></kbd>
            </button>
            <button type="button"
                    @click="showSaveDialog = true"
                    class="inline-flex items-center px-4 py-2 border border-stone-300 rounded-md shadow-sm text-sm font-mono font-medium text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 cursor-pointer">
                Save
            </button>
        </div>
    </section>

    {# ── Save Dialog ─────────────────────────────────────────────── #}
    <template x-if="showSaveDialog">
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40" @click.self="showSaveDialog = false" @keydown.escape.window="showSaveDialog = false">
            <div class="bg-white rounded-lg shadow-xl p-6 w-full max-w-md mx-4 space-y-4" role="dialog" aria-modal="true" aria-label="Save MRQL query" @click.stop x-trap.noscroll="showSaveDialog">
                <h3 class="text-lg font-semibold font-mono text-stone-800">Save Query</h3>
                <div>
                    <label for="mrql-save-name" class="block text-sm font-medium text-stone-700 mb-1">Name</label>
                    <input type="text" id="mrql-save-name" x-model="saveName"
                           class="w-full border border-stone-300 rounded-md px-3 py-2 text-sm focus:ring-amber-600 focus:border-amber-600"
                           placeholder="My query" />
                </div>
                <div>
                    <label for="mrql-save-desc" class="block text-sm font-medium text-stone-700 mb-1">Description (optional)</label>
                    <input type="text" id="mrql-save-desc" x-model="saveDescription"
                           class="w-full border border-stone-300 rounded-md px-3 py-2 text-sm focus:ring-amber-600 focus:border-amber-600"
                           placeholder="What does this query do?" />
                </div>
                <template x-if="saveError">
                    <p class="text-sm text-red-600 font-mono" role="alert" x-text="saveError"></p>
                </template>
                <div class="flex justify-end gap-2">
                    <button type="button" @click="showSaveDialog = false"
                            class="px-4 py-2 text-sm font-mono text-stone-700 hover:bg-stone-100 rounded-md cursor-pointer">
                        Cancel
                    </button>
                    <button type="button" @click="saveQuery()"
                            :disabled="!saveName.trim()"
                            class="px-4 py-2 text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 rounded-md disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 cursor-pointer">
                        Save
                    </button>
                </div>
            </div>
        </div>
    </template>

    {# ── Results Section ─────────────────────────────────────────── #}
    <section aria-label="Query results">
        <template x-if="error">
            <div class="rounded-md bg-red-50 p-4" role="alert">
                <div class="flex">
                    <div class="flex-shrink-0">
                        <svg class="h-5 w-5 text-red-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                        </svg>
                    </div>
                    <div class="ml-3">
                        <h3 class="text-sm font-medium text-red-800" x-text="error"></h3>
                    </div>
                </div>
            </div>
        </template>

        <template x-if="result && !error">
            <div class="space-y-3">
                <div class="flex items-center justify-between">
                    <h2 class="text-base font-semibold font-mono text-stone-800">
                        Results
                        <span class="text-sm font-normal text-stone-500" x-text="'(' + totalCount + ' items)'"></span>
                    </h2>
                    <span class="text-xs text-stone-500 font-mono" x-text="'Entity: ' + (result.entityType || 'mixed')"></span>
                </div>

                {# Resource results #}
                <template x-if="result.resources && result.resources.length > 0">
                    <div>
                        <h3 class="text-sm font-semibold font-mono text-amber-800 mb-2" x-show="result.entityType === 'mixed'">Resources</h3>
                        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                            <template x-for="entity in result.resources" :key="entity.ID">
                                <a :href="'/resource?id=' + entity.ID"
                                   class="block p-3 bg-white border border-stone-200 rounded-md hover:border-amber-400 hover:shadow-sm transition-colors">
                                    <div class="flex items-start gap-2">
                                        <template x-if="entity.ContentType && entity.ContentType.startsWith('image/')">
                                            <img :src="'/v1/resource/preview?id=' + entity.ID + '&width=96&height=96'" :alt="entity.Name" class="w-12 h-12 rounded object-cover flex-shrink-0" loading="lazy" />
                                        </template>
                                        <div class="min-w-0 flex-1">
                                            <p class="text-sm font-medium text-stone-900 truncate" x-text="entity.Name"></p>
                                            <p class="text-xs text-stone-500 mt-0.5" x-text="entity.ContentType || ''"></p>
                                        </div>
                                    </div>
                                </a>
                            </template>
                        </div>
                    </div>
                </template>

                {# Note results #}
                <template x-if="result.notes && result.notes.length > 0">
                    <div>
                        <h3 class="text-sm font-semibold font-mono text-amber-800 mb-2" x-show="result.entityType === 'mixed'">Notes</h3>
                        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                            <template x-for="entity in result.notes" :key="entity.ID">
                                <a :href="'/note?id=' + entity.ID"
                                   class="block p-3 bg-white border border-stone-200 rounded-md hover:border-amber-400 hover:shadow-sm transition-colors">
                                    <div class="min-w-0">
                                        <p class="text-sm font-medium text-stone-900 truncate" x-text="entity.Name"></p>
                                        <p class="text-xs text-stone-500 mt-0.5 line-clamp-2" x-text="entity.Description || ''"></p>
                                    </div>
                                </a>
                            </template>
                        </div>
                    </div>
                </template>

                {# Group results #}
                <template x-if="result.groups && result.groups.length > 0">
                    <div>
                        <h3 class="text-sm font-semibold font-mono text-amber-800 mb-2" x-show="result.entityType === 'mixed'">Groups</h3>
                        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                            <template x-for="entity in result.groups" :key="entity.ID">
                                <a :href="'/group?id=' + entity.ID"
                                   class="block p-3 bg-white border border-stone-200 rounded-md hover:border-amber-400 hover:shadow-sm transition-colors">
                                    <div class="min-w-0">
                                        <p class="text-sm font-medium text-stone-900 truncate" x-text="entity.Name"></p>
                                        <p class="text-xs text-stone-500 mt-0.5 line-clamp-2" x-text="entity.Description || ''"></p>
                                    </div>
                                </a>
                            </template>
                        </div>
                    </div>
                </template>

                {# Empty state #}
                <template x-if="totalCount === 0">
                    <p class="text-sm text-stone-500 font-mono py-4 text-center">No results found.</p>
                </template>
            </div>
        </template>
    </section>

    {# ── Saved Queries Section ───────────────────────────────────── #}
    <section aria-label="Saved queries">
        <div class="flex items-center justify-between mb-2">
            <h2 class="text-base font-semibold font-mono text-stone-800">
                Saved Queries
                <span class="text-sm font-normal text-stone-500" x-text="'(' + savedQueries.length + ')'"></span>
            </h2>
        </div>
        <template x-if="savedQueries.length === 0">
            <p class="text-sm text-stone-500 font-mono">No saved queries yet. Run a query and click Save.</p>
        </template>
        <template x-if="savedQueries.length > 0">
            <ul class="divide-y divide-stone-200 border border-stone-200 rounded-md bg-white">
                <template x-for="q in savedQueries" :key="q.id">
                    <li class="flex items-center justify-between px-3 py-2 hover:bg-stone-50 group">
                        <button type="button"
                                @click="loadSavedQuery(q)"
                                class="flex-1 text-left min-w-0 cursor-pointer">
                            <span class="text-sm font-medium text-stone-900 truncate block" x-text="q.name"></span>
                            <span class="text-xs text-stone-500 font-mono truncate block" x-text="q.query"></span>
                        </button>
                        <button type="button"
                                @click="deleteSavedQuery(q.id, q.name)"
                                class="ml-2 text-xs text-red-600 hover:text-red-800 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity flex-shrink-0 cursor-pointer"
                                :aria-label="'Delete saved query: ' + q.name">
                            Delete
                        </button>
                    </li>
                </template>
            </ul>
        </template>
    </section>

    {# ── Query History Section ───────────────────────────────────── #}
    <section aria-label="Query history" x-show="history.length > 0">
        <div x-data="{ showHistory: false }">
            <button type="button" @click="showHistory = !showHistory"
                    class="text-sm text-amber-700 hover:text-amber-900 font-mono flex items-center gap-1 mb-2 cursor-pointer"
                    :aria-expanded="showHistory.toString()"
                    aria-controls="mrql-history-panel">
                <svg :class="showHistory && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>
                Recent Queries
            </button>
            <div x-show="showHistory" x-collapse id="mrql-history-panel">
                <ul class="divide-y divide-stone-200 border border-stone-200 rounded-md bg-white">
                    <template x-for="(h, idx) in history" :key="idx">
                        <li class="px-3 py-2 hover:bg-stone-50">
                            <button type="button"
                                    @click="loadFromHistory(h)"
                                    class="w-full text-left text-sm text-stone-700 font-mono truncate block cursor-pointer">
                                <span x-text="h"></span>
                            </button>
                        </li>
                    </template>
                </ul>
            </div>
        </div>
    </section>

</div>
{% endblock %}
