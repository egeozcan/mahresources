<div class="bg-white shadow rounded-lg p-4">
    <div class="bg-yellow-50 border border-yellow-200 rounded p-4 mb-6 text-yellow-800">
        Content preview not available for this file type. Use the download links to compare locally.
    </div>

    <div class="grid grid-cols-2 gap-6">
        <div class="border rounded p-4 text-center">
            {% if comparison.Version1.Width > 0 %}
            <img src="/v1/resource/preview?id={{ resource1.ID }}&maxX=200&maxY=200"
                 class="mx-auto mb-3 max-h-32" alt="Thumbnail">
            {% else %}
            <div class="w-24 h-24 bg-gray-200 mx-auto mb-3 flex items-center justify-center rounded">
                <span class="text-gray-500 text-xs">{{ comparison.Version1.ContentType }}</span>
            </div>
            {% endif %}
            <p class="font-medium">v{{ comparison.Version1.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">
                Download
            </a>
        </div>
        <div class="border rounded p-4 text-center">
            {% if comparison.Version2.Width > 0 %}
            <img src="/v1/resource/preview?id={{ resource2.ID }}&maxX=200&maxY=200"
                 class="mx-auto mb-3 max-h-32" alt="Thumbnail">
            {% else %}
            <div class="w-24 h-24 bg-gray-200 mx-auto mb-3 flex items-center justify-center rounded">
                <span class="text-gray-500 text-xs">{{ comparison.Version2.ContentType }}</span>
            </div>
            {% endif %}
            <p class="font-medium">v{{ comparison.Version2.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">
                Download
            </a>
        </div>
    </div>
</div>
