{% custom_css entity %}
{% if entity.Category.CustomMRQLResult %}
<article class="card group-card card--selectable" x-data="selectableItem({ itemId: {{ entity.ID }} })">
    <input type="checkbox" :checked="selected()" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    <div data-entity='{{ entity|json }}' x-data="{ entity: JSON.parse($el.dataset.entity) }">
        {% process_shortcodes entity.Category.CustomMRQLResult entity %}
    </div>
</article>
{% else %}
{% include "/partials/group.tpl" %}
{% endif %}
