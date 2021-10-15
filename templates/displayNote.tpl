{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex-1 prose lg:prose-xl bg-gray-50 p-4">
        {% autoescape off %}{{ note.Description|markdown }}{% endautoescape %}
    </div>
    {% include "/partials/subtitle.tpl" with title="Related Resources" %}
    <section class="note-container">
        {% for entity in note.Resources %}
            {% include "/partials/resource.tpl" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in note.Tags %}
            {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
    {% include "/partials/sideTitle.tpl" with title="People" %}
    <div>
        {% for group in note.Groups %}
            {% include "/partials/group.tpl" %}
        {% endfor %}
    </div>
{% endblock %}