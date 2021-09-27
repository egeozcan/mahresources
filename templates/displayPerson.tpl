{% extends "layouts/base.tpl" %}

{% block body %}
    <p class="flex mb-6">
        <div class="flex-1">
            {{ person.Description|markdown }}
        </div>
    </p>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2 mt-6">
            Own Albums
        </div>
    </div>
    <section class="album-container">
        {% for album in person.OwnAlbums %}
            {% include "./partials/album.tpl" %}
        {% endfor %}
    </section>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2 mt-6">
            Related Albums
        </div>
    </div>
    <section class="album-container">
        {% for album in person.RelatedAlbums %}
            {% include "./partials/album.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in person.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}