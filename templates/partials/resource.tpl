{% import "../macros/subTags.tpl" sub_tags %}

<div class="resource">
    <a href="/resource?id={{ resource.ID }}">
        <h3>{{ resource.Name }}</h3>
        <h4>{{ resource.FileSize | humanReadableSize }}</h4>
    </a>
    <a href="/files/{{ resource.Location }}">
        {% if resource.PreviewContentType != "" && len(resource.Preview) != 0 %}
            <img src="data:{{ resource.PreviewContentType }};base64,{{ resource.Preview|base64 }}" alt="">
        {% else %}
            <img src="/public/placeholders/file.jpg" alt="">
        {% endif %}
    </a>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {% for tag in resource.Tags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {% include "./tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.ID)) %}
            </a>
        {% endfor %}
    </div>
</div>