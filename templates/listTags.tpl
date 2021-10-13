{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex gap-4 flex-wrap">
        {% for tag in tags %}
            {% include "./partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "./partials/sideTitle.tpl" with title="Filter" %}
    <form>
        {% include "./partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "./partials/form/searchButton.tpl" %}
    </form>
{% endblock %}