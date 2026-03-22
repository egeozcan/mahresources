{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
{% endblock %}

{% block body %}
    {% include "/partials/timeline.tpl" with entityApiUrl="/v1/queries" entityType="queries" entityDefaultView="/queries" %}
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter queries">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Sort" %}
            {% include "/partials/form/selectInput.tpl" with name='SortBy' label='Sort' values=sortValues %}
        </div>
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Text' label='Text' value=queryValues.Text.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </div>
    </form>
{% endblock %}
