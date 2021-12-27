{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="sticky top-0 flex -ml-2 gap-4 flex-wrap  bg-white" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data>
        <form class="mb-6 p-4" method="post" :action="'/v1/groups/addTags?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id="tag_autocompleter"|nanoid %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
        <form class="mb-6 p-4" method="post" :action="'/v1/groups/removeTags?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id="tag_autocompleter"|nanoid %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
            </div>
        </form>
        <form class="mb-6 p-4" method="post" :action="'/v1/groups/addMeta?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/groups/meta/keys' jsonOutput="true" id="freeField"|nanoid %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
    </div>
    <div class="flex flex-col gap-4">
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
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/textInput.tpl" with name='URL' label='URL' value=queryValues.URL.0 %}

        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id="autocompleter"|nanoid %}
        {% include "/partials/form/checkboxInput.tpl" with name='SearchParentsForTags' label='Search Parents For Tags' value=queryValues.SearchParentsForTags.0 id="SearchParentsForTags"|nanoid %}
        {% include "/partials/form/checkboxInput.tpl" with name='SearchChildrenForTags' label='Search Children For Tags' value=queryValues.SearchChildrenForTags.0 id="SearchChildrenForTags"|nanoid %}

        {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='categories' title='Categories' selectedItems=categories id="autocompleter"|nanoid %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id="autocompleter"|nanoid %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/resources' elName='resources' title='Resources' selectedItems=resources id="autocompleter"|nanoid %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groupsSelection id="autocompleter"|nanoid extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owners id="autocompleter"|nanoid extraInfo="Category" %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/groups/meta/keys' fields=parsedQuery.MetaQuery id="freeField"|nanoid %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}