<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
</div>
<div x-cloak class="sticky top-0 z-30 flex pl-4 pb-2 lg:gap-4 gap-1 flex-wrap bulk-editors items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <form
        class="px-4"
        method="post"
        :action="'/v1/tags/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
        x-data="confirmAction('Selected tags will be merged. Are you sure?')"
        x-bind="events"
    >
        <template x-for="(id, i) in [...$store.bulkSelection.selectedIds]">
            <input type="hidden" name="losers" :value="id">
        </template>
        <div class="flex gap-2 items-start">
            {# Finding 94's other surface: the winner picker offered the rows that are    #}
            {# ticked as losers, which is the same impossible merge from the other side. #}
            {# The exclusion is read per search because the ticked set changes while    #}
            {# the field is on screen.                                                 #}
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='tag' max=1 elName='winner' title='Merge Winner' excludeIds='...$store.bulkSelection.selectedIds' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
        </div>
    </form>
    <form
        class="px-4 no-ajax"
        method="post"
        :action="'/v1/tags/delete?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
        x-data="confirmAction('Are you sure you want to delete the selected tags?')"
        x-bind="events"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Delete Selected</span>
            {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
        </div>
    </form>
</div>