<div x-data="bulkSelectionForms">
    <form x-cloak x-show="$selection.isActiveEditor($el)" x-collapse :class="$selection.isActiveEditor($el) && 'active'"
        class="px-4"
        method="post"
        :action="'/v1/tags/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
        {# Finding 153: the toolbar was blamed for "reusing the generic delete      #}
        {# confirm". It never did — it authored "Selected tags will be merged. Are #}
        {# you sure?" and confirmAction discarded it. Now it names the count and    #}
        {# the winner.                                                             #}
        {#                                                                         #}
        {# requireSelection is finding 16/92's guard on the surface that fix missed: #}
        {# this form has no fixed winner, so Merge with the picker empty confirmed  #}
        {# and then answered 400. It is also what makes {winner} always resolvable. #}
        x-data="confirmAction({
            message: 'Merge {count} tag{s} into {winner}? The merged tag{s} will be deleted.',
            requireSelection: { field: 'winner' }
        })"
        x-bind="events"
    >
        <template x-for="(id, i) in [...$selection.selectedIds]">
            <input type="hidden" name="losers" :value="id">
        </template>
        <div class="flex gap-2 items-start">
            {# Finding 94's other surface: the winner picker offered the rows that are    #}
            {# ticked as losers, which is the same impossible merge from the other side. #}
            {# The exclusion is read per search because the ticked set changes while    #}
            {# the field is on screen.                                                 #}
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='tag' max=1 elName='winner' title='Merge Winner' excludeIds='...$selection.selectedIds' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Merge" disabledWhen="!hasSelection" describedBy="bulk-merge-tag-hint" %}</div>
        </div>
        <p x-show="!hasSelection" x-cloak id="bulk-merge-tag-hint" class="mt-2 text-xs text-stone-600">Choose the tag to merge the selected ones into.</p>
    </form>

</div>
