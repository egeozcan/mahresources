{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="list-container">
        {% for tag in tags %}
            <article class="card tag-card">
                <h3 class="card-title card-title--simple">
                    <a href="/tag?id={{ tag.ID }}">{{ tag.Name }}</a>
                </h3>
                {% if tag.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=tag.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
