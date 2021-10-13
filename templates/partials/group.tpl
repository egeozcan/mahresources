<div class="group min-w-0">
    <div class="flex gap-3 content-center items-center mb-2 min-w-0">
        {% include "./avatar.tpl" with initials=group.Initials() %}
        <a class="min-w-0 overflow-ellipsis break-words flex-shrink" href="/group?id={{ group.ID }}">
            <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">{{ group.GetName() }}</h3>
        </a>
    </div>
    <p class="break-words min-w-0 overflow-ellipsis">{{ group.Description|truncatechars:40 }}</p>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {% for tag in group.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "./tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
        {% endfor %}
    </div>
</div>