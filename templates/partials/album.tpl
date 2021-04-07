{% import "../macros/subTags.tpl" sub_tags %}

<div class="album">
    <a href="/album/{{ album.ID }}">
        <h3>{{ album.Name }}</h3>
        {% if album.PreviewContentType != "" && len(album.Preview) != 0 %}
        <img src="data:{{ album.PreviewContentType }};base64,{{ album.Preview|base64 }}" alt="">
        {% endif %}
    </a>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {{ sub_tags(tags, album.Tags) }}
    </div>
</div>