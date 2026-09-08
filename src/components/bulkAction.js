export function bulkAction(action) {
    return {
        action,
        unavailableReason() {
            const selection = this.$selection;
            const count = selection.selectedIds.size;
            if (action.Min === action.Max && action.Min > 0 && count !== action.Min) return `Select exactly ${action.Min} items to ${action.Label.toLowerCase()}.`;
            if (count < (action.Min || 1)) return 'Select at least one item.';
            if (action.Max > 0 && count > action.Max) return `Select at most ${action.Max} items.`;
            const filters = action.Filters;
            if (!filters) return '';
            const entities = selection.selectedEntities();
            if (entities.length !== count && Object.values(filters).some(v => v?.length)) return 'Selection details are unavailable.';
            if (entities.some(entity => !matchesActionFilters(entity, filters))) return 'Every selected item must meet this action’s requirements.';
            return '';
        },
        compare(entity) {
            if (this.unavailableReason()) return;
            const [a, b] = this.$selection.selectedIds;
            const prefix = entity === 'resource' ? 'r' : 'g';
            window.location.href = `/${entity}/compare?${prefix}1=${a}&${prefix}2=${b}`;
        },
        runPlugin() {
            if (this.unavailableReason()) return;
            const plugin = action.Plugin;
            this.$dispatch('plugin-action-open', {
                ...plugin, plugin: plugin.plugin_name, action: plugin.id,
                entityType: plugin.entity, entityIds: [...this.$selection.selectedIds],
                selection: this.$selection,
            });
        },
    };
}

export function matchesActionFilters(entity, filters) {
    if (filters.category_ids?.length && !filters.category_ids.includes(entity.resourceCategoryId ?? entity.ResourceCategoryId ?? entity.CategoryId)) return false;
    if (filters.note_type_ids?.length && !filters.note_type_ids.includes(entity.NoteTypeId)) return false;
    if (filters.content_types?.length && (!entity.ContentType || !filters.content_types.includes(entity.ContentType))) return false;
    return true;
}
