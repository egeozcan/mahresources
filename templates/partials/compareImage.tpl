<div class="bg-white shadow rounded-lg p-4" x-data="imageCompare({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}'
})">
    <!-- Mode selector -->
    <div class="flex space-x-2 mb-4 border-b pb-4">
        <button @click="mode = 'side-by-side'"
                :class="mode === 'side-by-side' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Side-by-side</button>
        <button @click="mode = 'slider'"
                :class="mode === 'slider' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Slider</button>
        <button @click="mode = 'onion'"
                :class="mode === 'onion' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Onion skin</button>
        <button @click="mode = 'toggle'"
                :class="mode === 'toggle' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Toggle</button>
        <div class="flex-grow"></div>
        <button @click="swapSides()" class="px-4 py-2 bg-gray-200 rounded">Swap sides</button>
    </div>

    <!-- Side-by-side mode -->
    <div x-show="mode === 'side-by-side'" class="grid grid-cols-2 gap-4">
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600">v{{ comparison.Version1.VersionNumber }}</div>
            <img :src="leftUrl" class="max-w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        </div>
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600">v{{ comparison.Version2.VersionNumber }}</div>
            <img :src="rightUrl" class="max-w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
        </div>
    </div>

    <!-- Slider mode -->
    <div x-show="mode === 'slider'" class="relative border rounded overflow-hidden select-none" style="max-height: 600px;" x-ref="sliderContainer">
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
                <span class="text-gray-400">&#x22EE;</span>
            </div>
        </div>
    </div>

    <!-- Onion skin mode -->
    <div x-show="mode === 'onion'" class="relative border rounded overflow-hidden">
        <img :src="leftUrl" class="w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        <img :src="rightUrl" class="absolute inset-0 w-full h-auto"
             :style="'opacity: ' + (opacity / 100)"
             alt="Version {{ comparison.Version2.VersionNumber }}">
        <div class="absolute bottom-4 left-1/2 -translate-x-1/2 bg-white/80 rounded px-4 py-2 flex items-center space-x-3">
            <span class="text-sm">v{{ comparison.Version1.VersionNumber }}</span>
            <input type="range" min="0" max="100" x-model="opacity" class="w-48">
            <span class="text-sm">v{{ comparison.Version2.VersionNumber }}</span>
        </div>
    </div>

    <!-- Toggle mode -->
    <div x-show="mode === 'toggle'" class="relative border rounded overflow-hidden cursor-pointer" tabindex="0" role="button" @click="toggleSide()" @keydown.space.prevent="toggleSide()">
        <div class="absolute top-2 right-2 bg-white/80 rounded px-2 py-1 text-sm font-medium" x-text="showLeft ? 'v{{ comparison.Version1.VersionNumber }}' : 'v{{ comparison.Version2.VersionNumber }}'"></div>
        <img x-show="showLeft" :src="leftUrl" class="w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        <img x-show="!showLeft" :src="rightUrl" class="w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
        <div class="absolute bottom-4 left-1/2 -translate-x-1/2 bg-white/80 rounded px-4 py-2 text-sm">
            Click or press Space to toggle
        </div>
    </div>
</div>
