<div class="note">
    <a href="/note?id={{ note.ID }}">
        <h3 class="mb-2">{{ note.Name }}</h3>
        {% if note.Description != "" %}
        <p>{{ note.Description|truncatechars:250 }}</p>
        {% endif %}
    </a>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {% for tag in note.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "./tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
        {% endfor %}
    </div>
</div>