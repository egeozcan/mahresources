{# The /downloads bulk toolbar. Same shell as bulkEditorTag/Note/Resource: the    #}
{# empty-state Select All above the list, and a sticky `bulk-editors` bar that     #}
{# collapses in once something is selected, both driven by the shared              #}
{# `bulkSelection` store.                                                          #}
{#                                                                                 #}
{# No forms and no `bulkSelectionForms`, deliberately. That component registers    #}
{# every <form> beneath it as a *disclosure editor* — a toggle button that opens a #}
{# panel of fields — which is right for "Add Tag" and wrong for a one-click Retry. #}
{# These follow the other one-click action in the family, bulkEditorGroup's        #}
{# "Export selected": a plain button in a `px-4` block.                            #}
{#                                                                                 #}
{# The actions themselves live in the `downloads` store, because this toolbar is   #}
{# in the page's `prebody` block and the cards it acts on are in `body`.           #}
<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
</div>
<div x-cloak class="sticky top-0 z-30 flex pl-4 pb-2 lg:gap-4 gap-1 flex-wrap bulk-editors items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data>
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <div class="px-2">
        <span class="block text-sm font-mono font-medium text-stone-700 mt-3"
              data-testid="downloads-selected-count"
              x-text="[...$store.bulkSelection.selectedIds].length + ' selected'"></span>
    </div>
    {# order-2 so the selection controls (the .bulk-editors rule puts bare      #}
    {# buttons at order 1) stay left of the actions, as they do on every other   #}
    {# list page, where the editor forms carry order 2.                          #}
    <div class="px-4 order-2">
        <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Retry Selected</span>
        <button type="button"
                @click="$store.downloads.retrySelected()"
                :aria-disabled="$store.downloads.busy"
                aria-label="Retry selected downloads"
                data-testid="downloads-bulk-retry"
                class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500 aria-disabled:opacity-50 aria-disabled:cursor-not-allowed">
            Retry
        </button>
    </div>
    <div class="px-4 order-2">
        <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Delete Selected</span>
        {# The confirm is raised by the store rather than by `confirmAction`: this is #}
        {# a button, not a form submit, so there is no submit event to intercept.     #}
        {# The accessible name says "selected": the visible heading beside it is a    #}
        {# sibling <span> and names nothing, so to a reader listing buttons this was  #}
        {# indistinguishable from a card's own Delete.                                #}
        <button type="button"
                @click="$store.downloads.removeSelected()"
                :aria-disabled="$store.downloads.busy"
                aria-label="Delete selected downloads"
                data-testid="downloads-bulk-delete"
                class="inline-flex justify-center py-1.5 px-3 mt-3 border border-transparent items-center text-sm font-medium rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600 aria-disabled:opacity-50 aria-disabled:cursor-not-allowed">
            Delete
        </button>
    </div>
</div>
