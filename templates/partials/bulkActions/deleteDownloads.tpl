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
