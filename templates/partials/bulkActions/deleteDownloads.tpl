    <div class="bulk-action-item">
        {# The confirm is raised by the store rather than by `confirmAction`: this is #}
        {# a button, not a form submit, so there is no submit event to intercept.     #}
        {# The accessible name says "selected": the visible heading beside it is a    #}
        {# sibling <span> and names nothing, so to a reader listing buttons this was  #}
        {# indistinguishable from a card's own Delete.                                #}
        <button type="button"
                @click="$store.downloads.removeSelected($selection)"
                :aria-disabled="$store.downloads.busy"
                aria-label="Delete selected downloads"
                data-testid="downloads-bulk-delete"
                class="bulk-action-btn bulk-action-btn--danger">
            Delete
        </button>
    </div>
