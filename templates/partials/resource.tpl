{% import "../macros/subTags.tpl" sub_tags %}

<div class="resource">
    <a href="/resource/{{ resource.ID }}">
        <h3>{{ resource.Name }}</h3>
        {% if resource.PreviewContentType != "" && len(resource.Preview) != 0 %}
        <img src="data:{{ resource.PreviewContentType }};base64,{{ resource.Preview|base64 }}" alt="">
        {% endif %}
    </a>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {{ sub_tags(tags, resource.Tags) }}
    </div>
</div>