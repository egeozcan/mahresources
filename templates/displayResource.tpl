{% extends "layouts/base.tpl" %}

{% block body %}
    {% if resource.Description %}
        <div class="flex mb-6">
            <div class="flex-1">
                {{ resource.Description|markdown }}
            </div>
        </div>
    {% endif %}

    {% include "/partials/subtitle.tpl" with title="Related Notes" %}
    <section class="note-container">
        {% for entity in resource.Notes %}
            {% include "/partials/note.tpl" %}
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
    {% include "/partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in resource.Tags %}
            {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
    {% include "/partials/sideTitle.tpl" with title="Groups" %}
    <div>
        {% for group in resource.Groups %}
            {% include "/partials/group.tpl" %}
        {% endfor %}
    </div>
{% endblock %}