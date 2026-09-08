{% with actions=bulkActions(bulkEntity, pluginBulkActions) %}
<div x-data data-bulk-entity="{{ bulkEntity }}">
    <div class="pb-3" x-show="$selection.selectedIds.size === 0" x-collapse>
        {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
        {% for action in actions %}{% if action.ID == 'mass-edit' %}
        <button type="button" class="bulk-action-btn mt-3 px-3 py-1.5 border rounded-md"
                @click="$dispatch('mass-edit-open', { entityType: '{{ bulkEntity }}', target: 'filter', selection: $selection })">Mass edit all {% if not mrqlLists %}{{ totalCount|default:0 }} {% endif %}results</button>
        {% endif %}{% endfor %}
    </div>
    <div class="sticky top-0 z-30 flex pb-2 gap-2 flex-wrap bulk-editors items-start" x-show="$selection.selectedIds.size > 0" x-cloak x-collapse>
        {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
        {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
        <span data-testid="bulk-selected-count" class="text-sm mt-3" x-text="$selection.selectedIds.size + ' {{ bulkEntity }}' + ($selection.selectedIds.size === 1 ? '' : 's') + ' selected'"></span>
        {% for action in actions %}
        <div x-data='bulkAction({{ action|json }})' class="px-2" data-bulk-action="{{ action.ID }}" x-id="['bulk-action-reason']">
            <fieldset :disabled="!!unavailableReason()" :aria-describedby="unavailableReason() ? $id('bulk-action-reason') : null">
            {% if action.Component %}
                {% include action.Component %}
            {% else %}
                {% include "/partials/bulkActions/form.tpl" %}
            {% endif %}
            </fieldset>
            <p :id="$id('bulk-action-reason')" x-show="unavailableReason()" x-text="unavailableReason()" class="text-xs text-stone-600" role="status"></p>
        </div>
        {% endfor %}
    </div>
</div>
{% endwith %}
