{# CustomListFooter slot: rendered at the bottom of a list page filtered to exactly one category/type, below the results and above the pager. It shares listHeaderCarrier with customListHeader.tpl — the variable is named for the header only because it predates this slot; both slots bind the same carrier under the same rule. Processed with the carrier itself as the entity, so [property path="Name"] yields the carrier name, [meta] renders empty, and [mrql] resolves at global scope. The wrapper class lets CustomCSS (emitted page-wide via custom_css) style it. #}
{% if listHeaderCarrier && listHeaderCarrier.CustomListFooter %}
<div class="custom-list-footer mt-4">
    {% process_shortcodes listHeaderCarrier.CustomListFooter listHeaderCarrier %}
</div>
{% endif %}
