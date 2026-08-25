{# No per-version thumbnail exists — /v1/resource/preview is keyed on the #}
{# resource, so in a same-resource comparison it would render the current #}
{# thumbnail on both sides whatever versions are selected. The type placeholder #}
{# is at least true of both. #}
<div class="bg-white shadow rounded-lg p-4">
    <div class="bg-stone-50 border border-stone-200 rounded p-3 mb-6 text-stone-700 text-sm font-sans flex items-center gap-2">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0" aria-hidden="true"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
        {% if comparison.SameType %}
        No visual preview available for this file type. Use the download links to compare locally.
        {% else %}
        These two versions are different file types, so there is nothing to show side by side.
        Use the download links to compare locally.
        {% endif %}
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
        <div class="border rounded overflow-hidden text-center">
            <div class="compare-panel-header--old">{{ panelTitle1 }}</div>
            <div class="p-4">
                <div class="compare-binary-badge">
                    <span>{{ comparison.Version1.ContentType }}</span>
                </div>
                <p class="text-sm text-stone-600 font-mono mb-3">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
                   class="inline-block px-4 py-2 bg-teal-800 text-white rounded hover:bg-teal-900 text-sm font-medium">
                    Download {{ panelTitle1 }}
                </a>
            </div>
        </div>
        <div class="border rounded overflow-hidden text-center">
            <div class="compare-panel-header--new">{{ panelTitle2 }}</div>
            <div class="p-4">
                <div class="compare-binary-badge">
                    <span>{{ comparison.Version2.ContentType }}</span>
                </div>
                <p class="text-sm text-stone-600 font-mono mb-3">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
                   class="inline-block px-4 py-2 bg-teal-800 text-white rounded hover:bg-teal-900 text-sm font-medium">
                    Download {{ panelTitle2 }}
                </a>
            </div>
        </div>
    </div>
</div>
