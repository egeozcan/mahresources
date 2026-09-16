<button type="button"
        x-data
        @click="$dispatch('mass-edit-open', { entityType: '{{ massEditEntity }}', target: 'filter', selection: $store.bulkSelection })"
        class="inline-flex items-center px-4 py-2 border border-stone-300 rounded-md shadow-sm text-sm font-mono font-medium text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
    Mass edit all {{ totalCount|default:0 }} results
</button>
