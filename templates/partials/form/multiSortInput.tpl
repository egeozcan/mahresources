<div
    class="w-full"
    x-data="multiSort({
        availableColumns: {{ values|json }},
        name: '{{ name }}'
    })"
    role="group"
    aria-label="Sort options"
>
    <template x-for="(sort, index) in sortColumns" :key="index">
        <div class="flex gap-1 items-center mt-1.5">
            <template x-if="sort.column && (sort.column !== '__meta__' || (sort.metaKey && isValidMetaKey(sort.metaKey)))">
                <input type="hidden" :name="name" :value="formatSort(sort)">
            </template>

            <select
                x-model="sort.column"
                x-init="$nextTick(() => $el.value = sort.column)"
                @change="if (sort.column !== '__meta__') sort.metaKey = ''"
                class="select-compact flex-1 min-w-0 text-xs py-1 pl-1.5 border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                :aria-label="'Sort column ' + (index + 1)"
            >
                <option value="">-- Column --</option>
                <template x-for="col in getAvailableColumnsForRow(index)" :key="col.Value">
                    <option :value="col.Value" x-text="col.Name"></option>
                </template>
                {# Finding 20: only entities whose table has a meta column can be sorted #}
                {# by one. Offering it elsewhere (categories) produced a full-page 400. #}
                {% if sortMetaSupported %}<option value="__meta__">Custom Property</option>{% endif %}
            </select>

            <template x-if="sort.column === '__meta__'">
                <input
                    type="text"
                    x-model="sort.metaKey"
                    placeholder="meta key"
                    class="flex-1 min-w-[72px] text-xs py-1 px-1.5 border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                    :class="sort.metaKey && !isValidMetaKey(sort.metaKey) ? 'border-red-500' : ''"
                    :aria-label="'Custom property name for sort ' + (index + 1)"
                    title="Property name (lowercase letters and underscores only)"
                >
            </template>

            <button
                type="button"
                @click="sort.direction = sort.direction === 'asc' ? 'desc' : 'asc'"
                class="w-6 h-6 flex items-center justify-center border border-stone-300 rounded text-xs font-mono bg-white hover:bg-stone-50 focus:outline-none focus:ring-1 focus:ring-amber-600 shrink-0"
                :aria-label="'Sort direction: ' + (sort.direction === 'asc' ? 'ascending' : 'descending')"
                :title="sort.direction === 'asc' ? 'Ascending' : 'Descending'"
            >
                <span x-text="sort.direction === 'asc' ? '\u2191' : '\u2193'"></span>
            </button>

            {# Deferred-work item 4: these three were 18x12, 18x12 and 18x24, which is #}
            {# WCAG 2.2 SC 2.5.8 (findings 48/99/139 again). Turning on wcag22aa found #}
            {# the two arrows; the remove button is the same defect one axe check away #}
            {# (18px wide), so it is fixed with them.                                  #}
            {#                                                                         #}
            {# A pseudo-element hit area would not do: axe and the pointer both measure #}
            {# the element's own rect, so the box has to be real — the remedy WS5       #}
            {# finding 49 took for .calendar-day-number.                                #}
            {#                                                                         #}
            {# The arrows moved from a stacked 18px column to side by side so the row  #}
            {# stays one line tall, which is what it has to be: this widget sits among  #}
            {# fifteen other single-line filter fields. The cost is 40px of the select, #}
            {# measured at 328px wide on a 400px sidebar for a longest option of 15     #}
            {# characters, and 260px at 390px viewport — slack either way.              #}
            <button
                type="button"
                @click="moveUp(index)"
                :disabled="index === 0"
                class="w-6 h-6 flex items-center justify-center text-stone-400 hover:text-stone-700 disabled:opacity-30 focus:outline-none focus:text-amber-700 shrink-0"
                :aria-label="'Move sort ' + (index + 1) + ' up'"
                title="Move up"
            >
                <span class="text-[10px] leading-none">&#9650;</span>
            </button>
            <button
                type="button"
                @click="moveDown(index)"
                :disabled="index === sortColumns.length - 1"
                class="w-6 h-6 flex items-center justify-center text-stone-400 hover:text-stone-700 disabled:opacity-30 focus:outline-none focus:text-amber-700 shrink-0"
                :aria-label="'Move sort ' + (index + 1) + ' down'"
                title="Move down"
            >
                <span class="text-[10px] leading-none">&#9660;</span>
            </button>

            <button
                type="button"
                @click="removeSort(index)"
                class="w-6 h-6 flex items-center justify-center text-stone-300 hover:text-red-600 focus:outline-none focus:text-red-600 shrink-0 transition-colors duration-100"
                :aria-label="'Remove sort ' + (index + 1)"
                title="Remove"
            >
                <span class="text-xs">&#10005;</span>
            </button>
        </div>
    </template>

    <button
        type="button"
        @click="addSort()"
        :disabled="sortColumns.length >= availableColumns.length + 5"
        class="mt-1 inline-flex items-center gap-0.5 text-xs font-mono font-medium text-stone-500 hover:text-amber-700 focus:outline-none focus:text-amber-700 disabled:opacity-40 transition-colors duration-100"
        aria-label="Add another sort criteria"
    >
        + Add Sort
    </button>
</div>
