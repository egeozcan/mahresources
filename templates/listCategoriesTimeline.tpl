{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
{% endblock %}

{% block body %}
    {% include "/partials/timeline.tpl" with entityApiUrl="/v1/categories" entityType="categories" entityDefaultView="/categories" %}
{% endblock %}

{% block sidebar %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter categories">
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </form>
    </div>
{% endblock %}
