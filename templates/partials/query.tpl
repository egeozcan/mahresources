{% if entity %}
<div class="query min-w-0 flex gap-4" {% if selectable %} x-data="selectableItem({ itemId: {{ entity.ID }} })" {% endif %}>
    <div class="group min-w-0 flex gap-4" {% if selectable %} x-data="selectableItem({ itemId: {{ entity.ID }} })" {% endif %}>
        {% if selectable %}
            <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="mt-4 focus:ring-indigo-500 h-8 w-8 text-indigo-600 border-gray-300 rounded">
        {% endif %}
        <div class="flex gap-2 min-w-0 justify-between flex-1 flex-wrap" x-data='{ "entity": {{ entity|json }} }'>
            <div class="min-w-0 max-w-full flex-shrink">
                <div class="flex gap-3 content-center items-center mb-2 min-w-0">
                    <a class="min-w-0 overflow-ellipsis break-words flex-shrink" href="/query?id={{ entity.ID }}" title="{{ entity.Name }}">
                        <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">
                            {{ entity.Name }}
                        </h3>
                        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-600">Updated: </span>{{ entity.UpdatedAt|date:"2006-01-02 15:04" }}</small>
                        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-600">Created: </span>{{ entity.CreatedAt|date:"2006-01-02 15:04" }}</small>
                    </a>
                </div>

                {% if !noDescription %}
                    {% include "partials/description.tpl" with description=entity.Text preview=true %}
                {% endif %}
            </div>
        </div>
    </div>
</div>
{% endif %}