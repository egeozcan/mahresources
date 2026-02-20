{% extends "/layouts/base.tpl" %}

{% block prebody %}
{% endblock %}

{% block body %}
    {% if not readOnlyEnforced %}
    <div class="bg-yellow-100 border-l-4 border-yellow-500 text-yellow-700 p-4 mb-4" role="alert">
        <p class="font-bold">Warning</p>
        <p>Queries run without database-level read-only enforcement. Configure a separate <code>DB_READONLY_DSN</code> with read-only access for safety.</p>
    </div>
    {% endif %}
    <div class="flex flex-col gap-4 items-container">
        {% for entity in queries %}
            {% include "/partials/query.tpl" %}
        {% endfor %}
    </div>
{% endblock %}


{% block sidebar %}
    <form class="flex gap-2 items-start flex-col" aria-label="Filter queries">
        {% include "/partials/sideTitle.tpl" with title="Sort" %}
        {% include "/partials/form/selectInput.tpl" with name='SortBy' label='Sort' values=sortValues %}
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Text' label='Text' value=queryValues.Text.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}