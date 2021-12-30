<div class="resource {% if selectable %} pl-4 {% endif %}"  {% if selectable %} x-data="selectableItem({ itemId: {{ entity.ID }} })" {% endif %}>
    <div x-data="{ entity: {{ entity|json }} }">
        <div class="flex gap-2 items-center">
            {% if selectable %}
            <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" class="focus:ring-indigo-500 h-8 w-8 text-indigo-600 border-gray-300 rounded">
            {% endif %}
            <a class="min-w-0" href="/resource?id={{ entity.ID }}">
                <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">{{ entity.Name }}</h3>
                <h4>{{ entity.FileSize | humanReadableSize }}</h4>
            </a>
        </div>
        {% include "partials/description.tpl" with description=entity.Description preview=true %}
        <a href="/{% if entity.StorageLocation %}{{ entity.StorageLocation }}{% else %}files{% endif %}{{ entity.Location }}">
            <img height="300" src="/v1/resource/preview?id={{ entity.ID }}&height=300" alt="Preview">
        </a>
        <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
            {% for tag in entity.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
            {% endfor %}
        </div>
        {% if entity.Owner %}
        <p>
            Owner: <a href="/group?id={{ entity.Owner.ID }}">{{ entity.Owner.Name }}</a>
        </p>
        {% endif %}
    </div>
</div>