<article class="card note-card" x-data='{ "entity": {{ entity|json }} }'>
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
        <a class="card-badge{% if hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
            {{ tag.Name }}
        </a>
        {% endfor %}
    </div>
    {% endif %}
</article>
