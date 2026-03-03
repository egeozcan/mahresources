export function cardActionMenu() {
    return {
        open: false,
        toggle() { this.open = !this.open; },
        close() { this.open = false; },
        runAction(action, entityId, entityType) {
            this.close();
            window.dispatchEvent(new CustomEvent('plugin-action-open', {
                detail: {
                    plugin: action.PluginName,
                    action: action.ID,
                    label: action.Label,
                    description: action.Description,
                    entityIds: [entityId],
                    entityType: entityType,
                    async: action.Async,
                    params: action.Params,
                    confirm: action.Confirm,
                }
            }));
        }
    };
}
