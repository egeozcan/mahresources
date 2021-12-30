{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <div class="sticky top-0 flex pl-4 gap-4 flex-wrap  bg-white items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data>
        {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
        <form class="mb-6 p-4" method="post" :action="'/v1/resources/addTags?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id="tag_autocompleter"|nanoid %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
        <form class="mb-6 p-4" method="post" :action="'/v1/resources/removeTags?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id="tag_autocompleter"|nanoid %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
            </div>
        </form>
        <form class="mb-6 p-4" method="post" :action="'/v1/resources/addMeta?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' jsonOutput="true" id="freeField"|nanoid %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
        <form class="mb-6 p-4" method="post" :action="'/v1/resources/addGroups?redirect=' + encodeURIComponent(window.location)">
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex gap-2 items-start">
                {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='editedId' title='Add Groups' id="autocompleter"|nanoid extraInfo="Category" %}
                <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
            </div>
        </form>
        <form
            class="mb-6 p-4"
            method="post"
            :action="'/v1/resources/delete?redirect=' + encodeURIComponent(window.location)"
            x-data="confirmAction('Are you sure you want to delete the selected resources?')"
            x-bind="events"
        >
            {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
            <div class="flex flex-col">
                <span class="block text-sm font-medium text-gray-700 mt-3">Delete Selected</span>
                {% include "/partials/form/searchButton.tpl" with text="Delete" %}
            </div>
        </form>
    </div>
    <section class="note-container">
        {% for entity in resources %}
            {% include "/partials/resource.tpl" with selectable=true %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
<form class="flex gap-2 items-start flex-col">
    {% include "/partials/sideTitle.tpl" with title="Sort" %}
    {% include "/partials/form/selectInput.tpl" with name='SortBy' label='Sort' values=sortValues %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
    {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
    {% include "/partials/form/textInput.tpl" with name='OriginalName' label='Original Name' value=queryValues.OriginalName.0 %}
    {% include "/partials/form/textInput.tpl" with name='Hash' label='Hash' value=queryValues.Hash.0 %}
    {% include "/partials/form/textInput.tpl" with name='OriginalLocation' label='Original Location' value=queryValues.OriginalLocation.0 %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id="autocompleter"|nanoid %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id="autocompleter"|nanoid %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id="autocompleter"|nanoid extraInfo="Category" %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owner id="autocompleter"|nanoid extraInfo="Category" %}
    {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/resources/meta/keys' fields=parsedQuery.MetaQuery id="freeField"|nanoid %}
    {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
    {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
    {% include "/partials/form/searchButton.tpl" %}
</form>
{% endblock %}