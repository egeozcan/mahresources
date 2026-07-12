export function cardActionMenu() {
    return {
        open: false,
        toggle() {
            if (this.open) {
                this.close();
            } else {
                this.openAndFocus('first');
            }
        },
        close() { this.open = false; },
        closeAndFocus() {
            this.close();
            this.$nextTick(() => this.$refs.trigger?.focus());
        },
        openAndFocus(position = 'first') {
            this.open = true;
            this.$nextTick(() => {
                const items = this.menuItems();
                const item = position === 'last' ? items.at(-1) : items[0];
                item?.focus();
            });
        },
        menuItems() {
            return Array.from(this.$refs.menu?.querySelectorAll('[role="menuitem"]') || []);
        },
        onMenuKeydown(event) {
            const items = this.menuItems();
            const currentIndex = items.indexOf(event.target);

            if (event.key === 'Escape') {
                event.preventDefault();
                this.closeAndFocus();
                return;
            }

            let nextIndex;
            if (event.key === 'ArrowDown') nextIndex = currentIndex < 0 ? 0 : (currentIndex + 1) % items.length;
            if (event.key === 'ArrowUp') nextIndex = currentIndex < 0 ? items.length - 1 : (currentIndex - 1 + items.length) % items.length;
            if (event.key === 'Home') nextIndex = 0;
            if (event.key === 'End') nextIndex = items.length - 1;

            if (nextIndex !== undefined && items[nextIndex]) {
                event.preventDefault();
                items[nextIndex].focus();
            }
        },
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
