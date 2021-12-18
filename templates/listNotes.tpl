{% extends "/layouts/gallery.tpl" %}

{% block gallery %}
    {% for entity in notes %}
        {% include "/partials/note.tpl" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Text' value=queryValues.Description.0 %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id="autocompleter"|nanoid %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id="autocompleter"|nanoid extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owners id="autocompleter"|nanoid extraInfo="Category" %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/notes/meta/keys' fields=parsedQuery.MetaQuery id="freeField"|nanoid %}
        {% include "/partials/form/dateInput.tpl" with name='StartDateBefore' label='Start Date Before' value=queryValues.StartDateBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='StartDateAfter' label='Start Date After' value=queryValues.StartDateAfter.0 %}
        {% include "/partials/form/dateInput.tpl" with name='EndDateBefore' label='End Date Before' value=queryValues.EndDateBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='EndDateAfter' label='End Date After' value=queryValues.EndDateAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}