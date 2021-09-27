{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex mb-6">
        <p class="flex-1">
            {{ resource.Description }}
        </p>
    </div>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2">
            Albums
        </div>
    </div>
    <section class="album-container">
        {% for album in resource.Albums %}
            {% include "./partials/album.tpl" %}
        {% endfor %}
    </section>
{% endblock %}


{% block sidebar %}
    <a href="/files/{{ resource.Location }}">
        {% if resource.PreviewContentType != "" && len(resource.Preview) != 0 %}
            <img src="data:{{ resource.PreviewContentType }};base64,{{ resource.Preview|base64 }}" alt="">
        {% else %}
            <img src="/public/placeholders/file.jpg" alt="">
        {% endif %}
    </a>
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in resource.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">People</h3>
    <div class="mt-2 -ml-2">
        {% for person in resource.People %}
            {% include "./partials/person.tpl" %}
        {% endfor %}
    </div>
{% endblock %}