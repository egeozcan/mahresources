{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex">
        <p class="flex-1 mb-6">
            {{ album.Description }}
        </p>
    </div>
    <section class="album-container">
        {% for resource in album.Resources %}
            {% include "./partials/resource.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    {% if album.PreviewContentType != "" && len(album.Preview) != 0 %}
        <img class="mb-2" src="data:{{ album.PreviewContentType }};base64,{{ album.Preview|base64 }}" alt="">
    {% endif %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in album.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
    <div class="mt-2 -ml-2">
        {% for person in album.People %}
            {% include "./partials/person.tpl" %}
        {% endfor %}
    </div>
{% endblock %}