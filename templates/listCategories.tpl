{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
{% endblock %}

{% block body %}
    <div class="list-container">
        {% for category in categories %}
            <article class="card category-card">
                <h3 class="card-title card-title--simple">
                    <a href="/category?id={{ category.ID }}">{{ category.Name }}</a>
                </h3>
                {% if category.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=category.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% empty %}
            <div class="detail-empty">No categories found. <a href="/category/new" class="text-amber-700 hover:text-amber-900 underline">Create one</a>.</div>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter categories">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Sort" %}
            {% include "/partials/form/multiSortInput.tpl" with name='SortBy' values=sortValues %}
        </div>
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </div>
    </form>
{% endblock %}
