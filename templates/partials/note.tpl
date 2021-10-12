{% import "../macros/subTags.tpl" sub_tags %}

<div class="note">
    <a href="/note?id={{ note.ID }}">
        <h3 class="mb-2">{{ note.Name }}</h3>
        {% if note.Description != "" %}
        <p>{{ note.Description|truncatechars:250 }}</p>
        {% endif %}
    </a>
    {% if tags %}
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {{ sub_tags(tags, note.Tags) }}
    </div>
    {% endif %}
</div>