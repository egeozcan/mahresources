<section x-show="$store.lightbox.versionPanelOpen" x-cloak id="lightbox-version-panel" data-version-panel
         aria-label="Version history" :aria-busy="$store.lightbox.detailsBusy()"
         @touchstart.stop @touchmove.stop @touchend.stop
         class="shrink-0 bg-stone-900 text-white border-b border-stone-700 p-2 md:p-3">
    <div class="flex items-center gap-3 pr-16 mb-2 text-sm">
        <h2 class="font-semibold">Versions</h2>
        <a :href="'/resource?id=' + $store.lightbox.getCurrentItem()?.id" class="text-amber-300 underline">Resource page</a>
        <button type="button" @click="$store.lightbox.toggleVersionPanel()" aria-label="Close version history"
                class="ml-auto rounded px-2 hover:bg-stone-700 focus:ring-2 focus:ring-white">Close</button>
    </div>
    <p x-show="$store.lightbox.detailsLoading && !$store.lightbox.versionEntries().length" class="text-sm text-stone-300">Loading versions…</p>
    <div x-show="$store.lightbox.detailsFailed()" class="text-sm">
        Could not load versions.
        <button type="button" @click="$store.lightbox.fetchResourceDetails(undefined, true)" class="underline">Retry</button>
    </div>
    <p x-show="!$store.lightbox.detailsBusy() && !$store.lightbox.detailsFailed() && !$store.lightbox.versionEntries().length" class="text-sm text-stone-300">No versions recorded.</p>
    <p x-show="$store.lightbox.versionEntries().length === 1" class="text-sm text-stone-300 mb-1">This Resource has one version.</p>
    {# Mount thumbnails only while the strip is open, even when Info has cached versions. #}
    <template x-if="$store.lightbox.versionPanelOpen">
    <div role="group" aria-label="Resource versions" class="flex gap-2 overflow-x-auto pb-2">
        <template x-for="version in $store.lightbox.versionEntries()" :key="version.id">
            <button type="button" @click="$store.lightbox.selectVersion(version)"
                    :disabled="!$store.lightbox.isVersionDisplayable(version) || $store.lightbox.rotating"
                    :aria-current="$store.lightbox.displayedVersionId() === version.id ? 'true' : null"
                    :aria-label="'View version ' + version.versionNumber + (version.id === $store.lightbox.currentVersionId() ? ', current' : '') + (!$store.lightbox.isVersionDisplayable(version) ? ', ' + version.contentType + ', unavailable in viewer' : '')"
                    :title="version.comment || version.contentType"
                    :class="$store.lightbox.displayedVersionId() === version.id ? 'border-amber-400 bg-stone-800' : 'border-stone-600'"
                    class="shrink-0 rounded border-2 p-1 text-center disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-white">
                <template x-if="version.contentType?.startsWith('image/')">
                    <picture>
                        <source media="(min-width: 768px)" :srcset="$store.lightbox.versionThumbnailUrl(version, 96)">
                        <img :src="$store.lightbox.versionThumbnailUrl(version, 64)" alt="" loading="lazy" class="h-16 w-16 md:h-24 md:w-24 object-contain mx-auto">
                    </picture>
                </template>
                <template x-if="!version.contentType?.startsWith('image/')">
                    <span class="h-16 w-16 md:h-24 md:w-24 flex items-center justify-center text-xs break-all" x-text="version.contentType?.startsWith('video/') ? 'Video' : (version.contentType || 'File')"></span>
                </template>
                <span class="block text-sm" x-text="'v' + version.versionNumber"></span>
                <span x-show="version.id === $store.lightbox.currentVersionId()" class="hidden md:block text-xs text-amber-300">current</span>
                <span class="hidden md:block text-xs text-stone-300" x-text="$store.lightbox.versionDate(version)"></span>
                <span class="hidden md:block text-xs text-stone-300" x-text="$store.lightbox.formatVersionSize(version.fileSize)"></span>
            </button>
        </template>
    </div>
    </template>
</section>
