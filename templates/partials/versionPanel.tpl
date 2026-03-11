{% if versions %}
<details class="detail-collapsible mb-6" x-data="{ compareMode: false, selected: [] }" {% if versions|length > 1 %}open{% endif %}>
    <summary>Versions ({{ versions|length }})</summary>
    <div class="detail-panel-body">
        {% for version in versions %}
        <div class="p-4 {% if version.ID == currentVersionId %}bg-amber-50{% endif %}">
            <div class="flex items-center justify-between">
                <div class="flex items-center space-x-3">
                    <template x-if="compareMode">
                        <input type="checkbox"
                            :value="{{ version.VersionNumber }}"
                            @change="selected.includes({{ version.VersionNumber }}) ? selected = selected.filter(x => x !== {{ version.VersionNumber }}) : selected.push({{ version.VersionNumber }})"
                            :disabled="selected.length >= 2 && !selected.includes({{ version.VersionNumber }})"
                            :aria-label="'Select version {{ version.VersionNumber }} for comparison'"
                            class="rounded">
                    </template>
                    <span class="font-medium font-mono">
                        v{{ version.VersionNumber }}
                        {% if version.ID == currentVersionId %}
                        <span class="ml-1 px-2 py-0.5 text-xs bg-amber-100 text-amber-800 rounded">current</span>
                        {% endif %}
                    </span>
                    <span class="text-stone-500 text-sm font-mono">{{ version.CreatedAt|date:"Jan 02, 2006" }}</span>
                    <span class="text-stone-500 text-sm font-mono">{{ version.FileSize|humanReadableSize }}</span>
                </div>
                <div class="flex items-center space-x-2">
                    <a href="/v1/resource/version/file?versionId={{ version.ID }}"
                       class="px-3 py-1 text-sm text-amber-700 hover:text-amber-900">
                        Download
                    </a>
                    {% if version.ID != currentVersionId %}
                    <form action="/v1/resource/version/restore" method="post" class="inline">
                        <input type="hidden" name="resourceId" value="{{ resourceId }}">
                        <input type="hidden" name="versionId" value="{{ version.ID }}">
                        <button type="submit" class="px-3 py-1 text-sm font-mono text-amber-700 hover:text-amber-800">
                            Restore
                        </button>
                    </form>
                    <form action="/v1/resource/version/delete?resourceId={{ resourceId }}&versionId={{ version.ID }}" method="post" class="inline"
                          x-data="confirmAction({ message: 'Delete this version?' })" x-bind="events">
                        <button type="submit" class="px-3 py-1 text-sm font-mono text-red-700 hover:text-red-800">
                            Delete
                        </button>
                    </form>
                    {% endif %}
                </div>
            </div>
            {% if version.Comment %}
            <p class="mt-1 text-sm text-stone-600 italic">"{{ version.Comment }}"</p>
            {% endif %}
        </div>
        {% endfor %}

        <div class="p-4 bg-stone-50">
            <div class="flex items-center justify-between">
                <button @click="compareMode = !compareMode; selected = []"
                        class="px-3 py-1 text-sm border rounded hover:bg-stone-100"
                        :class="{ 'bg-amber-100 border-amber-300': compareMode }">
                    <span x-text="compareMode ? 'Cancel Compare' : 'Compare'"></span>
                </button>

                <template x-if="compareMode && selected.length === 2">
                    <a :href="'/resource/compare?r1={{ resourceId }}&v1=' + selected[0] + '&v2=' + selected[1]"
                       class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800">
                        Compare Selected
                    </a>
                </template>

                <form action="/v1/resource/versions?resourceId={{ resourceId }}" method="post" enctype="multipart/form-data"
                      class="flex items-center space-x-2">
                    <input type="file" name="file" required class="text-sm" aria-label="Upload file for new version">
                    <input type="text" name="comment" placeholder="Comment (optional)"
                           class="px-2 py-1 text-sm border rounded" aria-label="Version comment">
                    <button type="submit" class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800">
                        Upload New Version
                    </button>
                </form>
            </div>
        </div>
    </div>
</details>
{% endif %}
