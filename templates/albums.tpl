{% extends "base.tpl" %}

{% block body %}
    {% for album in albums %}
        <div class="album">
            <h3>{{ album.Name }}</h3>
        </div>
    {% endfor %}
{% endblock %}