{% import "../macros/subTags.tpl" sub_tags %}

<div class="album">
    <a href="/album?id={{ album.ID }}">
        <h3 class="mb-2">{{ album.Name }}</h3>
        {% if album.PreviewContentType != "" && len(album.Preview) != 0 %}
        <img src="data:{{ album.PreviewContentType }};base64,{{ album.Preview|base64 }}" alt="">
        {% else %}
        <img src="/public/placeholders/album.svg" alt="">
        {% endif %}
    </a>
    {% if tags %}
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {{ sub_tags(tags, album.Tags) }}
    </div>
    {% endif %}
</div>