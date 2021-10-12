{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex">
        <div class="flex-1 text-2xl mb-2">
            Albums
        </div>
    </div>
    <section class="album-container">
        {% for album in tag.Albums %}
            {% include "./partials/album.tpl" %}
        {% endfor %}
    </section>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2">
            Resources
        </div>
    </div>
    <section class="album-container">
        {% for resource in tag.Resources %}
            {% include "./partials/resource.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    <div class="mt-2 -ml-2">
        {% for group in tag.Groups %}
            {% include "./partials/group.tpl" %}
        {% endfor %}
    </div>
{% endblock %}