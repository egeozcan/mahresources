<article class="card note-card{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% else %}x-data='{ "entity": {{ entity|json }} }'{% endif %}>
    {% if selectable %}
    <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-indigo-500 h-6 w-6 text-indigo-600 border-gray-300 rounded">
    {% endif %}
    <header class="card-header">
        <div class="card-avatar">
            {% autoescape off %}{{ entity.NoteType.CustomAvatar }}{% endautoescape %}
            {% if not entity.NoteType.CustomAvatar %}
                {% include "partials/avatar.tpl" with initials=entity.Initials() %}
            {% endif %}
        </div>
        <div class="card-title-section">
            <h3 class="card-title">
                <a href="/note?id={{ entity.ID }}">{{ entity.Name }}</a>
            </h3>
            {% autoescape off %}
                {{ entity.NoteType.CustomSummary }}
            {% endautoescape %}
        </div>
    </header>

    <div class="card-description">
        {% include "partials/description.tpl" with description=entity.Description descriptionEditUrl="/blabla" preview=true %}
    </div>

    {% if entity.Tags %}
    <div class="tags card-tags">
        {% for tag in entity.Tags %}
        <a class="card-badge{% if !tagBaseUrl && hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{% if tagBaseUrl %}{{ tagBaseUrl }}?tags={{ tag.ID }}{% else %}{{ withQuery("tags", stringId(tag.ID), true) }}{% endif %}'>
            {{ tag.Name }}
        </a>
        {% endfor %}
    </div>
    {% endif %}
    {% include "partials/pluginActionsCard.tpl" with entityType="note" %}
</article>
