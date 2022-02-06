{% extends "/layouts/base.tpl" %}

{% block prebody %}
    <div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
        {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    </div>
    <div class="sticky top-0 flex pl-4 pb-2 lg:gap-4 gap-1 flex-wrap  bg-white items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
        {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
        {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
        <form class="px-4" method="post" :action="'/v1/groups/addTags?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
        <form class="px-4" method="post" :action="'/v1/groups/removeTags?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id=getNextId("tag_autocompleter") %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
            </div>
        </form>
        <form class="px-4" method="post" :action="'/v1/groups/addMeta?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/groups/meta/keys' jsonOutput="true" id=getNextId("freeField") %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
        <form
                class="px-4 no-ajax"
                method="post"
                :action="'/v1/groups/delete?redirect=' + encodeURIComponent(window.location)"
                x-data="confirmAction('Are you sure you want to delete the selected groups?')"
                x-bind="events"
        >
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex flex-col">
                <span class="block text-sm font-medium text-gray-700 mt-3">Delete Selected</span>
                {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
            </div>
        </form>
    </div>
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