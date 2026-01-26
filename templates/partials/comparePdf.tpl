<div class="bg-white shadow rounded-lg p-4" x-data="{ loaded: false }">
    <div class="flex items-center justify-between mb-4 border-b pb-4">
        <h3 class="text-lg font-medium">PDF Comparison</h3>
        <button @click="loaded = true" x-show="!loaded"
                class="px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">
            Load in viewer
        </button>
    </div>

    <!-- Thumbnails before loading -->
    <div x-show="!loaded" class="grid grid-cols-2 gap-6">
        <div class="border rounded p-4 text-center">
            <div class="w-24 h-32 bg-red-100 mx-auto mb-3 flex items-center justify-center">
                <span class="text-red-600 font-bold">PDF</span>
            </div>
            <p class="font-medium">v{{ comparison.Version1.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-gray-200 rounded hover:bg-gray-300">
                Download
            </a>
        </div>
        <div class="border rounded p-4 text-center">
            <div class="w-24 h-32 bg-red-100 mx-auto mb-3 flex items-center justify-center">
                <span class="text-red-600 font-bold">PDF</span>
            </div>
            <p class="font-medium">v{{ comparison.Version2.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-gray-200 rounded hover:bg-gray-300">
                Download
            </a>
        </div>
    </div>

    <!-- Iframes after loading -->
    <div x-show="loaded" class="grid grid-cols-2 gap-4">
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600 flex justify-between">
                <span>v{{ comparison.Version1.VersionNumber }}</span>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}" class="text-indigo-600 hover:underline">Download</a>
            </div>
            <iframe src="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
                    class="w-full h-[600px]"></iframe>
        </div>
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600 flex justify-between">
                <span>v{{ comparison.Version2.VersionNumber }}</span>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}" class="text-indigo-600 hover:underline">Download</a>
            </div>
            <iframe src="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
                    class="w-full h-[600px]"></iframe>
        </div>
    </div>
</div>
