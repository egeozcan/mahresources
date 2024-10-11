{% if entity %}
<div class="group min-w-0 flex gap-4" {% if selectable %} x-data="selectableItem({ itemId: {{ entity.ID }} })" {% endif %}>
    {% if selectable %}
        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" class="mt-4 focus:ring-indigo-500 h-8 w-8 text-indigo-600 border-gray-300 rounded">
    {% endif %}
    <div class="flex gap-2 min-w-0 justify-between flex-1 flex-wrap" x-data='{ "entity": {{ entity|json }} }'>
        <div class="min-w-0 max-w-full flex-shrink">
            <div class="flex gap-3 content-center items-center mb-2 min-w-0">
                {% if !fullText %}
                    {% include "partials/avatar.tpl" with initials=entity.Initials() %}
                {% endif %}
                <div>
                    <div class="flex gap-3 content-center items-center min-w-0">
                        {% if relation && reverse %}
                            <a href="/relation?id={{ relation.ID }}" class="inline-flex items-center px-3 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-800">
                                <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24"><path d="M21 12l-18 12v-24z"/></svg>
                                &nbsp;{{ relation.RelationType.Name }}
                            </a>
                        {% endif %}
                        <a class="overflow-hidden min-w-0 overflow-ellipsis break-words flex-shrink" href="/group?id={{ entity.ID }}" title="{{ entity.GetName() }}">
                            <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">
                                {{ entity.GetName() }}
                            </h3>
                            {% if !fullText %}
                                <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-400">Updated: </span>{{ entity.UpdatedAt|date:"2006-01-02 15:04" }}</small>
                                <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-400">Created: </span>{{ entity.CreatedAt|date:"2006-01-02 15:04" }}</small>
                            {% endif %}
                        </a>
                        {% if relation && !reverse %}
                            <a href="/relation?id={{ relation.ID }}" class="inline-flex items-center px-3 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-800">
                                <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24"><path d="M21 12l-18 12v-24z"/></svg>
                                &nbsp;{{ relation.RelationType.Name }}
                            </a>
                        {% endif %}
                        {% if entity.Category && !fullText %}
                            {% include "partials/category.tpl" with name=entity.Category.Name link=withQuery("categories", stringId(entity.CategoryId), true) active=hasQuery("categories", stringId(entity.CategoryId)) %}
                        {% endif %}
                    </div>
                    {% if entity.URL && !fullText %}
                        <a class="block text-blue-600" target="_blank" referrerpolicy="no-referrer" href="{{ entity.URL|printUrl }}">{{ entity.URL|printUrl }}</a>
                    {% endif %}
                </div>
            </div>
            {% autoescape off %}
                {{ entity.Category.CustomSummary }}
            {% endautoescape %}

            {% if !reverse && relation && relation.Description && !noRelDescription %}
                <a target="_blank" href="/relation?id={{ relation.ID }}" referrerpolicy="no-referrer">
                    {% include "partials/description.tpl" with description=relation.Description preview=!fullText %}
                </a>
            {% endif %}

            {% if !noDescription %}
                {% include "partials/description.tpl" with description=entity.Description preview=!fullText %}
            {% endif %}

            {% if !noTag %}
                <div class="tags mt-3 mb-2">
                    {% for tag in entity.Tags %}
                    <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                        {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
                    </a>
                    {% endfor %}
                </div>
            {% endif %}
        </div>
    </div>
</div>
{% endif %}