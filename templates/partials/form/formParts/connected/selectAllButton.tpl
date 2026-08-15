{# WS6, finding 68: on an empty list the predicate below is 0 + 1 !== 0, which  #}
{# is true, so "Select All" was offered with nothing behind it. The emptiness   #}
{# guard has to come first.                                                     #}
{#                                                                              #}
{# It asks hasSelectableItems() rather than `elements.length > 0` because the   #}
{# registry is still empty when Alpine first evaluates this — the rows register #}
{# from their own init() and they come after this row in the DOM. Reading the   #}
{# registry made the first frame disagree with the settled one, so x-collapse   #}
{# animated the row open on load and shoved the list and the footer's           #}
{# pagination down 37px. See the method for the measurement.                    #}
<div x-data x-show="$store.bulkSelection.hasSelectableItems() && [...$store.bulkSelection.selectedIds].length + 1 !== $store.bulkSelection.elements.length" x-collapse>
    <button type="button"
        data-bulk-select-all
        @click.prevent="$store.bulkSelection.selectAll()"
        class="
            inline-flex justify-center
            py-2 px-4 mt-3
            border border-transparent
            items-center
            shadow-sm text-xs font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600"
    >
        {% if text %}{{ text }}{% else %}Select All{% endif %}
    </button>
</div>