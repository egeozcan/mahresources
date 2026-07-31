{# Finding 131: Compare was `x-show`n at exactly two selections, so ticking a    #}
{# third made it vanish with no explanation — measured at 92.8x38 px with two    #}
{# ticked and 0x0 with three, still carrying the href for the first two. A       #}
{# control that disappears teaches nothing; one that is visibly unavailable and  #}
{# says why does.                                                                #}
{#                                                                              #}
{# It is a <button>, not an <a>, because a link has no disabled state: an        #}
{# `aria-disabled` anchor is still followed on Enter and still in the tab order  #}
{# leading nowhere. The button navigates when it can and is genuinely disabled   #}
{# when it cannot, and aria-describedby points at the hint that says why.        #}
<div class="px-4" x-data>
    <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Compare</span>
    <button type="button"
            data-testid="bulk-compare-action"
            :disabled="[...$store.bulkSelection.selectedIds].length !== 2"
            :aria-describedby="[...$store.bulkSelection.selectedIds].length !== 2 ? '{{ hintId }}' : null"
            @click="window.location.href = '{{ comparePath }}?{{ p1 }}=' + [...$store.bulkSelection.selectedIds][0] + '&{{ p2 }}=' + [...$store.bulkSelection.selectedIds][1]"
            class="inline-flex justify-center py-1.5 px-3 mt-3 border border-transparent items-center shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
        Compare
    </button>
    <p id="{{ hintId }}"
       x-show="[...$store.bulkSelection.selectedIds].length !== 2"
       x-cloak
       class="mt-1 text-xs text-stone-600">Select exactly two {{ noun }} to compare.</p>
</div>
