{% extends "/layouts/base.tpl" %}

{% block body %}
{% if errorMessage %}
<div class="max-w-7xl mx-auto">
    <p class="text-stone-600">Please provide a resource ID to compare versions.</p>
</div>
{% else %}
<div class="max-w-7xl mx-auto" x-data="compareView({
    r1: {{ query.Resource1ID }},
    v1: {{ query.Version1|default:0 }},
    r2: {{ query.Resource2ID }},
    v2: {{ query.Version2|default:0 }}
})" @resource1-selected.window="onResource1Change($event.detail.item.ID)" @resource2-selected.window="onResource2Change($event.detail.item.ID)">
    <!-- Picker Toolbar -->
    <div class="bg-white shadow rounded-lg p-3 mb-4">
        <div class="flex flex-col md:flex-row items-stretch md:items-center gap-3">
            <!-- Left (OLD) side -->
            <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
                <span class="compare-side-label--old" aria-label="{{ label1 }}">{{ label1 }}</span>
                <div x-data="autocompleter({
                    url: '/v1/resources',
                    selectedResults: [{{ resource1|json }}],
                    elName: 'r1',
                    max: 1,
                    standalone: true,
                    dispatchOnSelect: 'resource1-selected'
                })" class="relative flex-1 min-w-[140px]">
                    <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                           class="w-full border rounded px-3 py-1.5 text-sm"
                           placeholder="Search resources..."
                           aria-label="Search left resource">
                    <div x-show="dropdownActive" x-ref="list" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto w-full">
                        <template x-for="(item, index) in results" :key="item.ID">
                            <div @mousedown.prevent="selectedIndex = index; pushVal($event)"
                                 class="px-3 py-2 hover:bg-stone-100 cursor-pointer"
                                 :class="{ 'bg-amber-100': selectedIndex === index }"
                                 x-text="item.Name"></div>
                        </template>
                    </div>
                </div>
                <select x-model="v1" @change="updateUrl()" class="border rounded px-2 py-1.5 text-sm" aria-label="Left version">
                    {% for v in versions1 %}
                    <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version1 %}selected{% endif %}>
                        v{{ v.VersionNumber }} - {{ v.CreatedAt|date:"Jan 02, 2006" }}
                    </option>
                    {% endfor %}
                </select>
            </div>

            <!-- Swap button -->
            <button class="compare-swap-btn self-center" @click="swapSides()" aria-label="Swap sides">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 16V4m0 0L3 8m4-4l4 4M17 8v12m0 0l4-4m-4 4l-4-4"/></svg>
            </button>

            <!-- Right (NEW) side -->
            <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
                <span class="compare-side-label--new" aria-label="{{ label2 }}">{{ label2 }}</span>
                <div x-data="autocompleter({
                    url: '/v1/resources',
                    selectedResults: [{{ resource2|json }}],
                    elName: 'r2',
                    max: 1,
                    standalone: true,
                    dispatchOnSelect: 'resource2-selected'
                })" class="relative flex-1 min-w-[140px]">
                    <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                           class="w-full border rounded px-3 py-1.5 text-sm"
                           placeholder="Search resources..."
                           aria-label="Search right resource">
                    <div x-show="dropdownActive" x-ref="list" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto w-full">
                        <template x-for="(item, index) in results" :key="item.ID">
                            <div @mousedown.prevent="selectedIndex = index; pushVal($event)"
                                 class="px-3 py-2 hover:bg-stone-100 cursor-pointer"
                                 :class="{ 'bg-amber-100': selectedIndex === index }"
                                 x-text="item.Name"></div>
                        </template>
                    </div>
                </div>
                <select x-model="v2" @change="updateUrl()" class="border rounded px-2 py-1.5 text-sm" aria-label="Right version">
                    {% for v in versions2 %}
                    <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version2 %}selected{% endif %}>
                        v{{ v.VersionNumber }} - {{ v.CreatedAt|date:"Jan 02, 2006" }}
                    </option>
                    {% endfor %}
                </select>
            </div>
        </div>
    </div>

    {% if comparison %}
    <!-- Summary Banner -->
    <div class="compare-summary mb-4">
        {% if comparison.SameHash %}
        <span class="compare-verdict--identical">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6L9 17l-5-5"/></svg>
            Files are identical
        </span>
        {% else %}
        <span class="compare-verdict--different">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6L6 18M6 6l12 12"/></svg>
            Files differ
        </span>
        {% endif %}
        <span class="compare-stat">
            <span class="compare-stat-label">Type</span>
            {% if comparison.SameType %}Match{% else %}Changed{% endif %}
        </span>
        <span class="compare-stat">
            <span class="compare-stat-label">Size</span>
            {% if comparison.SizeDelta == 0 %}Match
            {% elif comparison.SizeDelta > 0 %}+{{ comparison.SizeDelta|humanReadableSize }}
            {% else %}{{ comparison.SizeDelta|humanReadableSize }}{% endif %}
        </span>
        {% if comparison.DimensionsDiff %}
        <span class="compare-stat">
            <span class="compare-stat-label">Dimensions</span>
            {{ comparison.Version1.Width }}&times;{{ comparison.Version1.Height }} &rarr; {{ comparison.Version2.Width }}&times;{{ comparison.Version2.Height }}
        </span>
        {% endif %}
        {% if crossResource %}
        <span class="compare-stat" style="background: #fef3c7; color: #92400e;">
            <span class="compare-stat-label">Cross-resource</span>
        </span>
        {% endif %}
    </div>

    <!-- Metadata -->
    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Metadata</summary>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            <!-- Content Type -->
            <div class="compare-meta-card{% if not comparison.SameType %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Content Type</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameType %}
                        {{ comparison.Version1.ContentType }}
                    {% else %}
                        {{ comparison.Version1.ContentType }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.ContentType }}
                    {% endif %}
                </div>
            </div>
            <!-- File Size -->
            <div class="compare-meta-card{% if comparison.SizeDelta != 0 %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">File Size</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SizeDelta == 0 %}
                        {{ comparison.Version1.FileSize|humanReadableSize }}
                    {% else %}
                        {{ comparison.Version1.FileSize|humanReadableSize }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.FileSize|humanReadableSize }}
                        <span class="text-xs {% if comparison.SizeDelta > 0 %}text-amber-700{% else %}text-orange-600{% endif %}">
                            ({% if comparison.SizeDelta > 0 %}+{% endif %}{{ comparison.SizeDelta|humanReadableSize }})
                        </span>
                    {% endif %}
                </div>
            </div>
            <!-- Dimensions -->
            <div class="compare-meta-card{% if comparison.DimensionsDiff %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Dimensions</div>
                <div class="compare-meta-card-value">
                    {% if comparison.DimensionsDiff %}
                        {{ comparison.Version1.Width }}&times;{{ comparison.Version1.Height }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.Width }}&times;{{ comparison.Version2.Height }}
                    {% else %}
                        {{ comparison.Version1.Width }}&times;{{ comparison.Version1.Height }}
                    {% endif %}
                </div>
            </div>
            <!-- Hash -->
            <div class="compare-meta-card{% if not comparison.SameHash %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Hash</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameHash %}
                        <span class="text-amber-700 font-medium">Match</span>
                        <span class="text-xs font-mono text-stone-500 ml-1">{{ comparison.Version1.Hash|truncatechars:16 }}...</span>
                    {% else %}
                        <span class="text-red-700 font-medium">Different</span>
                    {% endif %}
                </div>
            </div>
            <!-- Created -->
            <div class="compare-meta-card">
                <div class="compare-meta-card-label">Created</div>
                <div class="compare-meta-card-value">
                    {{ comparison.Version1.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    {% if comparison.Version1.CreatedAt != comparison.Version2.CreatedAt %}
                        <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    {% endif %}
                </div>
            </div>
            <!-- Resource (cross-resource only) -->
            {% if crossResource %}
            <div class="compare-meta-card compare-meta-card--diff">
                <div class="compare-meta-card-label">Resource</div>
                <div class="compare-meta-card-value">
                    <a href="/resource?id={{ resource1.ID }}" class="text-teal-700 hover:underline">{{ resource1.Name }}</a>
                    <span class="text-stone-400" aria-hidden="true">&rarr;</span>
                    <a href="/resource?id={{ resource2.ID }}" class="text-teal-700 hover:underline">{{ resource2.Name }}</a>
                </div>
            </div>
            {% endif %}
            <!-- Comment (only if either has one) -->
            {% if comparison.Version1.Comment or comparison.Version2.Comment %}
            <div class="compare-meta-card sm:col-span-2 lg:col-span-3{% if comparison.Version1.Comment != comparison.Version2.Comment %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Comment</div>
                <div class="compare-meta-card-value italic text-stone-600">
                    {% if comparison.Version1.Comment == comparison.Version2.Comment %}
                        "{{ comparison.Version1.Comment }}"
                    {% else %}
                        "{{ comparison.Version1.Comment }}" <span class="text-stone-400 not-italic" aria-hidden="true">&rarr;</span> "{{ comparison.Version2.Comment }}"
                    {% endif %}
                </div>
            </div>
            {% endif %}
        </div>
    </details>

    <!-- Content Comparison Area -->
    {% if contentCategory == "image" %}
        {% include "/partials/compareImage.tpl" %}
    {% elif contentCategory == "text" %}
        {% include "/partials/compareText.tpl" %}
    {% elif contentCategory == "pdf" %}
        {% include "/partials/comparePdf.tpl" %}
    {% else %}
        {% include "/partials/compareBinary.tpl" %}
    {% endif %}

    {% if canMerge %}
    <details class="mt-6 bg-white shadow rounded-lg" x-data="{ keepAsVersion: false }">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 p-4 select-none font-mono">Merge</summary>
        <div class="p-4 pt-0">
            <div class="mb-4">
                <label class="flex items-center gap-2 text-sm text-stone-600 cursor-pointer">
                    <input type="checkbox" x-model="keepAsVersion" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Keep loser as older version of winner
                </label>
            </div>
            <div class="flex justify-between items-center gap-4">
                <form
                    x-data="confirmAction({ message: 'Resource on the right will be merged into the left resource. Are you sure?' })"
                    action="/v1/resources/merge?redirect=%2Fresource%3Fid%3D{{ resource1.ID }}"
                    method="post"
                    x-bind="events"
                >
                    <input type="hidden" name="winner" value="{{ resource1.ID }}">
                    <input type="hidden" name="losers" value="{{ resource2.ID }}">
                    <input type="hidden" name="KeepAsVersion" :value="keepAsVersion">
                    {% include "/partials/form/searchButton.tpl" with text="← Left Wins" %}
                </form>
                <form
                    x-data="confirmAction({ message: 'Resource on the left will be merged into the right resource. Are you sure?' })"
                    action="/v1/resources/merge?redirect=%2Fresource%3Fid%3D{{ resource2.ID }}"
                    method="post"
                    x-bind="events"
                >
                    <input type="hidden" name="winner" value="{{ resource2.ID }}">
                    <input type="hidden" name="losers" value="{{ resource1.ID }}">
                    <input type="hidden" name="KeepAsVersion" :value="keepAsVersion">
                    {% include "/partials/form/searchButton.tpl" with text="Right Wins →" %}
                </form>
            </div>
        </div>
    </details>
    {% endif %}

    {% else %}
    <!-- Empty State -->
    <div class="compare-empty-state">
        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <rect x="2" y="3" width="8" height="14" rx="1"/>
            <rect x="14" y="7" width="8" height="14" rx="1"/>
            <path d="M12 10l2-2m0 0l-2-2m2 2H8"/>
        </svg>
        <p class="text-lg font-medium text-stone-700">Ready to Compare</p>
        <p class="text-sm max-w-xs">Select resources and versions above to see a detailed comparison.</p>
        <div class="flex items-center gap-2 text-xs mt-1">
            <span class="compare-side-label--old">{{ label1 }}</span>
            <span class="text-stone-400" aria-hidden="true">&harr;</span>
            <span class="compare-side-label--new">{{ label2 }}</span>
        </div>
    </div>
    {% endif %}
</div>
{% endif %}
{% endblock %}
