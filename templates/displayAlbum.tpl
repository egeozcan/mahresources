{% extends "layouts/base.tpl" %}

{% block body %}
    <h3>{{ album.Name }}</h3>
    <div class="flex">
        <div class="flex-1">
            {{ album.Description }}
        </div>
    </div>
{% endblock %}

{% block sidebar %}
    {% if album.PreviewContentType != "" && len(album.Preview) != 0 %}
        <img class="mb-2" src="data:{{ album.PreviewContentType }};base64,{{ album.Preview|base64 }}" alt="">
    {% endif %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in album.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name %}
        {% endfor %}
    </div>
{% endblock %}