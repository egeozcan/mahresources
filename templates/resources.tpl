{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for resource in resources %}
        {% include "./partials/resource.tpl" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
<h3 class="font-regular text-base md:text-lg leading-snug truncate">Filter</h3>
<form class="mt-5">
    {% include "./partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags.SelectedRelations id="autocompleter"|nanoid %}
    {% include "./partials/form/autocompleter.tpl" with url='/v1/albums' elName='albums' title='Albums' selectedItems=albums.SelectedRelations id="autocompleter"|nanoid %}
    {% include "./partials/form/autocompleter.tpl" with url='/v1/people/autocomplete' elName='people' title='People' selectedItems=people.SelectedRelations id="autocompleter"|nanoid %}
    {% include "./partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
    {% include "./partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
    {% include "./partials/form/searchButton.tpl" %}
</form>
{% endblock %}