<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
</div>
<div x-cloak class="sticky top-0 z-50 flex pl-4 pb-2 lg:gap-4 gap-1 flex-wrap bulk-editors items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <form class="px-4" method="post" :action="'/v1/groups/addTags?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/groups/removeTags?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/groups/addMeta?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/groups/meta/keys' jsonOutput="true" id=getNextId("freeField") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form
            class="px-4 no-ajax"
            method="post"
            :action="'/v1/groups/delete?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            x-data="confirmGroupDelete"
            x-bind="events"
            data-testid="bulk-delete-groups-form"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Delete Selected</span>
            {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
        </div>
    </form>
    <div class="px-4">
        <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Export</span>
        <button type="button"
                @click="window.location.href = '/admin/export?groups=' + [...$store.bulkSelection.selectedIds].join(',')"
                data-testid="bulk-export-selected"
                class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500">
            Export selected
        </button>
    </div>
    <div class="px-4" x-show="[...$store.bulkSelection.selectedIds].length === 2">
        <a :href="'/group/compare?g1=' + [...$store.bulkSelection.selectedIds][0] + '&g2=' + [...$store.bulkSelection.selectedIds][1]"
           class="inline-flex justify-center py-2 px-4 mt-3 border border-transparent items-center shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            Compare
        </a>
    </div>
    {% include "partials/pluginActionsBulk.tpl" with entityType="group" %}
</div>
