<div class="note" x-data='{ "entity": {{ entity|json }} }'>
    <div class="flex gap-3">
        <div>
            {% autoescape off %}{{ entity.NoteType.CustomAvatar }}{% endautoescape %}
            {% if not entity.NoteType.CustomAvatar %}
                {% include "partials/avatar.tpl" with initials=entity.Initials() %}
            {% endif %}
        </div>
        <div class="flex-1 min-w-0">
            <a href="/note?id={{ entity.ID }}">
                <h3 class="mb-2 font-bold">{{ entity.Name }}</h3>
            </a>
            {% autoescape off %}
                {{ entity.NoteType.CustomSummary }}
            {% endautoescape %}
            {% include "partials/description.tpl" with description=entity.Description descriptionEditUrl="/blabla" preview=true %}
            <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
                {% for tag in entity.Tags %}
                    <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                        {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
                    </a>
                {% endfor %}
            </div>
        </div>
    </div>
</div>