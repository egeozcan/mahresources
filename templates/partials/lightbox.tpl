<div
    x-data
    x-show="$store.lightbox.isOpen"
    x-cloak
    x-trap.noscroll="$store.lightbox.isOpen"
    @keydown.escape.window="$store.lightbox.isOpen && $store.lightbox.close()"
    @keydown.arrow-left.window="$store.lightbox.isOpen && $store.lightbox.prev()"
    @keydown.arrow-right.window="$store.lightbox.isOpen && $store.lightbox.next()"
    @touchstart="$store.lightbox.handleTouchStart($event)"
    @touchend="$store.lightbox.handleTouchEnd($event)"
    class="fixed inset-0 z-50 flex items-center justify-center"
    role="dialog"
    aria-modal="true"
    :aria-label="$store.lightbox.getCurrentItem()?.name || 'Media viewer'"
>
    <!-- Backdrop -->
    <div
        class="absolute inset-0 bg-black/90"
        @click="$store.lightbox.close()"
    ></div>

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
                class="max-h-[90vh] max-w-[90vw] object-contain"
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
                class="max-h-[90vh] max-w-[90vw]"
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
        class="absolute left-4 top-1/2 -translate-y-1/2 p-3 bg-white/10 hover:bg-white/20 disabled:opacity-30 disabled:cursor-not-allowed rounded-full text-white transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
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
        class="absolute right-4 top-1/2 -translate-y-1/2 p-3 bg-white/10 hover:bg-white/20 disabled:opacity-30 disabled:cursor-not-allowed rounded-full text-white transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
        aria-label="Next"
    >
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
        </svg>
    </button>

    <!-- Close button -->
    <button
        @click.stop="$store.lightbox.close()"
        class="absolute top-4 right-4 p-2 bg-white/10 hover:bg-white/20 rounded-full text-white transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
        aria-label="Close"
    >
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
        </svg>
    </button>

    <!-- Page loading indicator -->
    <div
        x-show="$store.lightbox.pageLoading"
        x-transition
        class="absolute bottom-20 left-1/2 -translate-x-1/2 px-4 py-2 bg-white/10 backdrop-blur rounded text-white text-sm"
    >
        Loading more items...
    </div>

    <!-- Bottom bar with counter and name -->
    <div class="absolute bottom-4 left-0 right-0 flex justify-between items-center px-4 text-white text-sm">
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
            class="bg-black/50 px-3 py-1 rounded max-w-[50vw] truncate"
            x-text="$store.lightbox.getCurrentItem()?.name"
        ></div>

        <!-- Link to resource page -->
        <a
            :href="'/resource?id=' + $store.lightbox.getCurrentItem()?.id"
            @click.stop
            class="bg-black/50 px-3 py-1 rounded hover:bg-white/20 transition-colors"
            title="Open resource details"
        >
            Details
        </a>
    </div>
</div>
