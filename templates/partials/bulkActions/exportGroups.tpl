    <div class="bulk-action-item">
        <button type="button"
                @click="window.location.href = '/admin/export?groups=' + [...$selection.selectedIds].join(',')"
                data-testid="bulk-export-selected"
                class="bulk-action-btn">
            Export selected
        </button>
    </div>
