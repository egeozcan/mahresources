{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Series Details</h2>
        </div>
        <div class="detail-panel-body">
            <form method="POST" action="/v1/series">
                <input type="hidden" name="ID" value="{{ series.ID }}">
                <div class="mb-4">
                    <label class="block text-sm font-medium font-mono text-stone-700" for="series-name">Name</label>
                    <input type="text" name="Name" id="series-name" value="{{ series.Name }}" class="mt-1 block w-full rounded-md border-stone-300 shadow-sm focus:border-amber-600 focus:ring-amber-600 sm:text-sm">
                </div>
                <div class="mb-4">
                    {% include "/partials/form/freeFields.tpl" with name="Meta" url="" fromJSON=series.Meta jsonOutput="true" id=getNextId("freeField") %}
                </div>
                {% include "partials/form/searchButton.tpl" with text="Save" %}
            </form>
        </div>
    </div>

    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Resources{% if series.Resources %}<span class="detail-panel-count">{{ series.Resources|length }}</span>{% endif %}</h2>
        </div>
        <div class="detail-panel-body">
            {% if series.Resources %}
            <div class="list-container">
                {% for entity in series.Resources %}
                    {% include partial("resource") %}
                {% endfor %}
            </div>
            {% else %}
            <div class="detail-empty">No resources in this series</div>
            {% endif %}
        </div>
    </div>
{% endblock %}

{% block sidebar %}

{% endblock %}
