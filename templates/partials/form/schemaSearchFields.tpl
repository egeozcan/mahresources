<div
    x-data="{
        schemas: [],
        handleCategoryChange(items) {
            if (!items || items.length === 0) {
                this.schemas = [];
                this.$refs.searchEditor.setAttribute('schema', '');
                return;
            }
            this.schemas = items.map(i => i.MetaSchema).filter(Boolean);
            if (this.schemas.length === 1) {
                this.$refs.searchEditor.setAttribute('schema', this.schemas[0]);
            } else if (this.schemas.length > 1) {
                this.$refs.searchEditor.setAttribute('schema', JSON.stringify(this.schemas));
            } else {
                this.$refs.searchEditor.setAttribute('schema', '');
            }
        }
    }"
    x-init="$nextTick(() => { const initial = {{ initialCategories|json }} || []; if (initial.length > 0) handleCategoryChange(initial); })"
    @multiple-input.window="if ($event.detail.name === '{{ elName }}') handleCategoryChange($event.detail.value)"
    class="w-full"
>
    <schema-editor
        x-ref="searchEditor"
        mode="search"
        schema=""
        meta-query='{{ existingMetaQuery|json }}'
        field-name="MetaQuery"
    ></schema-editor>
</div>
