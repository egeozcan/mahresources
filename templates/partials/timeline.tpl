<section
    class="timeline-container"
    x-data="timeline({ apiUrl: '{{ entityApiUrl }}/timeline', entityType: '{{ entityType }}', defaultView: '{{ entityDefaultView }}' })"
    aria-label="{{ entityType }} timeline"
>
    {# Navigation bar #}
    <div class="timeline-nav flex items-center gap-2 mb-4 flex-wrap">
        <button
            type="button"
            class="btn btn-sm"
            @click="prev()"
            aria-label="Previous time range"
        >
            <span aria-hidden="true">&larr;</span> Prev
        </button>

        <span class="timeline-range-label font-medium text-sm" x-text="rangeLabel" aria-live="polite"></span>

        <button
            type="button"
            class="btn btn-sm"
            @click="next()"
            aria-label="Next time range"
        >
            Next <span aria-hidden="true">&rarr;</span>
        </button>

        <div role="group" aria-label="Time granularity" class="flex gap-1 ml-auto">
            <button
                type="button"
                class="btn btn-sm"
                :class="granularity === 'year' ? 'btn-active' : ''"
                @click="setGranularity('year')"
                :aria-pressed="granularity === 'year' ? 'true' : 'false'"
            >Y</button>
            <button
                type="button"
                class="btn btn-sm"
                :class="granularity === 'month' ? 'btn-active' : ''"
                @click="setGranularity('month')"
                :aria-pressed="granularity === 'month' ? 'true' : 'false'"
            >M</button>
            <button
                type="button"
                class="btn btn-sm"
                :class="granularity === 'week' ? 'btn-active' : ''"
                @click="setGranularity('week')"
                :aria-pressed="granularity === 'week' ? 'true' : 'false'"
            >W</button>
        </div>
    </div>

    {# Loading indicator #}
    <div x-show="loading" class="text-center py-8 text-stone-500" aria-live="polite">
        Loading timeline data&hellip;
    </div>

    {# Error display #}
    <div x-show="error" class="bg-red-100 border-l-4 border-red-500 text-red-700 p-4 mb-4" role="alert" x-text="error"></div>

    {# Chart area #}
    <div
        class="timeline-chart"
        x-ref="chart"
        x-show="!loading && !error"
        tabindex="0"
        role="img"
        :aria-label="'Bar chart showing ' + entityType + ' activity over time'"
    ></div>

    {# Legend #}
    <div class="timeline-legend flex gap-4 mt-2 text-xs text-stone-600" x-show="!loading && !error">
        <span class="flex items-center gap-1">
            <span class="inline-block w-3 h-3 rounded" style="background-color: var(--timeline-created-color, #b45309);"></span>
            Created
        </span>
        <span class="flex items-center gap-1">
            <span class="inline-block w-3 h-3 rounded" style="background-color: var(--timeline-updated-color, #d97706); opacity: 0.6;"></span>
            Updated
        </span>
    </div>

    {# Preview panel (hidden until bar clicked) #}
    <div
        class="timeline-preview mt-4"
        x-show="previewItems && previewItems.length > 0"
        x-cloak
    >
        <h3 class="text-sm font-semibold mb-2" x-text="previewTitle"></h3>
        <ul class="list-disc pl-5 space-y-1">
            <template x-for="item in previewItems" :key="item.ID || item.id">
                <li>
                    <a :href="defaultView + '/../' + entityType.slice(0, -1) + '?id=' + (item.ID || item.id)"
                       class="text-amber-700 hover:text-amber-900 underline"
                       x-text="item.Name || item.name || ('ID: ' + (item.ID || item.id))"></a>
                </li>
            </template>
        </ul>
    </div>
</section>
