{# Follows the `{{ elName }}` selector in this same form through the selector registry. #}
<div
    x-data="schemaSearchFields({ field: '{{ elName }}', initialCategories: {{ initialCategories|json }} || [] })"
    class="w-full"
>
    <schema-search-mode
        x-ref="searchEditor"
        schema=""
        meta-query='{{ existingMetaQuery|json }}'
        field-name="MetaQuery"
    ></schema-search-mode>
</div>
