<div
    x-data="globalSearch()"
    x-cloak
    @keydown.escape.window="close()"
    class="global-search"
>
    <button
        @click="toggle()"
        class="flex items-center gap-2 px-3 py-1.5 text-sm text-gray-500 bg-gray-100 rounded-lg hover:bg-gray-200 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-1 transition-colors"
        title="Search (Ctrl+K / Cmd+K)"
        aria-label="Open search dialog"
        aria-haspopup="dialog"
    >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <span class="hidden sm:inline">Search</span>
        <kbd class="hidden sm:inline px-1.5 py-0.5 text-xs bg-white rounded border border-gray-300 font-sans" aria-hidden="true">
            <span x-text="navigator.platform.indexOf('Mac') > -1 ? '\u2318' : 'Ctrl'"></span>K
        </kbd>
    </button>

    <template x-if="isOpen">
        <div class="fixed inset-0 z-50 overflow-y-auto">
            <div
                class="fixed inset-0 bg-black/40 backdrop-blur-sm transition-opacity"
                @click="close()"
            ></div>

            <div class="relative min-h-screen flex items-start justify-center pt-[12vh] px-4" @click="close()">
                <div
                    class="relative bg-white rounded-xl shadow-2xl w-full max-w-lg overflow-hidden ring-1 ring-black/5"
                    @click.stop
                >
                    <div class="flex items-center gap-3 px-4 py-3 border-b border-gray-100">
                        <svg class="w-5 h-5 text-gray-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
                            class="flex-1 text-base text-gray-900 placeholder-gray-400 bg-transparent border-0 outline-none focus:ring-0 focus:outline-none"
                            style="box-shadow: none !important;"
                            autocomplete="off"
                            role="combobox"
                            aria-autocomplete="list"
                            aria-controls="search-results"
                            aria-label="Search"
                            :aria-expanded="results.length > 0"
                            :aria-activedescendant="results.length > 0 ? 'search-result-' + selectedIndex : null"
                        >
                        <template x-if="loading">
                            <svg class="w-5 h-5 text-gray-400 animate-spin flex-shrink-0" fill="none" viewBox="0 0 24 24">
                                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                        </template>
                        <template x-if="query.length > 0 && !loading">
                            <button
                                @click="query = ''; results = [];"
                                class="p-1 text-gray-400 hover:text-gray-600 rounded-md hover:bg-gray-100 transition-colors"
                                aria-label="Clear search"
                                type="button"
                            >
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                                </svg>
                            </button>
                        </template>
                    </div>

                    <ul
                        x-ref="resultsList"
                        id="search-results"
                        role="listbox"
                        class="max-h-[60vh] overflow-y-auto overscroll-contain"
                        x-show="results.length > 0"
                    >
                        <template x-for="(result, index) in results" :key="result.type + '-' + result.id">
                            <li
                                :id="'search-result-' + index"
                                @click="navigateTo(result.url)"
                                @mouseenter="selectedIndex = index"
                                :data-selected="selectedIndex === index"
                                role="option"
                                :aria-selected="selectedIndex === index"
                                class="px-4 py-3 cursor-pointer transition-colors"
                                :class="selectedIndex === index ? 'bg-indigo-50' : 'hover:bg-gray-50'"
                            >
                                <div class="flex items-center gap-3">
                                    <span
                                        class="flex-shrink-0 w-9 h-9 flex items-center justify-center rounded-lg text-lg"
                                        :class="selectedIndex === index ? 'bg-indigo-100' : 'bg-gray-100'"
                                        x-text="getIcon(result.type)"
                                    ></span>

                                    <div class="flex-1 min-w-0">
                                        <div class="flex items-center gap-2">
                                            <span
                                                class="font-medium text-gray-900 truncate"
                                                x-html="highlightMatch(result.name, query)"
                                            ></span>
                                            <span
                                                class="flex-shrink-0 text-xs px-2 py-0.5 rounded-full bg-gray-100 text-gray-500 font-medium"
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

                                    <svg x-show="selectedIndex === index" class="w-4 h-4 text-indigo-500 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                                    </svg>
                                </div>
                            </li>
                        </template>
                    </ul>

                    <div
                        x-show="query.length > 0 && results.length === 0 && !loading"
                        class="px-4 py-12 text-center"
                    >
                        <svg class="w-12 h-12 text-gray-300 mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        <p class="text-gray-500">No results found for "<span class="font-medium" x-text="query"></span>"</p>
                        <p class="text-sm text-gray-400 mt-1">Try a different search term</p>
                    </div>

                    <div
                        x-show="query.length === 0 && results.length === 0"
                        class="px-4 py-12 text-center"
                    >
                        <svg class="w-12 h-12 text-gray-300 mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                        </svg>
                        <p class="text-gray-500">Start typing to search</p>
                        <p class="text-sm text-gray-400 mt-1">Search across all your resources, notes, groups, and more</p>
                    </div>

                    <div class="flex items-center justify-center gap-6 px-4 py-2.5 bg-gray-50/80 border-t border-gray-100 text-xs text-gray-400">
                        <span class="flex items-center gap-1.5">
                            <kbd class="px-1.5 py-0.5 bg-white rounded border border-gray-200 font-sans text-gray-500 shadow-sm">&uarr;</kbd>
                            <kbd class="px-1.5 py-0.5 bg-white rounded border border-gray-200 font-sans text-gray-500 shadow-sm">&darr;</kbd>
                            <span>navigate</span>
                        </span>
                        <span class="flex items-center gap-1.5">
                            <kbd class="px-1.5 py-0.5 bg-white rounded border border-gray-200 font-sans text-gray-500 shadow-sm">&crarr;</kbd>
                            <span>select</span>
                        </span>
                        <span class="flex items-center gap-1.5">
                            <kbd class="px-2 py-0.5 bg-white rounded border border-gray-200 font-sans text-gray-500 shadow-sm">esc</kbd>
                            <span>close</span>
                        </span>
                    </div>
                </div>
            </div>
        </div>
    </template>
</div>
