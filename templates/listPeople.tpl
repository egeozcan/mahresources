{% extends "layouts/base.tpl" %}

{% block body %}
    <div class="flex flex-col gap-4">
        {% for person in people %}
            {% include "./partials/person.tpl" %}
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Filter</h3>
    <form class="mt-5">
        {% include "./partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags.SelectedRelations id="autocompleter"|nanoid %}
        {% include "./partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "./partials/form/textInput.tpl" with name='Surname' label='Surname' value=queryValues.Surname.0 %}
        {% include "./partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "./partials/form/searchButton.tpl" %}
    </form>
{% endblock %}