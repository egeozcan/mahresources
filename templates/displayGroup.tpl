{% extends "layouts/base.tpl" %}

{% block body %}

    {% if group.Description %}
    <div class="flex mb-6">
        <div class="flex-1">
            {{ group.Description|markdown }}
        </div>
    </div>
    {% endif %}

    {% if group.OwnNotes %}
        {% include "./partials/subtitle.tpl" with title="Notes" %}
        <section class="note-container">
            {% for note in group.OwnNotes %}
                {% include "./partials/note.tpl" %}
            {% endfor %}
        </section>
    {% endif %}

    {% if group.OwnResources %}
        {% include "./partials/subtitle.tpl" with title="Resources" %}
        <section class="note-container">
            {% for resource in group.OwnResources %}
            {% include "./partials/resource.tpl" %}
            {% endfor %}
        </section>
    {% endif %}

    {% if group.RelatedNotes %}
        {% include "./partials/subtitle.tpl" with title="Related Notes" %}
        <section class="note-container">
            {% for note in group.RelatedNotes %}
                {% include "./partials/note.tpl" %}
            {% endfor %}
        </section>
    {% endif %}

    {% if group.RelatedResources %}
        {% include "./partials/subtitle.tpl" with title="Related Resources" %}
        <section class="note-container">
            {% for resource in group.RelatedResources %}
                {% include "./partials/resource.tpl" %}
            {% endfor %}
        </section>
    {% endif %}

{% endblock %}

{% block sidebar %}
    {% include "./partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in group.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}