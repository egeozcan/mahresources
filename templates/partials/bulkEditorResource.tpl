<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
</div>
<div class="sticky top-0 flex pl-4 pb-2 gap-4 lg:gap-4 gap-1 flex-wrap bulk-editors bg-white items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <form class="px-4" method="post" :action="'/v1/resources/addTags?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resources/removeTags?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resources/addMeta?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' jsonOutput="true" id=getNextId("freeField") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resources/addGroups?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='editedId' title='Add Groups' id=getNextId("autocompleter") extraInfo="Category" %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resource/recalculateDimensions?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-medium text-gray-700 mt-3">Update Dimensions</span>
            {% include "/partials/form/searchButton.tpl" with text="Update Dimensions" %}
        </div>
    </form>
    <div class="px-4" x-show="[...$store.bulkSelection.selectedIds].length === 2">
        <a :href="'/resource/compare?r1=' + [...$store.bulkSelection.selectedIds][0] + '&r2=' + [...$store.bulkSelection.selectedIds][1]"
           class="inline-flex justify-center py-2 px-4 mt-3 border border-transparent items-center shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            Compare
        </a>
    </div>
    <form
            class="px-4 no-ajax"
            method="post"
            :action="'/v1/resources/delete?redirect=' + encodeURIComponent(window.location)"
            x-data="confirmAction('Are you sure you want to delete the selected resources?')"
            x-bind="events"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-medium text-gray-700 mt-3">Delete Selected</span>
            {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
        </div>
    </form>
</div>