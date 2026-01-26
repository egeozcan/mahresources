<div class="bg-white shadow rounded-lg p-4" x-data="textDiff({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}'
})">
    <!-- Mode selector -->
    <div class="flex items-center space-x-4 mb-4 border-b pb-4">
        <button @click="mode = 'unified'"
                :class="mode === 'unified' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Unified</button>
        <button @click="mode = 'split'"
                :class="mode === 'split' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Side-by-side</button>
        <div class="flex-grow"></div>
        <span class="text-sm text-gray-600" x-show="stats.added || stats.removed">
            <span class="text-green-600">+<span x-text="stats.added"></span></span>
            <span class="text-red-600 ml-2">-<span x-text="stats.removed"></span></span>
            lines
        </span>
    </div>

    <!-- Loading state -->
    <div x-show="loading" class="text-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 mx-auto"></div>
        <p class="mt-2 text-gray-600">Loading files...</p>
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
                    'bg-gray-50': line.type === 'context'
                }">
                    <td class="text-gray-400 text-right pr-2 select-none w-12" x-text="line.leftNum || ''"></td>
                    <td class="text-gray-400 text-right pr-2 select-none w-12 border-r" x-text="line.rightNum || ''"></td>
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
            <div class="bg-gray-100 px-2 py-1 text-gray-600 sticky top-0">v{{ comparison.Version1.VersionNumber }}</div>
            <table class="w-full">
                <template x-for="(line, index) in splitLeft" :key="index">
                    <tr :class="{'bg-red-50': line.changed}">
                        <td class="text-gray-400 text-right pr-2 select-none w-12" x-text="line.num || ''"></td>
                        <td class="pl-2 whitespace-pre" x-text="line.content"></td>
                    </tr>
                </template>
            </table>
        </div>
        <div>
            <div class="bg-gray-100 px-2 py-1 text-gray-600 sticky top-0">v{{ comparison.Version2.VersionNumber }}</div>
            <table class="w-full">
                <template x-for="(line, index) in splitRight" :key="index">
                    <tr :class="{'bg-green-50': line.changed}">
                        <td class="text-gray-400 text-right pr-2 select-none w-12" x-text="line.num || ''"></td>
                        <td class="pl-2 whitespace-pre" x-text="line.content"></td>
                    </tr>
                </template>
            </table>
        </div>
    </div>
</div>
