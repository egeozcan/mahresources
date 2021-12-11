<div class="group min-w-0">
    <div class="flex gap-3 content-center items-center mb-2 min-w-0">
        {% include "partials/avatar.tpl" with initials=entity.Initials() %}
        {% if relation && reverse %}
        <a href="/relation?id={{ relation.ID }}" class="inline-flex items-center px-3 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-800">
            <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24"><path d="M21 12l-18 12v-24z"/></svg>
            &nbsp;{{ relation.RelationType.Name }}
        </a>
        {% endif %}
        <a class="min-w-0 overflow-ellipsis break-words flex-shrink" href="/group?id={{ entity.ID }}">
            <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">
                {{ entity.GetName() }}
            </h3>
        </a>
        {% if relation && !reverse %}
            <a href="/relation?id={{ relation.ID }}" class="inline-flex items-center px-3 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-800">
                <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24"><path d="M21 12l-18 12v-24z"/></svg>
                &nbsp;{{ relation.RelationType.Name }}
            </a>
        {% endif %}
        {% include "partials/category.tpl" with name=entity.Category.Name link=withQuery("categories", stringId(entity.CategoryId), true) active=hasQuery("categories", stringId(entity.CategoryId)) %}
    </div>
    {% if !reverse && relation && relation.Description && !noRelDescription %}
        <a href="/relation?id={{ relation.ID }}">
            {% include "partials/description.tpl" with description=relation.Description preview=true %}
        </a>
    {% endif %}

    {% if !noDescription %}
        {% include "partials/description.tpl" with description=entity.Description preview=true %}
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