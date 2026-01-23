<div
    x-data="{
        canNavigate() {
            // Allow navigation unless focus is on an input or textarea
            const activeEl = document.activeElement;
            return !activeEl || !['INPUT', 'TEXTAREA'].includes(activeEl.tagName);
        }
    }"
    x-show="$store.lightbox.isOpen"
    x-cloak
    x-trap.noscroll="$store.lightbox.isOpen"
    @keydown.escape.window="$store.lightbox.isOpen && $store.lightbox.handleEscape()"
    @keydown.arrow-left.window="$store.lightbox.isOpen && canNavigate() && $store.lightbox.prev()"
    @keydown.arrow-right.window="$store.lightbox.isOpen && canNavigate() && $store.lightbox.next()"
    @touchstart="$store.lightbox.handleTouchStart($event)"
    @touchend="$store.lightbox.handleTouchEnd($event)"
    class="fixed inset-0 z-50 flex"
    role="dialog"
    aria-modal="true"
    :aria-label="$store.lightbox.getCurrentItem()?.name || 'Media viewer'"
>
    <!-- Backdrop -->
    <div
        class="absolute inset-0 bg-black/90"
        @click="$store.lightbox.close()"
    ></div>

    <!-- Main content area (shrinks when edit panel opens on desktop) -->
    <div
        class="relative flex-1 flex items-center justify-center transition-all duration-300 ease-in-out"
        :class="$store.lightbox.editPanelOpen ? 'md:mr-[400px]' : ''"
    >
        <!-- Loading spinner (shown while media is loading) -->
        <div
            x-show="$store.lightbox.loading"
            class="absolute inset-0 flex items-center justify-center pointer-events-none z-10"
        >
            <svg class="w-12 h-12 text-white animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
        </div>

        <!-- Media content -->
        <div class="relative max-h-[90vh] max-w-[90vw] flex items-center justify-center">
            <!-- Image display -->
            <template x-if="$store.lightbox.isImage($store.lightbox.getCurrentItem()?.contentType)">
                <img
                    :src="$store.lightbox.getCurrentItem()?.viewUrl"
                    :alt="$store.lightbox.getCurrentItem()?.name || 'Image'"
                    class="max-h-[90vh] object-contain transition-all duration-300"
                    :class="$store.lightbox.editPanelOpen ? 'md:max-w-[calc(100vw-450px)]' : 'max-w-[90vw]'"
                    x-init="$nextTick(() => $store.lightbox.checkIfMediaLoaded($el))"
                    @load="$store.lightbox.onMediaLoaded()"
                    @error="$store.lightbox.onMediaLoaded()"
                >
            </template>

            <!-- Video display -->
            <template x-if="$store.lightbox.isVideo($store.lightbox.getCurrentItem()?.contentType)">
                <video
                    :src="$store.lightbox.getCurrentItem()?.viewUrl"
                    :key="$store.lightbox.getCurrentItem()?.id"
                    controls
                    class="max-h-[90vh] transition-all duration-300"
                    :class="$store.lightbox.editPanelOpen ? 'md:max-w-[calc(100vw-450px)]' : 'max-w-[90vw]'"
                    x-init="$nextTick(() => $store.lightbox.checkIfMediaLoaded($el))"
                    @loadeddata="$store.lightbox.onMediaLoaded()"
                    @error="$store.lightbox.onMediaLoaded()"
                >
                    Your browser does not support video playback.
                </video>
            </template>
        </div>

        <!-- Previous button -->
        <button
            @click.stop="$store.lightbox.prev()"
            :disabled="$store.lightbox.pageLoading || ($store.lightbox.currentIndex === 0 && !$store.lightbox.hasPrevPage)"
            class="absolute left-4 top-1/2 -translate-y-1/2 p-3 bg-white/10 hover:bg-white/20 disabled:opacity-30 disabled:cursor-not-allowed rounded-full text-white transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 z-10"
            aria-label="Previous"
        >
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
            </svg>
        </button>

        <!-- Next button -->
        <button
            @click.stop="$store.lightbox.next()"
            :disabled="$store.lightbox.pageLoading || ($store.lightbox.currentIndex === $store.lightbox.items.length - 1 && !$store.lightbox.hasNextPage)"
            class="absolute right-4 top-1/2 -translate-y-1/2 p-3 bg-white/10 hover:bg-white/20 disabled:opacity-30 disabled:cursor-not-allowed rounded-full text-white transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 z-10"
            aria-label="Next"
        >
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
            </svg>
        </button>

        <!-- Close button -->
        <button
            @click.stop="$store.lightbox.close()"
            class="absolute top-4 right-4 p-2 bg-white/10 hover:bg-white/20 rounded-full text-white transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 z-20"
            :class="$store.lightbox.editPanelOpen ? 'md:right-[416px]' : ''"
            aria-label="Close"
        >
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
        </button>
    </div>

    <!-- Edit Panel (slides in from right) -->
    <div
        x-show="$store.lightbox.editPanelOpen"
        x-transition:enter="transition ease-out duration-300"
        x-transition:enter-start="opacity-0 translate-x-full"
        x-transition:enter-end="opacity-100 translate-x-0"
        x-transition:leave="transition ease-in duration-200"
        x-transition:leave-start="opacity-100 translate-x-0"
        x-transition:leave-end="opacity-0 translate-x-full"
        data-edit-panel
        class="fixed md:absolute inset-0 md:inset-auto md:top-0 md:right-0 md:bottom-0 md:w-[400px] bg-gray-900 md:bg-gray-900/95 md:backdrop-blur-sm text-white overflow-y-auto z-30"
        @click.stop
    >
        <!-- Panel header -->
        <div class="sticky top-0 bg-gray-900 md:bg-gray-900/95 border-b border-gray-700 p-4 flex items-center justify-between z-10">
            <h2 class="text-lg font-semibold">Edit Resource</h2>
            <button
                @click="$store.lightbox.closeEditPanel()"
                class="p-1.5 hover:bg-white/10 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
                aria-label="Close edit panel"
            >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                </svg>
            </button>
        </div>

        <!-- Panel content -->
        <div class="p-4">
            <!-- Loading state -->
            <template x-if="$store.lightbox.detailsLoading && !$store.lightbox.resourceDetails">
                <div class="flex items-center justify-center py-12">
                    <svg class="w-8 h-8 animate-spin text-white/50" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                </div>
            </template>

            <!-- Edit form -->
            <template x-if="$store.lightbox.resourceDetails">
                <div class="space-y-5 relative">
                    <!-- Loading overlay when fetching new resource details -->
                    <div
                        x-show="$store.lightbox.detailsLoading"
                        x-transition:enter="transition ease-out duration-100"
                        x-transition:enter-start="opacity-0"
                        x-transition:enter-end="opacity-100"
                        x-transition:leave="transition ease-in duration-75"
                        x-transition:leave-start="opacity-100"
                        x-transition:leave-end="opacity-0"
                        class="absolute inset-0 bg-gray-900/50 flex items-center justify-center z-10 rounded"
                    >
                        <svg class="w-6 h-6 animate-spin text-white/70" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                    </div>

                    <!-- Name field -->
                    <div>
                        <label for="lightbox-edit-name" class="block text-sm font-medium text-gray-300 mb-1.5">Name</label>
                        <input
                            type="text"
                            id="lightbox-edit-name"
                            :value="$store.lightbox.resourceDetails?.Name || ''"
                            @blur="$store.lightbox.updateName($event.target.value)"
                            @keydown.enter="$event.target.blur()"
                            @keydown.escape.stop="$event.target.blur()"
                            class="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-md text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                            placeholder="Resource name"
                        >
                    </div>

                    <!-- Description field -->
                    <div>
                        <label for="lightbox-edit-description" class="block text-sm font-medium text-gray-300 mb-1.5">Description</label>
                        <textarea
                            id="lightbox-edit-description"
                            :value="$store.lightbox.resourceDetails?.Description || ''"
                            @blur="$store.lightbox.updateDescription($event.target.value)"
                            @keydown.escape.stop="$event.target.blur()"
                            rows="4"
                            class="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-md text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent resize-y"
                            placeholder="Add a description..."
                        ></textarea>
                    </div>

                    <!-- Tags section - uses autocompleter component -->
                    <!-- :key forces recreation when resource changes to get fresh tags -->
                    <!-- Use spread to create a fresh array copy, avoiding shared reference issues with Alpine reactivity -->
                    <div
                        x-data="autocompleter({
                            selectedResults: [...($store.lightbox.resourceDetails?.Tags || [])],
                            url: '/v1/tags',
                            addUrl: '/v1/tag',
                            standalone: true,
                            onSelect: (tag) => $store.lightbox.saveTagAddition(tag),
                            onRemove: (tag) => $store.lightbox.saveTagRemoval(tag)
                        })"
                        :key="$store.lightbox.getCurrentItem()?.id"
                        class="relative"
                    >
                        <label class="block text-sm font-medium text-gray-300 mb-1.5">Tags</label>

                        <!-- Add tag input -->
                        <template x-if="addModeForTag == ''">
                            <div class="relative mb-3">
                                <input
                                    x-ref="autocompleter"
                                    type="text"
                                    x-bind="inputEvents"
                                    class="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-md text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                                    placeholder="Search or add tags..."
                                    autocomplete="off"
                                    role="combobox"
                                    aria-autocomplete="list"
                                    :aria-expanded="dropdownActive && results.length > 0"
                                    aria-controls="lightbox-tag-listbox"
                                    :aria-activedescendant="selectedIndex >= 0 && results[selectedIndex] ? 'lightbox-tag-result-' + selectedIndex : null"
                                >

                                <!-- Tag search results dropdown -->
                                <template x-if="dropdownActive && results.length > 0">
                                    <div
                                        id="lightbox-tag-listbox"
                                        x-ref="list"
                                        role="listbox"
                                        class="absolute z-10 mt-1 w-full bg-gray-800 border border-gray-700 rounded-md shadow-lg max-h-48 overflow-y-auto"
                                    >
                                        <template x-for="(tag, index) in results" :key="tag.ID">
                                            <div
                                                @mousedown.prevent="selectedIndex = index; pushVal($event)"
                                                @mouseover="selectedIndex = index"
                                                role="option"
                                                :id="'lightbox-tag-result-' + index"
                                                :aria-selected="index === selectedIndex"
                                                class="px-3 py-2 cursor-pointer"
                                                :class="index === selectedIndex ? 'bg-indigo-600 text-white' : 'text-gray-300 hover:bg-gray-700'"
                                            >
                                                <span x-text="tag.Name"></span>
                                            </div>
                                        </template>
                                    </div>
                                </template>

                                <!-- Loading indicator -->
                                <template x-if="loading">
                                    <div class="absolute right-3 top-1/2 -translate-y-1/2">
                                        <svg class="w-4 h-4 animate-spin text-gray-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                        </svg>
                                    </div>
                                </template>
                            </div>
                        </template>

                        <!-- Add new tag confirmation -->
                        <template x-if="addModeForTag">
                            <div class="flex gap-2 items-stretch justify-between mb-3">
                                <button
                                    type="button"
                                    class="flex-1 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-green-700 hover:bg-green-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 py-2 px-3"
                                    x-text="'Add ' + addModeForTag + '?'"
                                    x-init="setTimeout(() => $el.focus(), 1)"
                                    @keydown.escape.prevent="exitAdd"
                                    @keydown.enter.prevent="addVal"
                                    @click="addVal"
                                ></button>
                                <button
                                    type="button"
                                    class="border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 py-2 px-3"
                                    @click="exitAdd"
                                    @keydown.escape.prevent="exitAdd"
                                >Cancel</button>
                            </div>
                        </template>

                        <!-- Current tags -->
                        <div class="flex flex-wrap gap-2">
                            <template x-for="tag in selectedResults" :key="tag.ID">
                                <span class="inline-flex items-center gap-1 px-2.5 py-1 bg-indigo-600 text-white text-sm rounded-full">
                                    <span x-text="tag.Name"></span>
                                    <button
                                        @click="removeItem(tag)"
                                        type="button"
                                        class="hover:bg-indigo-700 rounded-full p-0.5 focus:outline-none focus:ring-1 focus:ring-white"
                                        :aria-label="'Remove tag ' + tag.Name"
                                    >
                                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                        </svg>
                                    </button>
                                </span>
                            </template>
                            <!-- Use x-show instead of x-if to avoid interfering with x-for's DOM markers -->
                            <span x-show="!selectedResults?.length" x-cloak class="text-gray-500 text-sm italic">No tags yet</span>
                        </div>
                    </div>

                    <!-- Link to full details page -->
                    <div class="pt-4 border-t border-gray-700">
                        <a
                            :href="'/resource?id=' + $store.lightbox.getCurrentItem()?.id"
                            class="inline-flex items-center gap-2 text-indigo-400 hover:text-indigo-300 text-sm"
                        >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                            </svg>
                            View full resource details
                        </a>
                    </div>
                </div>
            </template>
        </div>
    </div>

    <!-- Page loading indicator -->
    <div
        x-show="$store.lightbox.pageLoading"
        x-transition
        class="absolute bottom-20 left-1/2 -translate-x-1/2 px-4 py-2 bg-white/10 backdrop-blur rounded text-white text-sm z-20"
        :class="$store.lightbox.editPanelOpen ? 'md:-translate-x-[calc(50%+200px)]' : ''"
    >
        Loading more items...
    </div>

    <!-- Bottom bar with counter and name -->
    <div
        class="absolute bottom-4 left-0 flex justify-between items-center px-4 text-white text-sm transition-all duration-300 z-20"
        :class="$store.lightbox.editPanelOpen ? 'right-0 md:right-[400px]' : 'right-0'"
    >
        <!-- Counter -->
        <div class="bg-black/50 px-3 py-1 rounded">
            <span x-text="$store.lightbox.currentIndex + 1"></span>
            /
            <span x-text="$store.lightbox.items.length"></span>
            <span x-show="$store.lightbox.hasNextPage" class="text-gray-400">+</span>
        </div>

        <!-- Name -->
        <div
            x-show="$store.lightbox.getCurrentItem()?.name"
            class="bg-black/50 px-3 py-1 rounded max-w-[30vw] truncate hidden md:block"
            x-text="$store.lightbox.getCurrentItem()?.name"
        ></div>

        <!-- Edit button -->
        <button
            @click.stop="$store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.openEditPanel()"
            class="bg-black/50 px-3 py-1.5 rounded hover:bg-white/20 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 flex items-center gap-1.5"
            :class="$store.lightbox.editPanelOpen ? 'bg-indigo-600/80 hover:bg-indigo-700/80' : ''"
            :aria-pressed="$store.lightbox.editPanelOpen"
            :title="$store.lightbox.editPanelOpen ? 'Close edit panel' : 'Edit resource'"
        >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
            </svg>
            <span x-text="$store.lightbox.editPanelOpen ? 'Close' : 'Edit'"></span>
        </button>
    </div>
</div>
