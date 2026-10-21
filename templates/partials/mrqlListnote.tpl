{% custom_css entity %}
{% if entity.NoteType.CustomMRQLResult %}
{% card_entity entity %}
<article class="card note-card card--selectable" x-data="selectableItem({ itemId: {{ entity.ID }} })">
    <input type="checkbox" :checked="selected()" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    <div data-entity='{% card_entity_attr %}' x-data="{ entity: JSON.parse($el.dataset.entity) }">
        {% process_shortcodes entity.NoteType.CustomMRQLResult entity %}
    </div>
</article>
{% endcard_entity %}
{% else %}
{% include "/partials/note.tpl" %}
{% endif %}
