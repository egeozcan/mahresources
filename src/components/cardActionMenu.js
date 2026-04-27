export function cardActionMenu() {
    return {
        open: false,
        toggle() { this.open = !this.open; },
        close() { this.open = false; },
        runAction(action, entityId, entityType) {
            this.close();
            window.dispatchEvent(new CustomEvent('plugin-action-open', {
                detail: {
                    plugin: action.plugin_name,
                    action: action.id,
                    label: action.label,
                    description: action.description,
                    entityIds: [entityId],
                    entityType: entityType,
                    async: action.async,
                    params: action.params,
                    confirm: action.confirm,
                    filters: action.filters,
                    bulk_max: action.bulk_max,
                }
            }));
        }
    };
}
