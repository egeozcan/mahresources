import { observeSelectorField } from '../selector/selectorRegistry.ts';

/**
 * The schema-aware meta filter on a list page. It follows one category-like selector -- named by
 * `field` and resolved within this element's own form -- and hands `schema-search-mode` either a
 * single schema or the JSON array of schemas to intersect.
 *
 * Schema fields are suppressed entirely when any selected category has no MetaSchema: an
 * intersection with an unconstrained category is meaningless, so offering its fields would
 * promise filtering the server cannot honour.
 */
export function schemaSearchFields({ field, initialCategories }) {
    return {
        schemas: [],
        _unobserveField: null,

        init() {
            this._unobserveField = observeSelectorField(this.$el, field, (change) => {
                this.applySelection(change.values);
            });
            // The editor element upgrades on the next tick; setting `schema` before then is lost.
            this.$nextTick(() => {
                const initial = initialCategories || [];
                if (initial.length > 0) this.applySelection(initial);
            });
        },

        applySelection(items) {
            if (!items || items.length === 0 || items.some((item) => !item.MetaSchema)) {
                this.schemas = [];
                this.$refs.searchEditor.setAttribute('schema', '');
                return;
            }
            this.schemas = items.map((item) => item.MetaSchema);
            this.$refs.searchEditor.setAttribute(
                'schema',
                this.schemas.length === 1 ? this.schemas[0] : JSON.stringify(this.schemas),
            );
        },

        destroy() {
            this._unobserveField?.();
            this._unobserveField = null;
        },
    };
}
