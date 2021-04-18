{% extends "layouts/base.tpl" %}

{% block body %}
    {% include "./partials/album.tpl" %}
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    {% for tag in Tags %}
        {% include "./partials/album.tpl" with name=tag.Name active=true %}
    {% endfor %}
{% endblock %}