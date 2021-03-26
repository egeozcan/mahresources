{% extends "base.tpl" %}

{% block head %}
    <style>
        * {
            color: red;
        }
    </style>
{% endblock %}

{% block body %}
    {% for album in albums %}
        <div class="album">
            <h3>{{ album.Name }}</h3>
            {% if album.PreviewContentType != "" && len(album.Preview) != 0 %}
            <img src="data:{{ album.PreviewContentType }};base64,{{ album.Preview|base64 }}" alt="">
            {% endif %}
        </div>
    {% endfor %}
{% endblock %}