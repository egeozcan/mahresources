{% custom_css entity %}
{% if entity.NoteType.CustomMRQLResult %}
<article class="card note-card card--selectable" x-data="selectableItem({ itemId: {{ entity.ID }} })">
    <input type="checkbox" :checked="selected()" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    <div data-entity='{{ entity|json }}' x-data="{ entity: JSON.parse($el.dataset.entity) }">
        {% process_shortcodes entity.NoteType.CustomMRQLResult entity %}
    </div>
</article>
{% else %}
{% include "/partials/note.tpl" %}
{% endif %}
