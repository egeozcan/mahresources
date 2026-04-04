<div
    x-data="{
        schemas: [],
        handleCategoryChange(items) {
            if (!items || items.length === 0) {
                this.schemas = [];
                this.$refs.searchEditor.setAttribute('schema', '');
                return;
            }
            // If any selected category lacks a MetaSchema, schema fields cannot be
            // meaningfully intersected — suppress them entirely.
            if (items.some(i => !i.MetaSchema)) {
                this.schemas = [];
                this.$refs.searchEditor.setAttribute('schema', '');
                return;
            }
            this.schemas = items.map(i => i.MetaSchema);
            if (this.schemas.length === 1) {
                this.$refs.searchEditor.setAttribute('schema', this.schemas[0]);
            } else {
                this.$refs.searchEditor.setAttribute('schema', JSON.stringify(this.schemas));
            }
        }
    }"
    x-init="$nextTick(() => { const initial = {{ initialCategories|json }} || []; if (initial.length > 0) handleCategoryChange(initial); })"
    @multiple-input.window="if ($event.detail.name === '{{ elName }}') handleCategoryChange($event.detail.value)"
    class="w-full"
>
    <schema-search-mode
        x-ref="searchEditor"
        schema=""
        meta-query='{{ existingMetaQuery|json }}'
        field-name="MetaQuery"
    ></schema-search-mode>
</div>
