{# CustomListHeader slot (Phase 6): rendered at the top of a list page filtered to exactly one category/type. listHeaderCarrier is that carrier (Category/ResourceCategory/NoteType) or nil. The slot is processed with the carrier itself as the entity, so [property path="Name"] yields the carrier name, [meta] renders empty, and [mrql] resolves at global scope. The wrapper class lets CustomCSS (emitted page-wide via custom_css) style it. #}
{% if listHeaderCarrier && listHeaderCarrier.CustomListHeader %}
<div class="custom-list-header mb-4">
    {% process_shortcodes listHeaderCarrier.CustomListHeader listHeaderCarrier %}
</div>
{% endif %}
