{% extends "/layouts/base.tpl" %}

{% block body %}
{% if errorMessage %}
<div class="max-w-7xl mx-auto">
    <p class="text-stone-600">{{ errorMessage }}</p>
</div>
{% else %}
<div class="max-w-7xl mx-auto" x-data="groupCompareView({
    g1: {{ query.Group1ID }},
    g2: {{ query.Group2ID }}
})" @group1-selected.window="onGroup1Change($event.detail.item.ID)" @group2-selected.window="onGroup2Change($event.detail.item.ID)">
    <div class="bg-white shadow rounded-lg p-3 mb-4">
        <div class="flex flex-col md:flex-row items-stretch md:items-center gap-3">
            <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
                <span class="compare-side-label--old" aria-label="{{ label1 }}">{{ label1 }}</span>
                <div x-data="autocompleter({
                    url: '/v1/groups',
                    selectedResults: [{{ group1Picker|json }}],
                    elName: 'g1',
                    max: 1,
                    standalone: true,
                    extraInfo: 'Category',
                    dispatchOnSelect: 'group1-selected'
                })" class="relative flex-1 min-w-[180px]">
                    <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                           role="combobox"
                           class="w-full border rounded px-3 py-1.5 text-sm"
                           placeholder="Search groups..."
                           aria-label="Search left group">
                    <div x-show="dropdownActive" x-ref="list" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto w-full">
                        <template x-for="(item, index) in results" :key="item.ID">
                            <div @mousedown.prevent="selectedIndex = index; pushVal($event)"
                                 role="option"
                                 class="px-3 py-2 hover:bg-stone-100 cursor-pointer"
                                 :class="{ 'bg-amber-100': selectedIndex === index }">
                                <div x-text="item.Name"></div>
                                <div class="text-xs text-stone-500" x-show="item.Category && item.Category.Name" x-text="item.Category.Name"></div>
                            </div>
                        </template>
                    </div>
                </div>
            </div>

            <button class="compare-swap-btn self-center" @click="swapSides()" aria-label="Swap sides">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 16V4m0 0L3 8m4-4l4 4M17 8v12m0 0l4-4m-4 4l-4-4"/></svg>
            </button>

            <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
                <span class="compare-side-label--new" aria-label="{{ label2 }}">{{ label2 }}</span>
                <div x-data="autocompleter({
                    url: '/v1/groups',
                    selectedResults: [{{ group2Picker|json }}],
                    elName: 'g2',
                    max: 1,
                    standalone: true,
                    extraInfo: 'Category',
                    dispatchOnSelect: 'group2-selected'
                })" class="relative flex-1 min-w-[180px]">
                    <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                           role="combobox"
                           class="w-full border rounded px-3 py-1.5 text-sm"
                           placeholder="Search groups..."
                           aria-label="Search right group">
                    <div x-show="dropdownActive" x-ref="list" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto w-full">
                        <template x-for="(item, index) in results" :key="item.ID">
                            <div @mousedown.prevent="selectedIndex = index; pushVal($event)"
                                 role="option"
                                 class="px-3 py-2 hover:bg-stone-100 cursor-pointer"
                                 :class="{ 'bg-amber-100': selectedIndex === index }">
                                <div x-text="item.Name"></div>
                                <div class="text-xs text-stone-500" x-show="item.Category && item.Category.Name" x-text="item.Category.Name"></div>
                            </div>
                        </template>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <div class="compare-summary mb-4">
        {% if comparison.HasDifferences %}
        <span class="compare-verdict--different">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6L6 18M6 6l12 12"/></svg>
            Groups differ
        </span>
        {% else %}
        <span class="compare-verdict--identical">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6L9 17l-5-5"/></svg>
            Groups are identical
        </span>
        {% endif %}
        <span class="compare-stat">
            <span class="compare-stat-label">Sections</span>
            {{ comparison.DifferentSectionCount }} changed
        </span>
        <span class="compare-stat">
            <span class="compare-stat-label">Core</span>
            {% if comparison.CoreSame %}Match{% else %}{{ comparison.CoreDifferentCount }} changed{% endif %}
        </span>
        <span class="compare-stat">
            <span class="compare-stat-label">Tags</span>
            {{ comparison.Tags.TotalCount }}
        </span>
        <span class="compare-stat">
            <span class="compare-stat-label">Entities</span>
            {{ comparison.OwnEntitiesCount }} own / {{ comparison.RelatedEntitiesCount }} related
        </span>
        <span class="compare-stat">
            <span class="compare-stat-label">Relations</span>
            {{ comparison.RelationsCount }}
        </span>
        {% if comparison.CrossGroup %}
        <span class="compare-stat" style="background: #fef3c7; color: #92400e;">
            <span class="compare-stat-label">Cross-group</span>
        </span>
        {% endif %}
    </div>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Metadata</summary>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            <div class="compare-meta-card{% if not comparison.SameName %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Name</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameName %}
                    {{ comparison.Group1.Name }}
                    {% else %}
                    {{ comparison.Group1.Name }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Group2.Name }}
                    {% endif %}
                </div>
            </div>
            <div class="compare-meta-card{% if not comparison.SameCategory %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Category</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameCategory %}
                    {{ comparison.Category1 }}
                    {% else %}
                    {{ comparison.Category1 }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Category2 }}
                    {% endif %}
                </div>
            </div>
            <div class="compare-meta-card{% if not comparison.SameOwner %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Owner</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameOwner %}
                    {{ comparison.Owner1 }}
                    {% else %}
                    {{ comparison.Owner1 }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Owner2 }}
                    {% endif %}
                </div>
            </div>
            <div class="compare-meta-card{% if not comparison.SameURL %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">URL</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameURL %}
                    {% if comparison.URL1 %}<a href="{{ comparison.URL1 }}" target="_blank" referrerpolicy="no-referrer" class="text-teal-700 hover:underline">{{ comparison.URL1 }}</a>{% else %}None{% endif %}
                    {% else %}
                    {% if comparison.URL1 %}<a href="{{ comparison.URL1 }}" target="_blank" referrerpolicy="no-referrer" class="text-teal-700 hover:underline">{{ comparison.URL1 }}</a>{% else %}None{% endif %}
                    <span class="text-stone-400" aria-hidden="true">&rarr;</span>
                    {% if comparison.URL2 %}<a href="{{ comparison.URL2 }}" target="_blank" referrerpolicy="no-referrer" class="text-teal-700 hover:underline">{{ comparison.URL2 }}</a>{% else %}None{% endif %}
                    {% endif %}
                </div>
            </div>
            <div class="compare-meta-card{% if not comparison.SameCreatedAt %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Created</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameCreatedAt %}
                    {{ comparison.Group1.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    {% else %}
                    {{ comparison.Group1.CreatedAt|date:"Jan 02, 2006 15:04" }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Group2.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    {% endif %}
                </div>
            </div>
            <div class="compare-meta-card{% if not comparison.SameUpdatedAt %} compare-meta-card--diff{% endif %}">
                <div class="compare-meta-card-label">Updated</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameUpdatedAt %}
                    {{ comparison.Group1.UpdatedAt|date:"Jan 02, 2006 15:04" }}
                    {% else %}
                    {{ comparison.Group1.UpdatedAt|date:"Jan 02, 2006 15:04" }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Group2.UpdatedAt|date:"Jan 02, 2006 15:04" }}
                    {% endif %}
                </div>
            </div>
        </div>
    </details>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Tags ({{ comparison.Tags.TotalCount }})</summary>
        {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.Tags label1=label1 label2=label2 %}
    </details>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Own Entities ({{ comparison.OwnEntitiesCount }})</summary>
        <div class="space-y-6">
            <div>
                <h3 class="compare-section-subtitle">Groups ({{ comparison.OwnGroups.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.OwnGroups label1=label1 label2=label2 %}
            </div>
            <div>
                <h3 class="compare-section-subtitle">Notes ({{ comparison.OwnNotes.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.OwnNotes label1=label1 label2=label2 %}
            </div>
            <div>
                <h3 class="compare-section-subtitle">Resources ({{ comparison.OwnResources.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.OwnResources label1=label1 label2=label2 %}
            </div>
        </div>
    </details>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Related Entities ({{ comparison.RelatedEntitiesCount }})</summary>
        <div class="space-y-6">
            <div>
                <h3 class="compare-section-subtitle">Groups ({{ comparison.RelatedGroups.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.RelatedGroups label1=label1 label2=label2 %}
            </div>
            <div>
                <h3 class="compare-section-subtitle">Notes ({{ comparison.RelatedNotes.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.RelatedNotes label1=label1 label2=label2 %}
            </div>
            <div>
                <h3 class="compare-section-subtitle">Resources ({{ comparison.RelatedResources.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.RelatedResources label1=label1 label2=label2 %}
            </div>
        </div>
    </details>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Relations ({{ comparison.RelationsCount }})</summary>
        <div class="space-y-6">
            <div>
                <h3 class="compare-section-subtitle">Forward Relations ({{ comparison.ForwardRelations.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.ForwardRelations label1=label1 label2=label2 %}
            </div>
            <div>
                <h3 class="compare-section-subtitle">Reverse Relations ({{ comparison.ReverseRelations.TotalCount }})</h3>
                {% include "/partials/compareDiffBuckets.tpl" with diff=comparison.ReverseRelations label1=label1 label2=label2 %}
            </div>
        </div>
    </details>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Description</summary>
        {% include "/partials/compareInlineText.tpl" with leftText=comparison.DescriptionLeftText rightText=comparison.DescriptionRightText leftTitle=label1 rightTitle=label2 %}
    </details>

    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Meta JSON</summary>
        {% include "/partials/compareInlineText.tpl" with leftText=comparison.MetaLeftText rightText=comparison.MetaRightText leftTitle=label1 rightTitle=label2 %}
    </details>
</div>
{% endif %}
{% endblock %}
