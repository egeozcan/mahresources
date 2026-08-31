<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <button
            type="button"
            class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500"
            @click="$dispatch('mass-edit-open', { entityType: 'resource', target: 'filter' })"
    >
        Mass edit all {{ totalCount|default:0 }} results
    </button>
</div>
<div class="sticky top-0 z-30 flex pl-4 pb-2 gap-4 lg:gap-4 gap-1 flex-wrap bulk-editors items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <button
            type="button"
            class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500"
            @click="$dispatch('mass-edit-open', { entityType: 'resource', target: 'ids' })"
    >
        Mass Edit Selected
    </button>
    <form class="px-4" method="post" :action="'/v1/resources/addTags?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with profile='tag' usage='resource' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resources/removeTags?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='tag' usage='resource' elName='editedId' title='Remove Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resources/addMeta?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' jsonOutput="true" id=getNextId("freeField") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resources/addGroups?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='group' categoryDecoration=true elName='editedId' title='Add Groups' id=getNextId("autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/resource/recalculateDimensions?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Update Dimensions</span>
            {% include "/partials/form/searchButton.tpl" with text="Update Dimensions" %}
        </div>
    </form>
    {% include "/partials/form/bulkCompareAction.tpl" with comparePath="/resource/compare" p1="r1" p2="r2" noun="resources" hintId="bulk-compare-resources-hint" %}
    {% include "/partials/form/bulkReductionAction.tpl" with entity="resource" noun="Resources" panelId="bulk-reduction-resources" %}
    <form
            class="px-4 no-ajax"
            method="post"
            :action="'/v1/resources/delete?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            {# Finding 78: this message was written and never shown — confirmAction #}
            {# destructured the string and fell back to its default, so one row and #}
            {# a full Select All both asked "Are you sure you want to delete?".      #}
            x-data="confirmAction('Delete {count} resource{s}? This cannot be undone.')"
            x-bind="events"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Delete Selected</span>
            {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
        </div>
    </form>
    {% include "partials/pluginActionsBulk.tpl" with entityType="resource" %}
</div>