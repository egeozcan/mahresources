{% extends "/layouts/base.tpl" %}

{% block body %}
    <form method="POST" action="/v1/series">
        <input type="hidden" name="ID" value="{{ series.ID }}">
        <div class="mb-4">
            <label class="block text-sm font-medium text-gray-700" for="series-name">Name</label>
            <input type="text" name="Name" id="series-name" value="{{ series.Name }}" class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm">
        </div>
        <div class="mb-4">
            {% include "/partials/form/freeFields.tpl" with name="Meta" fromJSON=series.Meta jsonOutput="true" id=getNextId("freeField") %}
        </div>
        {% include "partials/form/searchButton.tpl" with text="Save" %}
    </form>

    <section class="mt-8">
        {% include "partials/subtitle.tpl" with small=true title="Resources in Series" %}
        {% if series.Resources %}
        <div class="list-container mt-4">
            {% for entity in series.Resources %}
                {% include partial("resource") %}
            {% endfor %}
        </div>
        {% else %}
        <p>No resources in this series.</p>
        {% endif %}
    </section>
{% endblock %}

{% block sidebar %}

{% endblock %}
