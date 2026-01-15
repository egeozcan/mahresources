<div
    x-data="globalSearch()"
    x-cloak
    @keydown.escape.window="close()"
    class="global-search"
>
    <button
        @click="toggle()"
        class="flex items-center gap-2 px-3 py-1 text-sm text-gray-500 bg-gray-100 rounded-md hover:bg-gray-200 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        title="Search (Ctrl+K / Cmd+K)"
    >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <span class="hidden sm:inline">Search</span>
        <kbd class="hidden sm:inline px-1.5 py-0.5 text-xs bg-white rounded border">
            <span x-text="navigator.platform.indexOf('Mac') > -1 ? '\u2318' : 'Ctrl'"></span>K
        </kbd>
    </button>

    <template x-if="isOpen">
        <div class="fixed inset-0 z-50 overflow-y-auto">
            <div
                class="fixed inset-0 bg-black bg-opacity-25 transition-opacity"
                @click="close()"
            ></div>

            <div class="relative min-h-screen flex items-start justify-center pt-16 sm:pt-24 px-4">
                <div
                    class="relative bg-white rounded-lg shadow-2xl w-full max-w-xl overflow-hidden"
                    @click.stop
                >
                    <div class="flex items-center border-b px-4">
                        <svg class="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                        </svg>
                        <input
                            x-ref="searchInput"
                            x-model="query"
                            @input="search()"
                            @keydown.arrow-up.prevent="navigateUp()"
                            @keydown.arrow-down.prevent="navigateDown()"
                            @keydown.enter.prevent="selectResult()"
                            type="text"
                            placeholder="Search resources, notes, groups, tags..."
                            class="w-full px-3 py-4 text-sm focus:outline-none"
                            autocomplete="off"
                            role="combobox"
                            aria-autocomplete="list"
                            aria-controls="search-results"
                            :aria-expanded="results.length > 0"
                        >
                        <template x-if="loading">
                            <svg class="w-5 h-5 text-gray-400 animate-spin" fill="none" viewBox="0 0 24 24">
                                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                        </template>
                    </div>

                    <ul
                        x-ref="resultsList"
                        id="search-results"
                        role="listbox"
                        class="max-h-80 overflow-y-auto"
                        x-show="results.length > 0"
                    >
                        <template x-for="(result, index) in results" :key="result.type + '-' + result.id">
                            <li
                                @click="navigateTo(result.url)"
                                @mouseenter="selectedIndex = index"
                                :data-selected="selectedIndex === index"
                                role="option"
                                :aria-selected="selectedIndex === index"
                                class="px-4 py-3 cursor-pointer border-b border-gray-100 last:border-b-0 transition-colors"
                                :class="selectedIndex === index ? 'bg-indigo-50' : 'hover:bg-gray-50'"
                            >
                                <div class="flex items-start gap-3">
                                    <span
                                        class="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded bg-gray-100 text-lg"
                                        x-text="getIcon(result.type)"
                                    ></span>

                                    <div class="flex-1 min-w-0">
                                        <div class="flex items-center gap-2">
                                            <span
                                                class="font-medium text-gray-900 truncate"
                                                x-html="highlightMatch(result.name, query)"
                                            ></span>
                                            <span
                                                class="flex-shrink-0 text-xs px-1.5 py-0.5 rounded bg-gray-200 text-gray-600"
                                                x-text="getLabel(result.type)"
                                            ></span>
                                        </div>
                                        <p
                                            x-show="result.description"
                                            class="text-sm text-gray-500 truncate mt-0.5"
                                            x-html="highlightMatch(result.description, query)"
                                        ></p>
                                        <div
                                            x-show="result.extra && Object.keys(result.extra).length > 0"
                                            class="flex gap-2 mt-1"
                                        >
                                            <template x-for="(value, key) in (result.extra || {})" :key="key">
                                                <span class="text-xs text-gray-400" x-text="value"></span>
                                            </template>
                                        </div>
                                    </div>
                                </div>
                            </li>
                        </template>
                    </ul>

                    <div
                        x-show="query.length > 0 && results.length === 0 && !loading"
                        class="px-4 py-8 text-center text-gray-500"
                    >
                        <p>No results found for "<span x-text="query"></span>"</p>
                    </div>

                    <div
                        x-show="query.length === 0 && results.length === 0"
                        class="px-4 py-8 text-center text-gray-400"
                    >
                        <p>Start typing to search...</p>
                    </div>

                    <div class="flex items-center justify-between px-4 py-2 bg-gray-50 text-xs text-gray-500 border-t">
                        <div class="flex items-center gap-4">
                            <span class="flex items-center gap-1">
                                <kbd class="px-1.5 py-0.5 bg-white rounded border">&uarr;</kbd>
                                <kbd class="px-1.5 py-0.5 bg-white rounded border">&darr;</kbd>
                                <span>to navigate</span>
                            </span>
                            <span class="flex items-center gap-1">
                                <kbd class="px-1.5 py-0.5 bg-white rounded border">&crarr;</kbd>
                                <span>to select</span>
                            </span>
                            <span class="flex items-center gap-1">
                                <kbd class="px-1.5 py-0.5 bg-white rounded border">Esc</kbd>
                                <span>to close</span>
                            </span>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </template>
</div>
