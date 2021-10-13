{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for resource in resources %}
        {% include "./partials/resource.tpl" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
{% include "./partials/sideTitle.tpl" with title="Filter" %}
<form>
    {% include "./partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id="autocompleter"|nanoid %}
    {% include "./partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id="autocompleter"|nanoid %}
    {% include "./partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id="autocompleter"|nanoid %}
    {% include "./partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
    {% include "./partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
    {% include "./partials/form/searchButton.tpl" %}
</form>
{% endblock %}