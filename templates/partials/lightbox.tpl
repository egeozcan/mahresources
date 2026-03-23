<div
    x-data="{
        canNavigate() {
            // Allow navigation unless focus is on an input, textarea, or select
            const activeEl = document.activeElement;
            return !activeEl || !['INPUT', 'TEXTAREA', 'SELECT'].includes(activeEl.tagName);
        }
    }"
    x-show="$store.lightbox.isOpen"
    x-cloak
    x-trap="$store.lightbox.isOpen"
    @keydown.escape.window="$store.lightbox.isOpen && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.handleEscape())"
    @keydown.arrow-left.window="$store.lightbox.isOpen && canNavigate() && $store.lightbox.prev()"
    @keydown.arrow-right.window="$store.lightbox.isOpen && canNavigate() && $store.lightbox.next()"
    @keydown.page-up.window.prevent="$store.lightbox.isOpen && $store.lightbox.prev()"
    @keydown.page-down.window.prevent="$store.lightbox.isOpen && $store.lightbox.next()"
    @keydown.enter.window="$store.lightbox.isOpen && canNavigate() && !document.activeElement?.closest('[data-edit-panel], [data-quick-tag-panel]') && $store.lightbox.toggleFullscreen()"
    @keydown.e.window="$store.lightbox.isOpen && canNavigate() && ($store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.openEditPanel())"
    @keydown.f2.window.prevent="$store.lightbox.isOpen && ($store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.openEditPanel())"
    @keydown.t.window="$store.lightbox.isOpen && canNavigate() && ($store.lightbox.quickTagPanelOpen ? $store.lightbox.closeQuickTagPanel() : $store.lightbox.openQuickTagPanel())"
    @keydown.1.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(0, $event)"
    @keydown.2.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(1, $event)"
    @keydown.3.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(2, $event)"
    @keydown.4.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(3, $event)"
    @keydown.5.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(4, $event)"
    @keydown.6.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(5, $event)"
    @keydown.7.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(6, $event)"
    @keydown.8.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(7, $event)"
    @keydown.9.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(8, $event)"
    @keyup.1.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(0)"
    @keyup.2.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(1)"
    @keyup.3.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(2)"
    @keyup.4.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(3)"
    @keyup.5.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(4)"
    @keyup.6.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(5)"
    @keyup.7.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(6)"
    @keyup.8.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(7)"
    @keyup.9.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(8)"
    @keyup.0.window="$store.lightbox.isOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.focusTagEditor())"
    @keydown.z.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.switchTab(0)"
    @keydown.x.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.switchTab(1)"
    @keydown.c.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.switchTab(2)"
    @keydown.v.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.switchTab(3)"
    @keydown.b.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.switchTab(4)"
    @touchstart="$store.lightbox.handleTouchStart($event)"
    @touchmove="$store.lightbox.handleTouchMove($event)"
    @touchend="$store.lightbox.handleTouchEnd($event)"
    class="fixed inset-0 z-50 flex h-dvh w-dvw"
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    :aria-label="$store.lightbox.getCurrentItem()?.name || 'Media viewer'"
>
    <!-- Backdrop -->
    <div
        class="absolute inset-0 h-full w-full bg-black/90"
        @click="$store.lightbox.close()"
    ></div>

    <!-- Main content area (shrinks when edit panel opens on desktop) -->
    <div
        class="relative flex-1 flex flex-col transition-all duration-300 ease-in-out overflow-hidden"
        :class="[
            $store.lightbox.editPanelOpen ? ($store.lightbox.quickTagPanelOpen ? 'lg:mr-[320px]' : 'lg:mr-[400px]') : '',
            $store.lightbox.quickTagPanelOpen ? ($store.lightbox.editPanelOpen ? 'lg:ml-[320px]' : 'lg:ml-[400px]') : ''
        ]"
    >
    <!-- Media area (centered, fills available space) -->
    <div
        class="flex-1 flex items-center justify-center min-h-0 relative"
        :class="$store.lightbox.isDragging ? 'cursor-grabbing' : 'cursor-grab'"
        @click.self="$store.lightbox.close()"
        @mousedown="$store.lightbox.handleMouseDown($event)"
        @mousemove="$store.lightbox.handleMouseMove($event)"
        @mouseup="$store.lightbox.handleMouseUp($event)"
        @mouseleave="$store.lightbox.handleMouseUp($event)"
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
        <div class="relative max-h-[90vh] max-w-[90vw] flex items-center justify-center" @click.self="$store.lightbox.close()" @dblclick="$store.lightbox.handleDoubleClick($event)">
            <!-- Image display -->
            <template x-if="$store.lightbox.isImage($store.lightbox.getCurrentItem()?.contentType)">
                <img
                    :src="$store.lightbox.getCurrentItem()?.viewUrl"
                    :alt="$store.lightbox.getCurrentItem()?.name || 'Image'"
                    tabindex="-1"
                    class="max-h-[90vh] object-contain"
                    :class="[$store.lightbox._mediaMaxWidthClass(), $store.lightbox.animationsDisabled ? '' : 'transition-all duration-300']"
                    :style="{ transform: `scale(${$store.lightbox.zoomLevel}) translate(${$store.lightbox.panX}px, ${$store.lightbox.panY}px)`, transformOrigin: 'center center' }"
                    x-init="$nextTick(() => $store.lightbox.checkIfMediaLoaded($el))"
                    @load="$store.lightbox.onMediaLoaded()"
                    @error="$store.lightbox.onMediaLoaded()"
                >
            </template>

            <!-- SVG display - use object tag for better SVG rendering with proper sizing -->
            <!-- Wrapped in a div with overlay to prevent the embedded SVG from stealing focus -->
            <template x-if="$store.lightbox.isSvg($store.lightbox.getCurrentItem()?.contentType)">
                <div class="relative">
                    <object
                        :data="$store.lightbox.getCurrentItem()?.viewUrl"
                        type="image/svg+xml"
                        :aria-label="$store.lightbox.getCurrentItem()?.name || 'SVG Image'"
                        tabindex="-1"
                        class="max-h-[90vh] max-w-[90vw] min-h-[50vh] min-w-[50vw] pointer-events-none"
                        :class="[$store.lightbox._mediaMaxWidthClass(), $store.lightbox.animationsDisabled ? '' : 'transition-all duration-300']"
                        :style="{ transform: `scale(${$store.lightbox.zoomLevel}) translate(${$store.lightbox.panX}px, ${$store.lightbox.panY}px)`, transformOrigin: 'center center' }"
                        x-init="$nextTick(() => $store.lightbox.checkIfMediaLoaded($el))"
                        @load="$store.lightbox.onMediaLoaded()"
                        @error="$store.lightbox.onMediaLoaded()"
                    >
                        <!-- Fallback to img if object fails -->
                        <img
                            :src="$store.lightbox.getCurrentItem()?.viewUrl"
                            :alt="$store.lightbox.getCurrentItem()?.name || 'SVG Image'"
                            tabindex="-1"
                            class="max-h-[90vh] max-w-[90vw]"
                        >
                    </object>
                    <!-- Transparent overlay to capture clicks without stealing focus -->
                    <div class="absolute inset-0" tabindex="-1"></div>
                </div>
            </template>

            <!-- Video display -->
            <template x-if="$store.lightbox.isVideo($store.lightbox.getCurrentItem()?.contentType)">
                <video
                    :src="$store.lightbox.getCurrentItem()?.viewUrl"
                    :key="$store.lightbox.getCurrentItem()?.id"
                    controls
                    class="max-h-[90vh] transition-all duration-300"
                    :class="$store.lightbox._mediaMaxWidthClass()"
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
            aria-label="Close"
        >
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
        </button>
    </div>

    <!-- Bottom bar with counter, resolution, and controls (in flow, does not cover media) -->
    <div class="flex flex-wrap justify-between items-center gap-1 px-4 py-2 text-white text-sm z-20">
        <!-- Counter -->
        <div class="bg-black/50 px-3 py-1 rounded">
            <span x-text="$store.lightbox.currentIndex + 1"></span>
            /
            <span x-text="$store.lightbox.items.length"></span>
            <span x-show="$store.lightbox.hasNextPage" class="text-stone-400">+</span>
        </div>

        <!-- Native resolution -->
        <div
            x-show="$store.lightbox.getCurrentItem()?.width && $store.lightbox.getCurrentItem()?.height"
            class="bg-black/50 px-3 py-1 rounded tabular-nums"
            x-text="($store.lightbox.getCurrentItem()?.width || '') + ' \u00d7 ' + ($store.lightbox.getCurrentItem()?.height || '')"
        ></div>

        <!-- Native zoom percentage with preset picker -->
        <div x-show="$store.lightbox.nativeZoomPercent()">
            <button
                @click.stop="$store.lightbox.showZoomPresets($el)"
                class="bg-black/50 px-3 py-1 rounded tabular-nums hover:bg-white/30 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
                x-text="$store.lightbox.nativeZoomPercent()"
                title="Choose zoom level"
            ></button>
        </div>

        <!-- Fullscreen button -->
        <button
            x-show="$store.lightbox.fullscreenSupported()"
            @click.stop="$store.lightbox.toggleFullscreen()"
            class="bg-black/50 px-3 py-1.5 rounded hover:bg-white/20 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 flex items-center gap-1.5"
            :title="$store.lightbox.isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'"
            :aria-label="$store.lightbox.isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'"
        >
            <!-- Expand icon (not fullscreen) -->
            <svg x-show="!$store.lightbox.isFullscreen" aria-hidden="true" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4"></path>
            </svg>
            <!-- Compress icon (fullscreen) -->
            <svg x-show="$store.lightbox.isFullscreen" x-cloak aria-hidden="true" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 9L4 4m0 0v4m0-4h4m6 0l5-5m0 0v4m0-4h-4M9 15l-5 5m0 0v-4m0 4h4m6 0l5 5m0 0v-4m0 4h-4"></path>
            </svg>
        </button>

        <!-- Owner -->
        <a
            x-show="$store.lightbox.getCurrentItem()?.ownerName"
            :href="'/group?id=' + $store.lightbox.getCurrentItem()?.ownerId"
            class="bg-black/50 px-3 py-1 rounded max-w-[20vw] truncate hidden md:block hover:bg-white/20 transition-colors"
            x-text="$store.lightbox.getCurrentItem()?.ownerName"
        ></a>

        <!-- Name -->
        <div
            x-show="$store.lightbox.getCurrentItem()?.name"
            class="bg-black/50 px-3 py-1 rounded max-w-[30vw] truncate hidden md:block"
            x-text="$store.lightbox.getCurrentItem()?.name"
        ></div>

        <!-- Quick Tag button (hidden when panel is open — panel has its own close button) -->
        <button
            x-show="!$store.lightbox.quickTagPanelOpen"
            @click.stop="$store.lightbox.openQuickTagPanel()"
            class="bg-black/50 px-3 py-1.5 rounded hover:bg-white/20 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 flex items-center gap-1.5"
            title="Edit tags"
        >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A2 2 0 013 12V7a4 4 0 014-4z"></path>
            </svg>
            <span>Edit Tags</span>
        </button>

        <!-- Edit button (hidden when panel is open — panel has its own close button) -->
        <button
            x-show="!$store.lightbox.editPanelOpen"
            @click.stop="$store.lightbox.openEditPanel()"
            class="bg-black/50 px-3 py-1.5 rounded hover:bg-white/20 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 flex items-center gap-1.5"
            title="Edit resource"
        >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
            </svg>
            <span>Edit</span>
        </button>
    </div>
    </div>

    <!-- Quick Tag Panel (slides in from left) -->
    <div
        x-show="$store.lightbox.quickTagPanelOpen"
        x-transition:enter="transition ease-out duration-300"
        x-transition:enter-start="opacity-0 -translate-x-full"
        x-transition:enter-end="opacity-100 translate-x-0"
        x-transition:leave="transition ease-in duration-200"
        x-transition:leave-start="opacity-100 translate-x-0"
        x-transition:leave-end="opacity-0 -translate-x-full"
        data-quick-tag-panel
        class="fixed md:absolute inset-0 md:inset-auto md:top-0 md:left-0 md:bottom-0 bg-stone-900 md:bg-stone-900/95 md:backdrop-blur-sm text-white overflow-y-auto z-30"
        :class="$store.lightbox.editPanelOpen ? 'md:w-[320px]' : 'md:w-[400px]'"
        @click.stop
        x-effect="$store.lightbox._setupExpandedClickOutside()"
        @focusout="$store.lightbox.isExpanded() && $nextTick(() => { if (!$el.contains(document.activeElement)) $store.lightbox.collapseExpanded(); })"
    >
        <!-- Panel header -->
        <div class="sticky top-0 bg-stone-900 md:bg-stone-900/95 border-b border-stone-700 p-4 flex items-center justify-between z-10">
            <h2 class="text-lg font-semibold">Edit Tags</h2>
            <button
                @click="$store.lightbox.closeQuickTagPanel()"
                class="p-1.5 hover:bg-white/10 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
                aria-label="Close edit tags panel"
            >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                </svg>
            </button>
        </div>

        <!-- Panel content -->
        <div class="p-4 space-y-4">
            <!-- Tag editor (autocompleter) -->
            <template x-if="$store.lightbox.resourceDetails">
                <div
                    x-data="autocompleter({
                        selectedResults: [...($store.lightbox.resourceDetails?.Tags || [])],
                        url: '/v1/tags',
                        addUrl: '/v1/tag',
                        standalone: true,
                        sortBy: 'most_used_resource',
                        onSelect: (tag) => $store.lightbox.saveTagAddition(tag),
                        onRemove: (tag) => $store.lightbox.saveTagRemoval(tag)
                    })"
                    :key="$store.lightbox.resourceDetails?.ID"
                    x-effect="selectedResults = [...($store.lightbox.resourceDetails?.Tags || [])]"
                    class="relative"
                >
                <label class="block text-sm font-medium font-mono text-stone-300 mb-1.5">Tags</label>

                <!-- Add tag input -->
                <template x-if="!addModeForTag">
                    <div class="relative mb-3">
                        <input
                            x-ref="autocompleter"
                            data-tag-editor-input
                            type="text"
                            x-bind="inputEvents"
                            class="w-full px-3 py-2 bg-stone-800 border border-stone-700 rounded-md text-white placeholder-stone-500 focus:outline-none focus:ring-2 focus:ring-stone-400 focus:border-transparent"
                            placeholder="Search or add tags..."
                            autocomplete="off"
                            role="combobox"
                            aria-autocomplete="list"
                            aria-controls="lightbox-tag-listbox"
                            :aria-activedescendant="selectedIndex >= 0 && results[selectedIndex] ? 'lightbox-tag-result-' + selectedIndex : null"
                            :aria-expanded="dropdownActive && results.length > 0"
                        >

                        <!-- Tag search results dropdown (popover) -->
                        <div x-ref="dropdown" popover
                             id="lightbox-tag-listbox"
                             class="bg-stone-800 border border-stone-700 rounded-md shadow-lg max-h-48 overflow-y-auto"
                             role="listbox">
                            <template x-for="(tag, rIndex) in results" :key="tag.ID">
                                <div
                                    @mousedown.prevent="selectedIndex = rIndex; pushVal($event)"
                                    @mouseover="selectedIndex = rIndex"
                                    :id="'lightbox-tag-result-' + rIndex"
                                    role="option"
                                    :aria-selected="rIndex === selectedIndex"
                                    class="px-3 py-2 cursor-pointer text-sm"
                                    :class="rIndex === selectedIndex ? 'bg-amber-700 text-white' : 'text-stone-300 hover:bg-stone-700'"
                                >
                                    <span x-text="tag.Name"></span>
                                </div>
                            </template>
                        </div>

                        <!-- Loading indicator -->
                        <template x-if="loading">
                            <div class="absolute right-3 top-1/2 -translate-y-1/2">
                                <svg class="w-4 h-4 animate-spin text-stone-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
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
                            class="flex-1 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 py-2 px-3"
                            x-text="'Add ' + addModeForTag + '?'"
                            x-init="setTimeout(() => $el.focus(), 1)"
                            @keydown.escape.prevent="exitAdd"
                            @keydown.enter.prevent.stop="addVal"
                            @click="addVal"
                        ></button>
                        <button
                            type="button"
                            class="border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600 py-2 px-3"
                            @click="exitAdd"
                            @keydown.escape.prevent="exitAdd"
                        >Cancel</button>
                    </div>
                </template>

                <!-- Current tags as pills -->
                <div class="flex flex-wrap gap-2">
                    <template x-for="tag in selectedResults" :key="tag.ID">
                        <span class="inline-flex items-center gap-1 px-2.5 py-1 bg-amber-700 text-white text-sm rounded-full font-mono">
                            <span x-text="tag.Name"></span>
                            <button
                                @click="removeItem(tag)"
                                type="button"
                                class="hover:bg-amber-800 rounded-full p-0.5 focus:outline-none focus:ring-1 focus:ring-white"
                                :aria-label="'Remove tag ' + tag.Name"
                            >
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                </svg>
                            </button>
                        </span>
                    </template>
                    <span x-show="!selectedResults?.length" x-cloak class="text-stone-500 text-sm italic">No tags yet</span>
                </div>
                </div>
            </template>
            <!-- Tags loading state -->
            <template x-if="!$store.lightbox.resourceDetails">
                <div class="relative">
                    <label class="block text-sm font-medium font-mono text-stone-300 mb-1.5">Tags</label>
                    <div class="text-stone-500 text-sm italic">Loading tags...</div>
                </div>
            </template>

            <!-- Divider -->
            <div class="border-t border-stone-700"></div>

            <!-- Tab bar / Expanded header -->
            <template x-if="!$store.lightbox.isExpanded()">
              <div class="flex" role="tablist" aria-label="Tag slot tabs">
                <template x-for="(tab, tIdx) in $store.lightbox.tabLabels" :key="tIdx">
                    <button
                        @click="$store.lightbox.switchTab(tIdx)"
                        role="tab"
                        :aria-selected="$store.lightbox.activeTab === tIdx"
                        class="flex-1 flex flex-col items-center py-1.5 rounded-lg text-xs font-mono transition-colors focus:outline-none focus:ring-2 focus:ring-stone-400"
                        :class="$store.lightbox.activeTab === tIdx
                            ? 'bg-stone-700 text-white'
                            : 'text-stone-400 hover:bg-stone-800 hover:text-stone-300'"
                    >
                        <span x-text="tab.name" class="font-semibold tracking-wide"></span>
                        <kbd class="text-[10px] opacity-60" x-text="'(' + tab.key + ')'"></kbd>
                    </button>
                </template>
              </div>
            </template>
            <template x-if="$store.lightbox.isExpanded()">
              <div class="flex items-center gap-2 py-1.5">
                <button
                  @click="$store.lightbox.collapseExpanded()"
                  class="px-2 py-1 bg-stone-700 hover:bg-stone-600 text-stone-200 rounded-md text-xs font-mono transition-colors focus:outline-none focus:ring-2 focus:ring-stone-400"
                  aria-label="Back to quick slots"
                >&larr; Back</button>
                <span class="text-xs text-stone-400" x-text="'Slot ' + $store.lightbox.quickTagKeyLabel($store.lightbox.expandedSlotIndex) + ' tags'"></span>
                <span class="text-[10px] text-stone-600 ml-auto">ESC / 0 to close</span>
              </div>
            </template>

            <!-- Divider -->
            <div class="border-t border-stone-700"></div>

            <!-- NORMAL GRID (not expanded) -->
            <template x-if="!$store.lightbox.isExpanded()">
            <div class="grid grid-cols-3 gap-2" role="tabpanel">
                <template x-for="(_, vIdx) in $store.lightbox._numpadOrder" :key="vIdx">
                    <div x-data="{
                        get idx() { return $store.lightbox.numpadIndex(vIdx) },
                        get slot() { return $store.lightbox.getActiveTabSlots()[this.idx] },
                        get tags() {
                            const s = this.slot;
                            if (!s) return [];
                            return Array.isArray(s) ? s : [s];
                        },
                        get matchState() { return $store.lightbox.slotMatchState(this.idx) },
                        get isEditing() { return $store.lightbox.editingSlotIndex === this.idx && $store.lightbox.isQuickTab() },
                        tagNames() { return this.tags.map(t => t.name ?? t.Name).join(', ') },
                    }">
                        <!-- EDITING MODE: pills + autocomplete -->
                        <template x-if="isEditing">
                            <div class="w-full min-h-[4.5rem] rounded-lg border-2 border-stone-500 bg-stone-800 p-2 flex flex-col gap-1.5"
                                 @click.outside="$store.lightbox.editingSlotIndex = null"
                                 @keydown.escape.stop="$store.lightbox.editingSlotIndex = null"
                                 @focusout="$nextTick(() => { if (!$el.contains(document.activeElement)) $store.lightbox.editingSlotIndex = null })">
                                <kbd class="text-xs font-mono text-stone-500 self-center" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                                <!-- Tag pills -->
                                <div class="flex flex-wrap gap-1">
                                    <template x-for="t in tags" :key="t.id">
                                        <span class="inline-flex items-center gap-0.5 bg-stone-700 text-stone-200 rounded px-1.5 py-0.5 text-xs">
                                            <span x-text="t.name" class="truncate max-w-[6rem]"></span>
                                            <button
                                                @click.stop="$store.lightbox.removeTagFromSlot(idx, t.id)"
                                                class="hover:text-red-400 focus:outline-none focus:text-red-400"
                                                :aria-label="'Remove ' + t.name + ' from slot'"
                                            >&times;</button>
                                        </span>
                                    </template>
                                </div>
                                <!-- Autocomplete for adding more tags (seed with existing slot tags to exclude them) -->
                                <div x-data="autocompleter({
                                         selectedResults: tags.map(t => ({ID: t.id, Name: t.name})),
                                         url: '/v1/tags',
                                         standalone: true,
                                         sortBy: 'most_used_resource',
                                         max: 0,
                                         onSelect: (tag) => { $store.lightbox.addTagToSlot(idx, tag); }
                                     })">
                                    <div class="relative">
                                        <input
                                            x-ref="autocompleter"
                                            type="text"
                                            x-bind="inputEvents"
                                            x-init="$nextTick(() => $el.focus())"
                                            class="w-full px-1.5 py-1 bg-stone-900/50 border border-stone-600 rounded text-xs text-white placeholder-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-400"
                                            placeholder="Add tag..."
                                            :aria-label="'Add tag to slot ' + $store.lightbox.quickTagKeyLabel(idx)"
                                            autocomplete="off"
                                            role="combobox"
                                            aria-autocomplete="list"
                                            :aria-expanded="dropdownActive && results.length > 0"
                                        >
                                        <div x-ref="dropdown" popover
                                             class="bg-stone-800 border border-stone-700 rounded-md shadow-lg max-h-48 overflow-y-auto"
                                             role="listbox">
                                            <template x-for="(tag, rIndex) in results" :key="tag.ID">
                                                <div
                                                    @mousedown.prevent="selectedIndex = rIndex; pushVal($event)"
                                                    @mouseover="selectedIndex = rIndex"
                                                    role="option"
                                                    :aria-selected="rIndex === selectedIndex"
                                                    class="px-3 py-2 cursor-pointer text-sm"
                                                    :class="rIndex === selectedIndex ? 'bg-amber-700 text-white' : 'text-stone-300 hover:bg-stone-700'"
                                                >
                                                    <span x-text="tag.Name"></span>
                                                </div>
                                            </template>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </template>

                        <!-- DISPLAY MODE: filled slot -->
                        <template x-if="!isEditing && tags.length > 0">
                            <div class="group relative w-full aspect-[4/3] rounded-lg transition-colors"
                                :class="{
                                    'bg-green-900/30 border-2 border-green-600/60 text-green-300 hover:bg-red-900/30 hover:border-red-600/60 hover:text-red-300': matchState === 'all',
                                    'bg-amber-900/20 border-2 border-amber-600/50 text-amber-300 hover:bg-green-900/30 hover:border-green-600/60 hover:text-green-300': matchState === 'some',
                                    'bg-stone-800 border border-stone-700 text-stone-300 hover:bg-amber-900/20 hover:border-amber-700 hover:text-amber-300': matchState === 'none',
                                }"
                            >
                                <button
                                    @click="tags.length <= 1 && $store.lightbox.toggleTabTag(idx)"
                                    @mousedown="tags.length > 1 && $store.lightbox.handleSlotMousedown(idx)"
                                    @mouseup="tags.length > 1 && $store.lightbox.handleSlotMouseup(idx)"
                                    @mouseleave="tags.length > 1 && $store.lightbox.handleSlotMouseleave(idx)"
                                    class="w-full h-full flex flex-col items-center justify-center gap-1 focus:outline-none focus:ring-2 focus:ring-stone-400 rounded-lg px-1.5"
                                    :aria-label="(matchState === 'all' ? 'Remove ' : 'Add ') + tagNames() + (matchState === 'some' ? ' (partially active: ' + tags.filter(t => $store.lightbox.isTagOnResource(t.id ?? t.ID)).length + ' of ' + tags.length + ')' : '')"
                                    :aria-description="tags.length > 1 ? 'Hold to expand individual tags' : null"
                                >
                                    <kbd class="text-sm font-mono text-stone-500" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                                    <span class="text-xs font-semibold line-clamp-2 max-w-full text-center leading-tight" x-text="tagNames()"></span>
                                </button>
                                <!-- Add button (QUICK tabs only) -->
                                <template x-if="$store.lightbox.isQuickTab()">
                                    <button
                                        @click.stop="$store.lightbox.editingSlotIndex = idx"
                                        class="absolute top-1 left-1 p-0.5 hover:bg-white/10 rounded-full opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity focus:outline-none focus:ring-1 focus:ring-white"
                                        :aria-label="'Add tags to slot ' + $store.lightbox.quickTagKeyLabel(idx)"
                                    >
                                        <svg class="w-3 h-3 text-stone-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
                                        </svg>
                                    </button>
                                </template>
                                <!-- Clear button (QUICK tabs only) -->
                                <template x-if="$store.lightbox.isQuickTab()">
                                    <button
                                        @click.stop="$store.lightbox.clearQuickTagSlot(idx)"
                                        class="absolute top-1 right-1 p-0.5 hover:bg-white/10 rounded-full opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity focus:outline-none focus:ring-1 focus:ring-white"
                                        :aria-label="'Clear slot ' + $store.lightbox.quickTagKeyLabel(idx)"
                                    >
                                        <svg class="w-3 h-3 text-stone-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                        </svg>
                                    </button>
                                </template>
                                <!-- Hold progress bar (multi-tag slots only) -->
                                <div
                                  x-show="tags.length > 1 && $store.lightbox._longPressSlotIdx === idx"
                                  x-cloak
                                  class="quick-tag-hold-bar"
                                  x-effect="if ($store.lightbox._longPressSlotIdx === idx) { $el.classList.remove('animating'); $el.offsetWidth; $el.classList.add('animating'); } else { $el.classList.remove('animating'); }"
                                  aria-hidden="true"
                                ></div>
                            </div>
                        </template>

                        <!-- EMPTY SLOT -->
                        <template x-if="!isEditing && tags.length === 0">
                            <div class="w-full aspect-[4/3] rounded-lg border border-dashed border-stone-700 flex flex-col items-center justify-center gap-1"
                                 :class="{ 'cursor-pointer hover:border-stone-500': $store.lightbox.isQuickTab() }"
                                 @click="$store.lightbox.isQuickTab() && ($store.lightbox.editingSlotIndex = idx)">
                                <kbd class="text-sm font-mono text-stone-600" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                                <!-- Empty label for non-QUICK tabs -->
                                <span x-show="!$store.lightbox.isQuickTab()" x-cloak class="text-[10px] text-stone-600 italic">empty</span>
                                <!-- Assign hint for QUICK tabs -->
                                <span x-show="$store.lightbox.isQuickTab()" x-cloak class="text-[10px] text-stone-500 italic">click to assign</span>
                            </div>
                        </template>
                    </div>
                </template>
            </div>
            </template>

            <!-- EXPANDED GRID (individual tags from one slot) -->
            <template x-if="$store.lightbox.isExpanded()">
              <div class="grid grid-cols-3 gap-2" role="region" aria-label="Expanded slot tags">
                <template x-for="(_, vIdx) in $store.lightbox._numpadOrder" :key="vIdx">
                  <div x-data="{
                    get idx() { return $store.lightbox.numpadIndex(vIdx) },
                    get tag() { return $store.lightbox.expandedTags()[this.idx] },
                    get isOn() { return this.tag ? $store.lightbox.isTagOnResource(this.tag.id ?? this.tag.ID) : false },
                    tagName() { return this.tag ? (this.tag.name ?? this.tag.Name) : '' },
                  }">
                    <!-- FILLED: tag exists at this position -->
                    <template x-if="tag">
                      <div class="group relative w-full aspect-[4/3] rounded-lg transition-colors"
                        :class="{
                          'bg-green-900/30 border-2 border-green-600/60 text-green-300 hover:bg-red-900/30 hover:border-red-600/60 hover:text-red-300': isOn,
                          'bg-stone-800 border border-stone-700 text-stone-300 hover:bg-amber-900/20 hover:border-amber-700 hover:text-amber-300': !isOn,
                        }"
                      >
                        <button
                          @click="$store.lightbox.toggleExpandedTag(idx)"
                          class="w-full h-full flex flex-col items-center justify-center gap-1 focus:outline-none focus:ring-2 focus:ring-stone-400 rounded-lg px-1.5"
                          :aria-label="(isOn ? 'Remove ' : 'Add ') + tagName()"
                        >
                          <kbd class="text-sm font-mono text-stone-500" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                          <span class="text-xs font-semibold line-clamp-2 max-w-full text-center leading-tight" x-text="tagName()"></span>
                        </button>
                      </div>
                    </template>
                    <!-- EMPTY: no tag at this position -->
                    <template x-if="!tag">
                      <div class="w-full aspect-[4/3] rounded-lg border border-dashed border-stone-700/30"></div>
                    </template>
                  </div>
                </template>
              </div>
            </template>
        </div>
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
        class="fixed md:absolute inset-0 md:inset-auto md:top-0 md:right-0 md:bottom-0 bg-stone-900 md:bg-stone-900/95 md:backdrop-blur-sm text-white overflow-y-auto z-30"
        :class="$store.lightbox.quickTagPanelOpen ? 'md:w-[320px]' : 'md:w-[400px]'"
        @click.stop
    >
        <!-- Panel header -->
        <div class="sticky top-0 bg-stone-900 md:bg-stone-900/95 border-b border-stone-700 p-4 flex items-center justify-between z-10">
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
                        class="absolute inset-0 bg-stone-900/50 flex items-center justify-center z-10 rounded"
                    >
                        <svg class="w-6 h-6 animate-spin text-white/70" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                    </div>

                    <!-- Name field -->
                    <div>
                        <label for="lightbox-edit-name" class="block text-sm font-medium font-mono text-stone-300 mb-1.5">Name</label>
                        <input
                            type="text"
                            id="lightbox-edit-name"
                            :value="$store.lightbox.resourceDetails?.Name || ''"
                            @blur="$store.lightbox.updateName($event.target.value)"
                            @keydown.enter="$event.target.blur()"
                            @keydown.escape.stop="$event.target.blur()"
                            class="w-full px-3 py-2 bg-stone-800 border border-stone-700 rounded-md text-white placeholder-stone-500 focus:outline-none focus:ring-2 focus:ring-stone-400 focus:border-transparent"
                            placeholder="Resource name"
                        >
                    </div>

                    <!-- Description field -->
                    <div>
                        <label for="lightbox-edit-description" class="block text-sm font-medium font-mono text-stone-300 mb-1.5">Description</label>
                        <textarea
                            id="lightbox-edit-description"
                            :value="$store.lightbox.resourceDetails?.Description || ''"
                            @blur="$store.lightbox.updateDescription($event.target.value)"
                            @keydown.escape.stop="$event.target.blur()"
                            rows="4"
                            class="w-full px-3 py-2 bg-stone-800 border border-stone-700 rounded-md text-white font-sans placeholder-stone-500 focus:outline-none focus:ring-2 focus:ring-stone-400 focus:border-transparent resize-y"
                            placeholder="Add a description..."
                        ></textarea>
                    </div>

                    <!-- Resource Category section -->
                    <template x-if="$store.lightbox.resourceDetails?.resourceCategory">
                        <div class="space-y-2">
                            <label class="block text-sm font-medium font-mono text-stone-300 mb-1.5">Category</label>
                            <a
                                :href="'/resourceCategory?id=' + $store.lightbox.resourceDetails.resourceCategory.ID"
                                class="text-amber-400 hover:text-amber-300 text-sm"
                                x-text="$store.lightbox.resourceDetails.resourceCategory.Name"
                            ></a>

                            {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal
                            information management application designed to run on private/internal networks
                            with no authentication layer. All users are trusted, and CustomSidebar is an
                            intentional extension point for admin-authored HTML templates.{% endcomment %}
                            <template x-if="$store.lightbox.resourceDetails.resourceCategory.CustomSidebar">
                                <div
                                    x-data="{ entity: $store.lightbox.resourceDetails }"
                                    x-html="$store.lightbox.resourceDetails.resourceCategory.CustomSidebar"
                                    class="text-sm text-stone-300 font-sans"
                                ></div>
                            </template>
                        </div>
                    </template>

                    <!-- Link to full details page -->
                    <div class="pt-4 border-t border-stone-700">
                        <a
                            :href="'/resource?id=' + $store.lightbox.getCurrentItem()?.id"
                            class="inline-flex items-center gap-2 text-amber-400 hover:text-amber-300 text-sm"
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
        :class="{
            'lg:-translate-x-[calc(50%+200px)]': $store.lightbox.editPanelOpen && !$store.lightbox.quickTagPanelOpen,
            'lg:translate-x-[calc(-50%+200px)]': !$store.lightbox.editPanelOpen && $store.lightbox.quickTagPanelOpen
        }"
    >
        Loading more items...
    </div>

    <!-- Zoom preset popover (top layer, no clipping) -->
    <div id="zoom-preset-popover" popover></div>
</div>
