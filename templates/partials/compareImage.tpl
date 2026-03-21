<div class="bg-white shadow rounded-lg p-4" x-data="imageCompare({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}'
})">
    <!-- Mode selector -->
    <div class="flex flex-wrap items-center gap-3 mb-4 border-b pb-4">
        <div class="compare-segmented-control" role="radiogroup" aria-label="Comparison mode">
            <button @click="mode = 'side-by-side'" role="radio" :aria-checked="mode === 'side-by-side'"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="3" width="8" height="18" rx="1"/><rect x="14" y="3" width="8" height="18" rx="1"/></svg>
                <span class="compare-seg-label">Side by side</span>
            </button>
            <button @click="mode = 'slider'" role="radio" :aria-checked="mode === 'slider'"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="12" y1="2" x2="12" y2="22"/><polyline points="8 6 12 2 16 6"/><polyline points="8 18 12 22 16 18"/></svg>
                <span class="compare-seg-label">Slider</span>
            </button>
            <button @click="mode = 'onion'" role="radio" :aria-checked="mode === 'onion'"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="9" cy="12" r="7"/><circle cx="15" cy="12" r="7"/></svg>
                <span class="compare-seg-label">Onion skin</span>
            </button>
            <button @click="mode = 'toggle'" role="radio" :aria-checked="mode === 'toggle'"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="1" y="5" width="22" height="14" rx="7"/><circle cx="16" cy="12" r="4"/></svg>
                <span class="compare-seg-label">Toggle</span>
            </button>
        </div>
        <button @click="swapSides()" class="compare-swap-btn-sm" aria-label="Swap sides">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 16V4m0 0L3 8m4-4l4 4M17 8v12m0 0l4-4m-4 4l-4-4"/></svg>
            Swap
        </button>
    </div>

    <!-- Side-by-side mode -->
    <div x-show="mode === 'side-by-side'" class="grid grid-cols-2 gap-4">
        <div class="border rounded overflow-hidden">
            <div class="compare-panel-header--old">{{ label1 }}{% if not crossResource %} — v{{ comparison.Version1.VersionNumber }}{% endif %}</div>
            <img :src="leftUrl" class="max-w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        </div>
        <div class="border rounded overflow-hidden">
            <div class="compare-panel-header--new">{{ label2 }}{% if not crossResource %} — v{{ comparison.Version2.VersionNumber }}{% endif %}</div>
            <img :src="rightUrl" class="max-w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
        </div>
    </div>

    <!-- Slider mode -->
    <div x-show="mode === 'slider'" class="relative border rounded overflow-hidden select-none" x-ref="sliderContainer">
        <img :src="rightUrl" class="w-full h-auto pointer-events-none" alt="Version {{ comparison.Version2.VersionNumber }}">
        <div class="absolute inset-0 overflow-hidden pointer-events-none" :style="'clip-path: inset(0 ' + (100 - sliderPos) + '% 0 0)'">
            <img :src="leftUrl" class="w-full h-auto"
                 alt="Version {{ comparison.Version1.VersionNumber }}">
        </div>
        <div class="absolute inset-y-0 bg-white w-1 cursor-ew-resize z-10"
             :style="'left: ' + sliderPos + '%'"
             @mousedown="startSliderDrag"
             @touchstart.prevent="startSliderDrag">
            <div class="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-6 h-12 bg-white rounded shadow flex items-center justify-center">
                <span class="text-stone-400">&#x22EE;</span>
            </div>
        </div>
        <div class="absolute top-2 left-2"><span class="compare-side-label--old">{{ label1 }}</span></div>
        <div class="absolute top-2 right-2"><span class="compare-side-label--new">{{ label2 }}</span></div>
    </div>

    <!-- Onion skin mode -->
    <div x-show="mode === 'onion'">
        <div class="relative border rounded overflow-hidden">
            <img :src="leftUrl" class="w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
            <img :src="rightUrl" class="absolute inset-0 w-full h-auto"
                 :style="'opacity: ' + (opacity / 100)"
                 alt="Version {{ comparison.Version2.VersionNumber }}">
        </div>
        <div class="sticky bottom-0 z-20 flex items-center justify-center gap-3 py-2 px-4 bg-white/90 backdrop-blur border-t border-stone-200">
            <span class="compare-side-label--old">{{ label1 }}</span>
            <input type="range" min="0" max="100" x-model="opacity" class="w-48" aria-label="Onion skin opacity">
            <span class="compare-side-label--new">{{ label2 }}</span>
        </div>
    </div>

    <!-- Toggle mode -->
    <div x-show="mode === 'toggle'" class="relative border rounded overflow-hidden cursor-pointer" tabindex="0" role="button" @click="toggleSide()" @keydown.space.prevent="toggleSide()">
        <div class="absolute top-2 right-2 z-10">
            <span x-show="showLeft" class="compare-side-label--old">{{ label1 }}{% if not crossResource %} — v{{ comparison.Version1.VersionNumber }}{% endif %}</span>
            <span x-show="!showLeft" class="compare-side-label--new">{{ label2 }}{% if not crossResource %} — v{{ comparison.Version2.VersionNumber }}{% endif %}</span>
        </div>
        <img x-show="showLeft" :src="leftUrl" class="w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        <img x-show="!showLeft" :src="rightUrl" class="w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
    </div>
</div>
