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
        <div class="flex gap-2 items-center mt-2">
            <template x-if="sort.column">
                <input type="hidden" :name="name" :value="formatSort(sort)">
            </template>

            <select
                x-model="sort.column"
                class="flex-1 shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md"
                :aria-label="'Sort column ' + (index + 1)"
            >
                <option value="">-- Select column --</option>
                <template x-for="col in getAvailableColumnsForRow(index)" :key="col.Value">
                    <option :value="col.Value" x-text="col.Name"></option>
                </template>
            </select>

            <button
                type="button"
                @click="sort.direction = sort.direction === 'asc' ? 'desc' : 'asc'"
                class="px-3 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                :aria-label="'Sort direction: ' + (sort.direction === 'asc' ? 'ascending' : 'descending')"
                :title="sort.direction === 'asc' ? 'Ascending' : 'Descending'"
            >
                <span x-text="sort.direction === 'asc' ? '\u2191' : '\u2193'"></span>
            </button>

            <button
                type="button"
                @click="moveUp(index)"
                :disabled="index === 0"
                class="px-2 py-2 border border-gray-300 rounded-md shadow-sm text-xs bg-white hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                :aria-label="'Move sort ' + (index + 1) + ' up'"
                title="Move up"
            >
                <span class="text-xs">\u25B2</span>
            </button>

            <button
                type="button"
                @click="moveDown(index)"
                :disabled="index === sortColumns.length - 1"
                class="px-2 py-2 border border-gray-300 rounded-md shadow-sm text-xs bg-white hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                :aria-label="'Move sort ' + (index + 1) + ' down'"
                title="Move down"
            >
                <span class="text-xs">\u25BC</span>
            </button>

            <button
                type="button"
                @click="removeSort(index)"
                class="px-2 py-2 border border-red-300 rounded-md shadow-sm text-xs text-red-700 bg-white hover:bg-red-50"
                :aria-label="'Remove sort ' + (index + 1)"
                title="Remove"
            >
                <span class="text-xs">\u2715</span>
            </button>
        </div>
    </template>

    <button
        type="button"
        @click="addSort()"
        :disabled="sortColumns.length >= availableColumns.length"
        class="mt-2 inline-flex items-center px-2 py-1 border border-gray-300 rounded-md shadow-sm text-xs font-medium text-white bg-green-700 hover:bg-green-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
        aria-label="Add another sort criteria"
    >
        Add Sort
    </button>
</div>
