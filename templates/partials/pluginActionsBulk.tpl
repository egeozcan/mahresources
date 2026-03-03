{% if pluginBulkActions %}
    {% for action in pluginBulkActions %}
    <button class="px-4 inline-flex justify-center py-2 mt-3 border border-transparent items-center shadow-sm text-sm font-medium rounded-md text-white bg-purple-600 hover:bg-purple-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-purple-500"
            @click="$dispatch('plugin-action-open', { ...{{ action|json }}, entityIds: Array.from($store.bulkSelection.selectedIds), entityType: '{{ entityType }}' })">
        {{ action.Label }}
    </button>
    {% endfor %}
{% endif %}
