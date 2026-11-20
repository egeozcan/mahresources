<button type="button" class="bulk-action-btn"
        @click="$dispatch('mass-edit-open', { entityType: '{{ bulkEntity }}', target: 'ids', selection: $selection })">Mass Edit Selected</button>
