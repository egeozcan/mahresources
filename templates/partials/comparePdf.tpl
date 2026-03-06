<div class="bg-white shadow rounded-lg p-4" x-data="{ loaded: false }">
    <div class="flex items-center justify-between mb-4 border-b pb-4">
        <h3 class="text-lg font-medium">PDF Comparison</h3>
        <button @click="loaded = true" x-show="!loaded"
                class="px-4 py-2 bg-teal-700 text-white rounded hover:bg-teal-800 text-sm font-medium">
            Load in viewer
        </button>
    </div>

    <!-- Thumbnails before loading -->
    <div x-show="!loaded" class="grid grid-cols-2 gap-6">
        <div class="border rounded p-4 text-center">
            <div class="compare-panel-header--old rounded-t -mx-4 -mt-4 mb-3">OLD — v{{ comparison.Version1.VersionNumber }}</div>
            <svg xmlns="http://www.w3.org/2000/svg" width="48" height="56" viewBox="0 0 24 28" fill="none" stroke="#6b7280" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="mx-auto mb-3" aria-hidden="true">
                <path d="M14 2H6a2 2 0 00-2 2v20a2 2 0 002 2h12a2 2 0 002-2V8l-6-6z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/><line x1="8" y1="21" x2="12" y2="21"/>
            </svg>
            <p class="text-sm text-gray-500">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-gray-200 rounded hover:bg-gray-300 text-sm">
                Download
            </a>
        </div>
        <div class="border rounded p-4 text-center">
            <div class="compare-panel-header--new rounded-t -mx-4 -mt-4 mb-3">NEW — v{{ comparison.Version2.VersionNumber }}</div>
            <svg xmlns="http://www.w3.org/2000/svg" width="48" height="56" viewBox="0 0 24 28" fill="none" stroke="#6b7280" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="mx-auto mb-3" aria-hidden="true">
                <path d="M14 2H6a2 2 0 00-2 2v20a2 2 0 002 2h12a2 2 0 002-2V8l-6-6z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/><line x1="8" y1="21" x2="12" y2="21"/>
            </svg>
            <p class="text-sm text-gray-500">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-gray-200 rounded hover:bg-gray-300 text-sm">
                Download
            </a>
        </div>
    </div>

    <!-- Iframes after loading -->
    <div x-show="loaded" class="grid grid-cols-2 gap-4">
        <div class="border rounded overflow-hidden">
            <div class="compare-panel-header--old flex justify-between items-center">
                <span>OLD — v{{ comparison.Version1.VersionNumber }}</span>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}" class="text-red-900/60 hover:underline text-xs">Download</a>
            </div>
            <iframe src="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
                    class="w-full h-[600px]"></iframe>
        </div>
        <div class="border rounded overflow-hidden">
            <div class="compare-panel-header--new flex justify-between items-center">
                <span>NEW — v{{ comparison.Version2.VersionNumber }}</span>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}" class="text-green-900/60 hover:underline text-xs">Download</a>
            </div>
            <iframe src="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
                    class="w-full h-[600px]"></iframe>
        </div>
    </div>
</div>
