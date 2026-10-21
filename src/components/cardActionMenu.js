import { createLiveRegion } from '../utils/ariaLiveRegion.js';

// Plugin action definitions for card menus, one request per entity type per page. A
// card's menu items carry only the plugin and action ids: inlining each definition
// into every card's @click made it most of a list page's HTML (an action's params
// run to kilobytes) and gave Alpine one handler to compile per card per action.
const actionDefinitions = new Map();

function loadActionDefinitions(entityType) {
    if (!entityType) return Promise.resolve(new Map());
    let pending = actionDefinitions.get(entityType);
    if (!pending) {
        pending = fetch(`/v1/plugin/actions?entity=${encodeURIComponent(entityType)}`)
            .then(response => {
                if (!response.ok) throw new Error(`plugin actions: HTTP ${response.status}`);
                return response.json();
            })
            .then(actions => new Map((actions || []).map(action => [`${action.plugin_name}\u0000${action.id}`, action])))
            .catch(error => {
                // Not cached: the next click asks again.
                actionDefinitions.delete(entityType);
                throw error;
            });
        actionDefinitions.set(entityType, pending);
    }
    return pending;
}

// Every activation takes a number; only the newest may open a dialog. Lookups for
// different entity types answer in any order, and an older one arriving late
// would otherwise replace the dialog the reader is already filling in.
let latestActivation = 0;
let failureRegion = null;

function announceFailure(message) {
    failureRegion ??= createLiveRegion(document.body, { assertive: true });
    failureRegion.announce(message);
}

export function resetCardActionDefinitions() {
    actionDefinitions.clear();
}

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
            loadActionDefinitions(this.$refs.trigger?.dataset?.actionEntityType).catch(() => {});
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
        async runAction(item, { announce = announceFailure } = {}) {
            // The trigger, captured before close() hides the menu. The menu item the
            // reader activated stays in the document but goes display:none, so it
            // cannot take focus back when the action modal closes; the button that
            // opened the menu is still on screen and is where they came from.
            const trigger = this.$refs.trigger;
            const selection = this.$selection;
            const entityType = trigger?.dataset?.actionEntityType;
            const { actionPlugin, actionId, actionEntityId } = item.dataset;
            const label = item.textContent?.trim() || actionId;
            const activation = ++latestActivation;
            this.close();
            let action;
            try {
                action = (await loadActionDefinitions(entityType)).get(`${actionPlugin}\u0000${actionId}`);
            } catch (error) {
                console.error('Could not load plugin actions:', error);
            }
            if (activation !== latestActivation) return;
            if (!action) {
                // The menu item that had focus is hidden now; put the reader back
                // on the button they opened the menu from, and say why nothing opened.
                trigger?.focus();
                announce(`${label} could not be opened. Try again, or reload the page.`);
                return;
            }
            window.dispatchEvent(new CustomEvent('plugin-action-open', {
                detail: {
                    returnFocusTo: trigger,
                    plugin: action.plugin_name,
                    action: action.id,
                    label: action.label,
                    description: action.description,
                    entityIds: [Number(actionEntityId)],
                    entityType: entityType,
                    async: action.async,
                    params: action.params,
                    confirm: action.confirm,
                    filters: action.filters,
                    bulk_max: action.bulk_max,
                    selection,
                }
            }));
        }
    };
}
