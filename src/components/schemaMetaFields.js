import { observeSelectorField } from '../selector/selectorRegistry.ts';

/**
 * Parses a MetaSchema string only if it describes an object schema. An entity whose schema is
 * blank, malformed, or a bare scalar falls back to free-form meta fields rather than rendering
 * an editor for something schema-form-mode cannot drive.
 */
function objectSchemaOrNull(rawSchema) {
    if (!rawSchema) return null;
    try {
        const parsed = JSON.parse(rawSchema);
        return parsed && typeof parsed === 'object' ? rawSchema : null;
    } catch {
        return null;
    }
}

/**
 * The entity meta editor on a create/edit form. It follows one category-like selector -- named
 * by `field` and resolved within this element's own form -- and swaps between the schema-driven
 * editor and free-form meta fields as that selection changes.
 *
 * The initial schema and meta arrive as data attributes on the root element so the server can
 * render an already-categorized entity without a round trip through the selector.
 */
export function schemaMetaFields({ field }) {
    return {
        currentSchema: null,
        currentMeta: {},
        metaEdited: false,
        _unobserveField: null,

        init() {
            this.currentSchema = objectSchemaOrNull(this.$el.dataset.initialSchema);
            try {
                this.currentMeta = JSON.parse(this.$el.dataset.initialMeta || '{}');
            } catch {
                this.currentMeta = {};
            }
            this._unobserveField = observeSelectorField(this.$el, field, (change) => {
                this.applySelection(change.values);
            });
        },

        applySelection(values) {
            this.currentSchema = objectSchemaOrNull(values?.[0]?.MetaSchema);
        },

        handleMetaChange(event) {
            if (event.detail && event.detail.value !== undefined) {
                this.currentMeta = event.detail.value;
                this.metaEdited = true;
            }
        },

        destroy() {
            this._unobserveField?.();
            this._unobserveField = null;
        },
    };
}
