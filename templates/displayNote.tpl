{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex">
        <p class="flex-1 mb-6">
            {{ note.Description }}
        </p>
    </div>
    <section class="note-container">
        {% for resource in note.Resources %}
            {% include "./partials/resource.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in note.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
    <div class="mt-2 -ml-2">
        {% for group in note.Groups %}
            {% include "./partials/group.tpl" %}
        {% endfor %}
    </div>
{% endblock %}