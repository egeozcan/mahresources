{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorGroup.tpl" %}
{% endblock %}

{% block body %}
    <div class="flex flex-col gap-4 items-container">
        {% for entity in groups %}
            {% include "/partials/group.tpl" with selectable=true %}
        {% endfor %}
    </div>
{% endblock %}


{% block sidebar %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/sideTitle.tpl" with title="Sort" %}
        {% include "/partials/form/selectInput.tpl" with name='SortBy' label='Sort' values=sortValues %}
        {% include "/partials/sideTitle.tpl" with title="Filter" %}

        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/checkboxInput.tpl" with name='SearchParentsForName' label='Search Parents For Name' value=queryValues.SearchParentsForName.0 id=getNextId("SearchParentsForName") %}
        {% include "/partials/form/checkboxInput.tpl" with name='SearchChildrenForName' label='Search Children For Name' value=queryValues.SearchChildrenForName.0 id=getNextId("SearchChildrenForName") %}

        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/textInput.tpl" with name='URL' label='URL' value=queryValues.URL.0 %}

        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
        {% include "/partials/form/checkboxInput.tpl" with name='SearchParentsForTags' label='Search Parents For Tags' value=queryValues.SearchParentsForTags.0 id=getNextId("SearchParentsForTags") %}
        {% include "/partials/form/checkboxInput.tpl" with name='SearchChildrenForTags' label='Search Children For Tags' value=queryValues.SearchChildrenForTags.0 id=getNextId("SearchChildrenForTags") %}

        {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='categories' title='Categories' selectedItems=categories id=getNextId("autocompleter") %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/resources' elName='resources' title='Resources' selectedItems=resources id=getNextId("autocompleter") %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groupsSelection id=getNextId("autocompleter") extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owners id=getNextId("autocompleter") extraInfo="Category" %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/groups/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}