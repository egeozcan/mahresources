{% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
<div class="sticky top-0 flex pl-4 gap-4 flex-wrap  bg-white items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data>
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    <form class="mb-6 p-4" method="post" :action="'/v1/resources/addTags?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="mb-6 p-4" method="post" :action="'/v1/resources/removeTags?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
        </div>
    </form>
    <form class="mb-6 p-4" method="post" :action="'/v1/resources/addMeta?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' jsonOutput="true" id=getNextId("freeField") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="mb-6 p-4" method="post" :action="'/v1/resources/addGroups?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='editedId' title='Add Groups' id=getNextId("autocompleter") extraInfo="Category" %}
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