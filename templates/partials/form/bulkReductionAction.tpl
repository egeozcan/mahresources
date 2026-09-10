{# The bulk-bar handoff into a Resource Reduction.                              #}
{#                                                                              #}
{# Buttons rather than a <form>: bulkSelectionForms intercepts every form in    #}
{# the bar, POSTs it and morphs the list back in place, which would leave the   #}
{# reviewer on the list they just left rather than on the Reduction. The        #}
{# component issues its own request and navigates.                              #}
{#                                                                              #}
{# Parameters:                                                                  #}
{#   entity   — 'resource' or 'group'; which half of the Extent this fills      #}
{#   noun     — the plural noun for the hint, e.g. "Resources"                  #}
{#   panelId  — unique id for the disclosure panel                              #}
{#   reductionOwnerId — optional group detail action with a subtree checkbox    #}
<div class="{% if reductionOwnerId %}relative{% else %}px-4{% endif %}" x-data="reductionBulkAction({ entity: '{{ entity }}', ownerId: {{ reductionOwnerId|default:0 }} })"
     @keydown.escape.stop="open = false; $refs.trigger.focus()" @click.outside="open = false">
    {% if !reductionOwnerId %}
    <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Reduce</span>
    {% endif %}
    <button type="button" x-ref="trigger"
            data-testid="bulk-reduction-action"
            :aria-expanded="open ? 'true' : 'false'"
            aria-controls="{{ panelId }}"
            @click="toggle()"
            class="{% if reductionOwnerId %}inline-flex justify-center py-1 px-2 border border-stone-300 text-xs font-mono font-semibold tracking-wide rounded text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 transition-colors duration-100{% else %}inline-flex justify-center py-1.5 px-3 mt-3 border border-transparent items-center shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600{% endif %}">
        {% if reductionOwnerId %}Reduce{% else %}Resource Reduction{% endif %}
    </button>

    <div id="{{ panelId }}" x-show="open" x-cloak x-collapse class="{% if reductionOwnerId %}absolute right-0 z-20 {% endif %}mt-2 p-3 bg-white border border-stone-200 rounded-md shadow-sm w-72">
        {% if reductionOwnerId %}
        <label class="flex items-center gap-2 mb-2 text-sm text-stone-700">
            <input type="checkbox" x-model="includeDescendants" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
            Include resources from all subgroups
        </label>
        <p class="mb-2 text-xs text-stone-600" x-text="includeDescendants ? 'Includes resources owned by this group and all subgroups.' : 'Includes resources owned directly by this group.'"></p>
        {% endif %}
        <fieldset>
            <legend class="sr-only">Add {{ noun }} to a Resource Reduction</legend>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="radio" x-model="mode" value="new" class="border-stone-300 text-amber-700 focus:ring-amber-600">
                New Resource Reduction
            </label>
            <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                <input type="radio" x-model="mode" value="existing" class="border-stone-300 text-amber-700 focus:ring-amber-600">
                Add to existing reduction
            </label>
        </fieldset>

        <div x-show="mode === 'new'" class="mt-2">
            <label class="block text-sm font-medium font-mono text-stone-700" for="{{ panelId }}-name">Name</label>
            <input id="{{ panelId }}-name" type="text" x-model="name" placeholder="Resource Reduction"
                   data-testid="bulk-reduction-name"
                   class="mt-1 block w-full rounded-md border-stone-300 shadow-sm text-sm focus:border-amber-600 focus:ring-amber-600">
            <label class="flex items-center gap-2 mt-2 text-sm text-stone-700">
                <input type="checkbox" x-model="excludeExternalResources"
                       data-testid="bulk-reduction-exclude-external"
                       aria-describedby="{{ panelId }}-external-hint"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Exclude external resources
            </label>
            <p id="{{ panelId }}-external-hint" class="mt-1 text-xs text-stone-600">
                Only compare resources within the Extent.
            </p>
        </div>

        <div x-show="mode === 'existing'" class="mt-2">
            <label class="block text-sm font-medium font-mono text-stone-700" for="{{ panelId }}-existing">Resource Reduction</label>
            <select id="{{ panelId }}-existing" x-model="existingId"
                    data-testid="bulk-reduction-existing"
                    class="mt-1 block w-full rounded-md border-stone-300 shadow-sm text-sm focus:border-amber-600 focus:ring-amber-600">
                <template x-for="reduction in existing" :key="reduction.id">
                    <option :value="reduction.id" x-text="optionLabel(reduction)"></option>
                </template>
            </select>
            <p x-show="loadedExisting && existing.length === 0" x-cloak class="mt-1 text-xs text-stone-600">
                No Resource Reductions yet.
            </p>
        </div>

        {% if !reductionOwnerId %}
        <p class="mt-2 text-xs text-stone-600">
            <span x-text="selectedIds().length"></span> {{ noun }} selected.
        </p>
        {% endif %}
        <p x-show="error" x-cloak x-text="error" data-testid="bulk-reduction-error" class="mt-1 text-xs text-red-700" role="alert"></p>

        <button type="button"
                data-testid="bulk-reduction-submit"
                :disabled="busy || !hasSelection()"
                @click="submit()"
                class="mt-2 inline-flex justify-center py-1.5 px-3 border border-transparent items-center shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
            <span x-text="mode === 'existing' ? 'Add' : 'Create'"></span>
        </button>
    </div>
</div>
