<button type="button" class="bulk-action-btn mt-3 px-3 py-1.5 border rounded-md"
        @click="$dispatch('mass-edit-open', { entityType: '{{ bulkEntity }}', target: 'ids', selection: $selection })">Mass Edit Selected</button>
