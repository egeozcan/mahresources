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
        <a class="card-badge{% if !tagBaseUrl && hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{% if tagBaseUrl %}{{ tagBaseUrl }}?tags={{ tag.ID }}{% else %}{{ withQuery("tags", stringId(tag.ID), true) }}{% endif %}'>
            {{ tag.Name }}
        </a>
        {% endfor %}
    </div>
    {% endif %}
    {% if pluginCardActions %}
    <div x-data="cardActionMenu()" @click.outside="close()" class="card-tags inline-block relative">
        <button @click="toggle()" class="card-badge card-badge--action" aria-label="More actions" aria-haspopup="true" :aria-expanded="open">
            &#x22EF;
        </button>
        <div x-show="open" x-cloak class="absolute right-0 mt-1 w-48 bg-white dark:bg-gray-800 shadow-lg rounded-md z-50 border border-gray-200 dark:border-gray-600" role="menu">
            {% for action in pluginCardActions %}
            <button @click="runAction({{ action|json }}, {{ entity.ID }}, 'note')"
                    class="block w-full text-left px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-gray-700" role="menuitem">
                {{ action.Label }}
            </button>
            {% endfor %}
        </div>
    </div>
    {% endif %}
</article>
