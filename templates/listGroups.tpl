{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex flex-col gap-4">
        {% for group in groups %}
            {% include "./partials/group.tpl" %}
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "./partials/sideTitle.tpl" with title="Filter" %}
    <form>
        {% include "./partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id="autocompleter"|nanoid %}
        {% include "./partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "./partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "./partials/form/searchButton.tpl" %}
    </form>
{% endblock %}