<div class="bg-white shadow rounded-lg p-4" x-data="textDiff({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}'
})">
    <!-- Mode selector -->
    <div class="flex flex-wrap items-center gap-3 mb-4 border-b pb-4">
        <div class="compare-segmented-control" role="radiogroup" aria-label="Diff mode">
            <button @click="mode = 'unified'" role="radio" :aria-checked="mode === 'unified'"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="7" y1="8" x2="17" y2="8"/><line x1="7" y1="12" x2="17" y2="12"/><line x1="7" y1="16" x2="13" y2="16"/></svg>
                <span class="compare-seg-label">Unified</span>
            </button>
            <button @click="mode = 'split'" role="radio" :aria-checked="mode === 'split'"
                    class="compare-seg-btn">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="3" width="8" height="18" rx="1"/><rect x="14" y="3" width="8" height="18" rx="1"/></svg>
                <span class="compare-seg-label">Side by side</span>
            </button>
        </div>
        <div class="flex-grow"></div>
        <span class="text-sm text-stone-600" x-show="stats.added || stats.removed">
            <span class="text-green-600">+<span x-text="stats.added"></span></span>
            <span class="text-red-600 ml-2">-<span x-text="stats.removed"></span></span>
            lines
        </span>
    </div>

    <!-- Loading state -->
    <div x-show="loading" class="text-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-teal-600 mx-auto"></div>
        <p class="mt-2 text-stone-600">Loading files...</p>
    </div>

    <!-- Error state -->
    <div x-show="error" class="bg-red-50 border border-red-200 rounded p-4 text-red-800" x-text="error"></div>

    <!-- Unified diff view -->
    <div x-show="!loading && !error && mode === 'unified'" class="font-mono text-sm overflow-x-auto">
        <table class="w-full">
            <template x-for="(line, index) in unifiedDiff" :key="index">
                <tr :class="{
                    'bg-red-50': line.type === 'removed',
                    'bg-green-50': line.type === 'added',
                    'bg-stone-50': line.type === 'context'
                }">
                    <td class="text-stone-400 text-right pr-2 select-none w-12" x-text="line.leftNum || ''"></td>
                    <td class="text-stone-400 text-right pr-2 select-none w-12 border-r" x-text="line.rightNum || ''"></td>
                    <td class="pl-2">
                        <span :class="{
                            'text-red-600': line.type === 'removed',
                            'text-green-600': line.type === 'added'
                        }" x-text="line.prefix"></span>
                        <span x-text="line.content" class="whitespace-pre"></span>
                    </td>
                </tr>
            </template>
        </table>
    </div>

    <!-- Split diff view -->
    <div x-show="!loading && !error && mode === 'split'" class="grid grid-cols-2 gap-0 font-mono text-sm overflow-x-auto">
        <div class="border-r">
            <div class="compare-panel-header--old sticky top-0 z-10">{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</div>
            <table class="w-full">
                <template x-for="(line, index) in splitLeft" :key="index">
                    <tr :class="{'bg-red-50': line.changed}">
                        <td class="text-stone-400 text-right pr-2 select-none w-12" x-text="line.num || ''"></td>
                        <td class="pl-2 whitespace-pre" x-text="line.content"></td>
                    </tr>
                </template>
            </table>
        </div>
        <div>
            <div class="compare-panel-header--new sticky top-0 z-10">{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</div>
            <table class="w-full">
                <template x-for="(line, index) in splitRight" :key="index">
                    <tr :class="{'bg-green-50': line.changed}">
                        <td class="text-stone-400 text-right pr-2 select-none w-12" x-text="line.num || ''"></td>
                        <td class="pl-2 whitespace-pre" x-text="line.content"></td>
                    </tr>
                </template>
            </table>
        </div>
    </div>
</div>
