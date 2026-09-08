    <div class="px-4">
        <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Export</span>
        <button type="button"
                @click="window.location.href = '/admin/export?groups=' + [...$selection.selectedIds].join(',')"
                data-testid="bulk-export-selected"
                class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500">
            Export selected
        </button>
    </div>
