<div class="group min-w-0">
    <div class="flex gap-3 content-center items-center mb-2 min-w-0">
        {% include "partials/avatar.tpl" with initials=group.Initials() %}
        <a class="min-w-0 overflow-ellipsis break-words flex-shrink" href="/group?id={{ group.ID }}">
            <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">{{ group.GetName() }}</h3>
        </a>
        {% include "partials/category.tpl" with name=group.Category.Name link=withQuery("categories", stringId(group.Category.ID), true) active=hasQuery("categories", stringId(group.Category.ID)) %}
    </div>
    {% include "partials/description.tpl" with description=group.Description %}
    <div class="tags mt-3 mb-2">
        {% for tag in group.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
        {% endfor %}
    </div>
</div>