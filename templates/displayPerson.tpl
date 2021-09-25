{% extends "layouts/base.tpl" %}

{% block body %}
    <h3>{{ person.Name }}</h3>
    <div class="flex">
        <div class="flex-1">
            {{ person.Description }}
        </div>
    </div>
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Tags</h3>
    <div class="mt-2 -ml-2">
        {% for tag in person.Tags %}
            {% include "./partials/tag.tpl" with name=tag.Name %}
        {% endfor %}
    </div>
{% endblock %}