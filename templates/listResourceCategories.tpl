{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="list-container">
        {% for resourceCategory in resourceCategories %}
            <article class="card resource-category-card">
                <h3 class="card-title card-title--simple">
                    <a href="/resourceCategory?id={{ resourceCategory.ID }}">{{ resourceCategory.Name }}</a>
                </h3>
                {% if resourceCategory.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=resourceCategory.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% empty %}
            <div class="detail-empty">No resource categories found.</div>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        <form class="flex gap-2 items-start flex-col w-full">
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </form>
    </div>
{% endblock %}
