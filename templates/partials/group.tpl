<div class="group min-w-0">
    <div class="flex gap-3 content-center items-center mb-2 min-w-0">
        {% include "partials/avatar.tpl" with initials=entity.Initials() %}
        <a class="min-w-0 overflow-ellipsis break-words flex-shrink" href="/group?id={{ entity.ID }}">
            <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">{{ entity.GetName() }}</h3>
        </a>
        {% include "partials/category.tpl" with name=entity.Category.Name link=withQuery("categories", stringId(entity.Category.ID), true) active=hasQuery("categories", stringId(entity.Category.ID)) %}
    </div>
    {% include "partials/description.tpl" with description=entity.Description %}
    <div class="tags mt-3 mb-2">
        {% for tag in entity.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
        {% endfor %}
    </div>
</div>