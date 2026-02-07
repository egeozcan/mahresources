{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="list-container">
        {% for relationType in relationTypes %}
            <article class="card relation-type-card">
                <h3 class="card-title card-title--simple">
                    <a href="/relationType?id={{ relationType.ID }}">{{ relationType.Name }}</a>
                </h3>
                {% if relationType.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=relationType.Description preview=true %}
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
        {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='FromCategory' title='From Category' max=1 selectedItems=fromCategories id=getNextId("autocompleter") %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='ToCategory' title='To Category' max=1 selectedItems=toCategories id=getNextId("autocompleter") %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
