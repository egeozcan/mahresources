{# `lead` is whichever image is currently on the old-coloured side and `trail` #}
{# the new-coloured one. Swapping flips which version each names, so the header #}
{# colour, the label and the alt text always describe the same file. #}
<div class="bg-white shadow rounded-lg p-4" x-data="imageCompare({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID|json }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID|json }}',
    leftLabel: {{ panelTitle1|json }},
    rightLabel: {{ panelTitle2|json }},
    leftSize: { w: {{ comparison.Version1.Width|json }}, h: {{ comparison.Version1.Height|json }} },
    rightSize: { w: {{ comparison.Version2.Width|json }}, h: {{ comparison.Version2.Height|json }} }
})">
    <!-- Mode selector -->
    <div class="flex flex-wrap items-center gap-3 mb-4 border-b pb-4">
        {# Each button carries an aria-label as well as its visible one: the visible #}
        {# label is hidden below 768px, which would otherwise leave the button with #}
        {# nothing but an aria-hidden icon and no accessible name. #}
        <div class="compare-segmented-control" role="radiogroup" aria-label="Comparison mode"
             @keydown="onRadiogroupKeydown($event, 'mode', ['side-by-side', 'slider', 'onion', 'toggle'])">
            <button @click="mode = 'side-by-side'" role="radio" :aria-checked="mode === 'side-by-side'" aria-label="Side by side"
                    :tabindex="mode === 'side-by-side' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="3" width="8" height="18" rx="1"/><rect x="14" y="3" width="8" height="18" rx="1"/></svg>
                <span class="compare-seg-label">Side by side</span>
            </button>
            <button @click="mode = 'slider'" role="radio" :aria-checked="mode === 'slider'" aria-label="Slider"
                    :tabindex="mode === 'slider' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="12" y1="2" x2="12" y2="22"/><polyline points="8 6 12 2 16 6"/><polyline points="8 18 12 22 16 18"/></svg>
                <span class="compare-seg-label">Slider</span>
            </button>
            <button @click="mode = 'onion'" role="radio" :aria-checked="mode === 'onion'" aria-label="Onion skin"
                    :tabindex="mode === 'onion' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="9" cy="12" r="7"/><circle cx="15" cy="12" r="7"/></svg>
                <span class="compare-seg-label">Onion skin</span>
            </button>
            <button @click="mode = 'toggle'" role="radio" :aria-checked="mode === 'toggle'" aria-label="Toggle"
                    :tabindex="mode === 'toggle' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="1" y="5" width="22" height="14" rx="7"/><circle cx="16" cy="12" r="4"/></svg>
                <span class="compare-seg-label">Toggle</span>
            </button>
        </div>
        {# Named for what it does. The toolbar's swap reloads the page with the two #}
        {# sides exchanged; this one only flips the images, so calling both "Swap #}
        {# sides" gave two controls one name and two behaviours. #}
        <button type="button" @click="swapSides()" class="compare-swap-btn-sm"
                :aria-pressed="swapped" aria-label="Flip which image is shown first">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 16V4m0 0L3 8m4-4l4 4M17 8v12m0 0l4-4m-4 4l-4-4"/></svg>
            Flip
        </button>
        {# Scale policy. Hidden in side-by-side, where each version has its own #}
        {# pane at its own width and a scale choice would change nothing — and #}
        {# which is the mode the page opens in, so dead controls would be the #}
        {# first thing a reader saw. Unavailability is aria-disabled rather than #}
        {# the disabled attribute: disabled removes a role="radio" from the tab #}
        {# order and breaks the roving tabindex this group depends on. #}
        <div class="compare-segmented-control" role="radiogroup" aria-label="Image scale"
             x-show="mode !== 'side-by-side'"
             :aria-disabled="!scaleAvailable"
             :title="scaleAvailable ? '' : 'Neither version reports its dimensions, so there is nothing to scale against.'"
             @keydown="onScaleKeydown($event)">
            <button @click="setScale('relative')" role="radio" :aria-checked="scale === 'relative'"
                    aria-label="Relative size" :aria-disabled="!scaleAvailable"
                    :tabindex="scale === 'relative' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="1"/><rect x="8" y="9" width="8" height="6" rx="1"/></svg>
                <span class="compare-seg-label">Relative</span>
            </button>
            <button @click="setScale('fit')" role="radio" :aria-checked="scale === 'fit'"
                    aria-label="Fit to frame" :aria-disabled="!scaleAvailable"
                    :tabindex="scale === 'fit' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="1"/><polyline points="7 9.5 9 9.5 9 11.5"/><polyline points="17 9.5 15 9.5 15 11.5"/><polyline points="7 14.5 9 14.5 9 12.5"/><polyline points="17 14.5 15 14.5 15 12.5"/></svg>
                <span class="compare-seg-label">Fit</span>
            </button>
            {# "Stretch", not "Fill": the CSS keyword reads as harmless to anyone #}
            {# not thinking in CSS, and the visible label is hidden below 768px, #}
            {# so on a phone the aria-label is the entire accessible name and is #}
            {# where the warning has to survive. #}
            <button @click="setScale('stretch')" role="radio" :aria-checked="scale === 'stretch'"
                    aria-label="Stretch to match, distorts aspect ratio" :aria-disabled="!scaleAvailable"
                    :tabindex="scale === 'stretch' ? 0 : -1"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="1"/><line x1="7" y1="12" x2="17" y2="12"/><polyline points="9 10 7 12 9 14"/><polyline points="15 10 17 12 15 14"/></svg>
                <span class="compare-seg-label">Stretch</span>
            </button>
        </div>
    </div>

    <!-- Side-by-side mode -->
    <div x-show="mode === 'side-by-side'" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {# The header colour follows the version, not the slot: after a flip the #}
        {# left pane holds the newer file, and a red "older" bar over it would #}
        {# contradict its own caption. #}
        <div class="border rounded overflow-hidden">
            <div :class="swapped ? 'compare-panel-header--new' : 'compare-panel-header--old'" x-text="leadLabel"></div>
            <img :src="leadUrl" :alt="leadAlt" class="max-w-full h-auto" data-compare-side="lead" @load="noteSizeFrom($event.target)">
        </div>
        <div class="border rounded overflow-hidden">
            <div :class="swapped ? 'compare-panel-header--old' : 'compare-panel-header--new'" x-text="trailLabel"></div>
            <img :src="trailUrl" :alt="trailAlt" class="max-w-full h-auto" data-compare-side="trail" @load="noteSizeFrom($event.target)">
        </div>
    </div>

    <!-- Slider mode -->
    <div x-show="mode === 'slider'" class="relative border rounded overflow-hidden select-none compare-overlay-box"
         x-ref="sliderContainer" :style="overlayBoxStyle">
        <img :src="trailUrl" :alt="trailAlt" class="compare-overlay-img pointer-events-none" :style="trailScale" data-compare-side="trail" @load="noteSizeFrom($event.target)">
        <div class="absolute inset-0 overflow-hidden pointer-events-none" :style="'clip-path: inset(0 ' + (100 - sliderPos) + '% 0 0)'">
            <img :src="leadUrl" :alt="leadAlt" class="compare-overlay-img" :style="leadScale" data-compare-side="lead" @load="noteSizeFrom($event.target)">
        </div>
        {# A real slider: focusable, announced, and driven by its own arrow keys. #}
        {# The handle used to be an unlabelled div reachable only through a #}
        {# document-level shortcut nothing advertised. #}
        <div class="absolute inset-y-0 bg-white w-1 cursor-ew-resize z-10 compare-slider-handle"
             role="slider"
             tabindex="0"
             aria-label="Reveal position"
             aria-valuemin="1"
             aria-valuemax="99"
             :aria-valuenow="Math.round(sliderPos)"
             :aria-valuetext="Math.round(sliderPos) + '% of ' + leadLabel"
             :style="'left: ' + sliderPos + '%'"
             @mousedown="startSliderDrag"
             @touchstart.prevent="startSliderDrag"
             @keydown.arrow-left.prevent="nudgeSlider($event.shiftKey ? -10 : -2)"
             @keydown.arrow-right.prevent="nudgeSlider($event.shiftKey ? 10 : 2)"
             @keydown.home.prevent="sliderPos = 1"
             @keydown.end.prevent="sliderPos = 99">
            <div class="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-6 h-12 bg-white rounded shadow flex items-center justify-center">
                <span class="text-stone-500" aria-hidden="true">&#x22EE;</span>
            </div>
        </div>
        <div class="absolute top-2 left-2"><span :class="swapped ? 'compare-side-label--new' : 'compare-side-label--old'" x-text="leadLabel"></span></div>
        <div class="absolute top-2 right-2"><span :class="swapped ? 'compare-side-label--old' : 'compare-side-label--new'" x-text="trailLabel"></span></div>
    </div>

    <!-- Onion skin mode -->
    <div x-show="mode === 'onion'">
        <div class="relative border rounded overflow-hidden compare-overlay-box"
             :style="overlayBoxStyle">
            <img :src="leadUrl" :alt="leadAlt" class="compare-overlay-img" :style="leadScale" data-compare-side="lead" @load="noteSizeFrom($event.target)">
            <img :src="trailUrl" :alt="trailAlt" class="compare-overlay-img compare-overlay-img--over"
                 :style="{ ...trailScale, opacity: opacity / 100 }" data-compare-side="trail" @load="noteSizeFrom($event.target)">
        </div>
        <div class="sticky bottom-0 z-20 flex items-center justify-center gap-3 py-2 px-4 bg-white/90 backdrop-blur border-t border-stone-200">
            <span :class="swapped ? 'compare-side-label--new' : 'compare-side-label--old'" x-text="leadLabel"></span>
            <input type="range" min="0" max="100" x-model.number="opacity" class="w-48" aria-label="Onion skin opacity">
            <span :class="swapped ? 'compare-side-label--old' : 'compare-side-label--new'" x-text="trailLabel"></span>
        </div>
    </div>

    <!-- Toggle mode -->
    {# A real button, so Enter and Space both work. As a div with role="button" it #}
    {# only bound Space, and Enter is the key most people reach for. #}
    <button type="button" x-show="mode === 'toggle'"
            class="relative border rounded overflow-hidden cursor-pointer block w-full p-0 compare-overlay-box"
            :style="overlayBoxStyle"
            :aria-label="'Showing ' + (showLeft ? leadLabel : trailLabel) + '. Activate to show the other.'"
            @click="toggleSide()">
        <span class="absolute top-2 right-2 z-10">
            <span x-show="showLeft" :class="swapped ? 'compare-side-label--new' : 'compare-side-label--old'" x-text="leadLabel"></span>
            <span x-show="!showLeft" :class="swapped ? 'compare-side-label--old' : 'compare-side-label--new'" x-text="trailLabel"></span>
        </span>
        <img x-show="showLeft" :src="leadUrl" :alt="leadAlt" class="compare-overlay-img" :style="leadScale" data-compare-side="lead" @load="noteSizeFrom($event.target)">
        <img x-show="!showLeft" :src="trailUrl" :alt="trailAlt" class="compare-overlay-img" :style="trailScale" data-compare-side="trail" @load="noteSizeFrom($event.target)">
    </button>
</div>
