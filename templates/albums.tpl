{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for album in albums %}
        {% include "./partials/album.tpl" %}
    {% endfor %}
{% endblock %}