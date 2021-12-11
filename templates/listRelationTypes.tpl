{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="flex gap-4 flex-wrap">
        {% for relationType in relationTypes %}
            <a href="/relationType?id={{ relationType.ID }}">
                <div class="bg-gray-50 p-4">
                    {% include "/partials/subtitle.tpl" with title=relationType.Name %}
                    {% include "/partials/description.tpl" with description=relationType.Description %}
                </div>
            </a>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='FromCategory' title='From Category' max=1 selectedItems=fromCategories id="autocompleter"|nanoid %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='ToCategory' title='To Category' max=1 selectedItems=toCategories id="autocompleter"|nanoid %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}