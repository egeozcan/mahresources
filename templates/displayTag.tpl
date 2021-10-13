{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex">
        <div class="flex-1 text-2xl mb-2">
            Notes
        </div>
    </div>
    <section class="note-container">
        {% for note in tag.Notes %}
            {% include "./partials/note.tpl" %}
        {% endfor %}
    </section>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2">
            Resources
        </div>
    </div>
    <section class="note-container">
        {% for resource in tag.Resources %}
            {% include "./partials/resource.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    <div>
        {% for group in tag.Groups %}
            {% include "./partials/group.tpl" %}
        {% endfor %}
    </div>
{% endblock %}