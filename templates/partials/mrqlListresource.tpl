{% custom_css entity %}
{% if entity.ResourceCategory.CustomMRQLResult %}
<article class="card resource-card card--selectable" x-data="selectableItem({ itemId: {{ entity.ID }} })">
    <input type="checkbox" :checked="selected()" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    <div data-entity='{{ entity|json }}' x-data="{ entity: JSON.parse($el.dataset.entity) }">
        {% process_shortcodes entity.ResourceCategory.CustomMRQLResult entity %}
    </div>
</article>
{% else %}
{% include "/partials/resource.tpl" %}
{% endif %}
