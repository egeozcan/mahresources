{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="list-container">
        {% for partial in partials %}
            <article class="card template-partial-card">
                <h3 class="card-title card-title--simple">
                    <a href="/templatePartial?id={{ partial.ID }}">{{ partial.Name }}</a>
                </h3>
                {% if partial.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=partial.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% empty %}
            <div class="detail-empty">No template partials found.</div>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter template partials">
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </form>
    </div>
{% endblock %}
