<section
    class="timeline-container"
    x-data="timeline({ apiUrl: '{{ entityApiUrl }}/timeline', entityType: '{{ entityType }}', defaultView: '{{ entityDefaultView }}' })"
    aria-label="{{ entityType }} timeline"
    tabindex="0"
    @keydown.left="prev()"
    @keydown.right="next()"
>
    {# Navigation bar #}
    <div class="timeline-nav flex items-center gap-2 mb-4 flex-wrap">
        <div role="group" aria-label="Activity type" class="timeline-granularity">
            <button
                type="button"
                class="timeline-gran-btn"
                :class="timelineMode === 'created' ? 'active' : ''"
                @click="setTimelineMode('created')"
                :aria-pressed="timelineMode === 'created' ? 'true' : 'false'"
            >Created</button>
            <button
                type="button"
                class="timeline-gran-btn"
                :class="timelineMode === 'updated' ? 'active' : ''"
                @click="setTimelineMode('updated')"
                :aria-pressed="timelineMode === 'updated' ? 'true' : 'false'"
            >Updated</button>
        </div>

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
            :disabled="!hasMore.right"
            aria-label="Next time range"
        >
            Next <span aria-hidden="true">&rarr;</span>
        </button>

        <div role="group" aria-label="Time granularity" class="timeline-granularity ml-auto">
            <button
                type="button"
                class="timeline-gran-btn"
                :class="granularity === 'year' ? 'active' : ''"
                @click="setGranularity('year')"
                :aria-pressed="granularity === 'year' ? 'true' : 'false'"
            >Year</button>
            <button
                type="button"
                class="timeline-gran-btn"
                :class="granularity === 'month' ? 'active' : ''"
                @click="setGranularity('month')"
                :aria-pressed="granularity === 'month' ? 'true' : 'false'"
            >Month</button>
            <button
                type="button"
                class="timeline-gran-btn"
                :class="granularity === 'week' ? 'active' : ''"
                @click="setGranularity('week')"
                :aria-pressed="granularity === 'week' ? 'true' : 'false'"
            >Week</button>
        </div>
    </div>

    {# Loading skeleton #}
    <div x-show="loading" x-cloak aria-busy="true" aria-label="Loading timeline data">
        <div class="timeline-skeleton-bars">
            <template x-for="i in columns" :key="i">
                <div class="timeline-skeleton-col">
                    <div class="skeleton-bar" :style="'height:' + (15 + Math.random() * 70) + '%'"></div>
                    <div class="skeleton-bar lighter" :style="'height:' + (10 + Math.random() * 50) + '%'"></div>
                </div>
            </template>
        </div>
    </div>

    {# Error display with retry #}
    <div x-show="error" x-cloak class="bg-red-100 border-l-4 border-red-500 text-red-700 p-4 mb-4" role="alert">
        <span x-text="error"></span>
        <button type="button" @click="fetchBuckets()" class="ml-2 underline font-medium">Retry</button>
    </div>

    {# Chart area #}
    <div
        class="timeline-chart"
        x-ref="chart"
        x-show="!loading && !error"
        role="group"
        :aria-label="'Bar chart showing ' + entityType + ' activity over time'"
    ></div>

    {# Preview panel (hidden until bar clicked) #}
    <div
        class="timeline-preview mt-4"
        x-show="previewHtml"
        x-cloak
    >
        <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold" x-text="previewTitle"></h3>
            <a :href="showAllUrl" class="text-sm text-amber-700 hover:text-amber-900 underline">
                Show all (<span x-text="previewTotalCount"></span>)
            </a>
        </div>
        <div class="timeline-preview-grid" x-html="previewHtml"></div>
    </div>
</section>
