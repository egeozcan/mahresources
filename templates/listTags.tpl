{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex gap-4 flex-wrap">
        {% for tag in tags %}
            {% include "./partials/tag.tpl" with name=tag.Name %}
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Filter</h3>
    <form class="mt-5">
        {% include "./partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "./partials/form/searchButton.tpl" %}
    </form>
{% endblock %}