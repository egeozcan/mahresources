<div class="resource">
    <a href="/resource?id={{ entity.ID }}">
        <h3>{{ entity.Name }}</h3>
        <h4>{{ entity.FileSize | humanReadableSize }}</h4>
    </a>
    <a href="/files/{{ entity.Location }}">
        {% if entity.PreviewContentType != "" && len(entity.Preview) != 0 %}
            <img src="data:{{ entity.PreviewContentType }};base64,{{ entity.Preview|base64 }}" alt="">
        {% else %}
            <img src="/public/placeholders/file.jpg" alt="">
        {% endif %}
    </a>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {% for tag in entity.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
        {% endfor %}
    </div>
</div>