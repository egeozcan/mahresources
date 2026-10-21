{% with actions=bulkActions(bulkEntity, pluginBulkActions) %}
<div x-data="{ editorsShown: false }" x-init="$selection.noun = '{{ bulkEntity|escapejs }}'" x-effect="if ($selection.selectedIds.size > 0) editorsShown = true">
    <div class="pb-3" x-show="$selection.selectedIds.size === 0" x-collapse>
        {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
        {# MRQL can render resources, notes, and groups in separate result buckets, #}
        {# so it has no single page-level entity action to move into the title bar. #}
        {% if mrqlLists %}{% for action in actions %}{% if action.ID == 'mass-edit' %}
        <button type="button" class="bulk-action-btn mt-3 px-3 py-1.5 border rounded-md"
                @click="$dispatch('mass-edit-open', { entityType: '{{ bulkEntity }}', target: 'filter', selection: $selection })">Mass edit all results</button>
        {% endif %}{% endfor %}{% endif %}
    </div>
    <div class="sticky top-0 z-30 flex pb-2 gap-2 flex-wrap bulk-editors items-start" :hidden="$selection.selectedIds.size === 0" hidden x-cloak>
        {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
        {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
        <span data-testid="bulk-selected-count" class="text-sm mt-3" x-text="$selection.selectedIds.size + ' {{ bulkEntity }}' + ($selection.selectedIds.size === 1 ? '' : 's') + ' selected'"></span>
        {# The action editors are rendered on the first selection, not with the page: #}
        {# hidden, they were a few hundred bindings Alpine set up on every list page   #}
        {# load. Once rendered they stay, hidden with the bar, so a form keeps what was #}
        {# typed into it and a command's focus keeper outlives the selection it empties. #}
        <template x-if="editorsShown"><div class="bulk-editors-items contents">
        {% for action in actions %}
        <div x-data='bulkAction({{ action|json }})' class="bulk-action" data-bulk-action="{{ action.ID }}" x-id="['bulk-action-reason']">
            <fieldset :disabled="!!unavailableReason()" :aria-describedby="unavailableReason() ? $id('bulk-action-reason') : null">
            {% if action.Component %}
                {% include action.Component %}
            {% else %}
                {% include "/partials/bulkActions/form.tpl" %}
            {% endif %}
            </fieldset>
            <p :id="$id('bulk-action-reason')" x-show="unavailableReason()" x-text="unavailableReason()" class="bulk-action-reason text-xs text-stone-600" role="status"></p>
        </div>
        {% endfor %}
        </div></template>
    </div>
</div>
{% endwith %}
