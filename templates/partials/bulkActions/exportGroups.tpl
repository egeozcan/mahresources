    <div class="px-4">
        <button type="button"
                @click="window.location.href = '/admin/export?groups=' + [...$selection.selectedIds].join(',')"
                data-testid="bulk-export-selected"
                class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            Export selected
        </button>
    </div>
