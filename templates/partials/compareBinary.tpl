<div class="bg-white shadow rounded-lg p-4">
    <div class="bg-stone-50 border border-stone-200 rounded p-3 mb-6 text-stone-600 text-sm font-sans flex items-center gap-2">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0" aria-hidden="true"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
        No visual preview available for this file type. Use the download links to compare locally.
    </div>

    <div class="grid grid-cols-2 gap-6">
        <div class="border rounded overflow-hidden text-center">
            <div class="compare-panel-header--old">OLD — v{{ comparison.Version1.VersionNumber }}</div>
            <div class="p-4">
                {% if comparison.Version1.Width > 0 %}
                <img src="/v1/resource/preview?id={{ resource1.ID }}&maxX=200&maxY=200"
                     class="mx-auto mb-3 max-h-32" alt="Thumbnail">
                {% else %}
                <div class="w-24 h-24 bg-stone-100 mx-auto mb-3 flex items-center justify-center rounded">
                    <span class="text-stone-500 text-xs">{{ comparison.Version1.ContentType }}</span>
                </div>
                {% endif %}
                <p class="text-sm text-stone-500 font-mono mb-3">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
                   class="inline-block px-4 py-2 bg-teal-700 text-white rounded hover:bg-teal-800 text-sm font-medium">
                    Download
                </a>
            </div>
        </div>
        <div class="border rounded overflow-hidden text-center">
            <div class="compare-panel-header--new">NEW — v{{ comparison.Version2.VersionNumber }}</div>
            <div class="p-4">
                {% if comparison.Version2.Width > 0 %}
                <img src="/v1/resource/preview?id={{ resource2.ID }}&maxX=200&maxY=200"
                     class="mx-auto mb-3 max-h-32" alt="Thumbnail">
                {% else %}
                <div class="w-24 h-24 bg-stone-100 mx-auto mb-3 flex items-center justify-center rounded">
                    <span class="text-stone-500 text-xs">{{ comparison.Version2.ContentType }}</span>
                </div>
                {% endif %}
                <p class="text-sm text-stone-500 font-mono mb-3">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
                   class="inline-block px-4 py-2 bg-teal-700 text-white rounded hover:bg-teal-800 text-sm font-medium">
                    Download
                </a>
            </div>
        </div>
    </div>
</div>
