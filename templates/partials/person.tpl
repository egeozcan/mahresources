{% import "../macros/subTags.tpl" sub_tags %}

<div class="person min-w-0 mt-6">
    <div class="flex gap-3 content-center items-center mb-2 min-w-0">
        {% include "./avatar.tpl" with initials=person.Initials() %}
        <a class="min-w-0 overflow-ellipsis break-words flex-shrink" href="/person?id={{ person.ID }}">
            <h3 class="min-w-0 font-bold whitespace-nowrap overflow-hidden overflow-ellipsis">{{ person.GetName() }}</h3>
        </a>
    </div>
    <p class="break-words min-w-0 overflow-ellipsis">{{ person.Description|truncatechars:40 }}</p>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {{ sub_tags(tags, person.Tags) }}
    </div>
</div>