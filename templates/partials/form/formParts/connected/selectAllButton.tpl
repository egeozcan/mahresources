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
{#                                                                              #}
{# The two terms sit on two elements so that only the second one animates.     #}
{# Where the rows arrive after the page does, as the MRQL page's results do,   #}
{# the first term flips when they land, and animating that grew the row in     #}
{# over the cards just drawn beneath it. The row now appears with its rows,    #}
{# and only a change of selection collapses or expands it.                     #}
<div x-data x-show="$selection.hasSelectableItems()">
    <div x-show="$selection.selectedIds.size === 0 || $selection.selectedIds.size !== Object.keys($selection.options).length" x-collapse>
    <button type="button"
        data-bulk-select-all
        @click.prevent="$selection.selectAll()"
        class="
            inline-flex justify-center
            py-2 px-4 mt-3
            border border-transparent
            items-center
            shadow-sm text-xs font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600"
    >
        {% if text %}{{ text }}{% else %}Select All{% endif %}
    </button>
    </div>
</div>