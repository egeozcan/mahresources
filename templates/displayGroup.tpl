{% extends "layouts/base.tpl" %}

{% block body %}
    <p class="flex mb-6">
        <div class="flex-1">
            {{ group.Description|markdown }}
        </div>
    </p>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2 mt-6">
            Own Notes
        </div>
    </div>
    <section class="note-container">
        {% for note in group.OwnNotes %}
            {% include "./partials/note.tpl" %}
        {% endfor %}
    </section>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2 mt-6">
            Related Notes
        </div>
    </div>
    <section class="note-container">
        {% for note in group.RelatedNotes %}
            {% include "./partials/note.tpl" %}
        {% endfor %}
    </section>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2 mt-6">
            Own Resources
        </div>
    </div>
    <section class="note-container">
        {% for resource in group.OwnResources %}
            {% include "./partials/resource.tpl" %}
        {% endfor %}
    </section>
    <div class="flex">
        <div class="flex-1 text-2xl mb-2 mt-6">
            Related Resources
        </div>
    </div>
    <section class="note-container">
        {% for resource in group.RelatedResources %}
            {% include "./partials/resource.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in group.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}