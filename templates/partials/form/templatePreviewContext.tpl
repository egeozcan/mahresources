{# The entity every slot preview renders against. tpEntityType, tpCarrierId, tpGeneratePath come from the enclosing with; tpCarrierId is null on the create form. #}
<div class="rounded-md border border-stone-300 bg-stone-50 p-3 space-y-2" data-template-preview-context
     x-data="templatePreviewContext({ entityType: '{{ tpEntityType }}', generatePath: '{{ tpGeneratePath }}', categoryId: {{ tpCarrierId|default:"null" }} })">
    <div class="flex flex-wrap items-end gap-3">
        <div class="relative">
            <label for="tp-entity-{{ tpEntityType }}" class="block text-xs font-mono text-stone-700 mb-0.5">Preview against</label>
            <input id="tp-entity-{{ tpEntityType }}" type="text"
                   x-model="query" @input="onSearchInput()" @focus="open = suggestions.length > 0"
                   autocomplete="off" role="combobox" aria-autocomplete="list" aria-controls="tp-suggestions-{{ tpEntityType }}"
                   aria-describedby="tp-entity-hint-{{ tpEntityType }}"
                   :aria-expanded="open ? 'true' : 'false'"
                   class="w-56 text-sm rounded border border-stone-300 px-2 py-1 focus:outline-hidden focus:ring-2 focus:ring-amber-600"
                   placeholder="Search {{ tpEntityType }}…">
            <ul x-show="open" @click.away="open = false" x-transition
                id="tp-suggestions-{{ tpEntityType }}" role="listbox"
                class="absolute z-20 mt-1 w-56 max-h-52 overflow-auto bg-white border border-stone-300 rounded shadow-md text-sm">
                <template x-for="s in suggestions" :key="s.id">
                    <li role="option" tabindex="0"
                        @click="pick(s)" @keydown.enter.prevent="pick(s)"
                        class="px-2 py-1 cursor-pointer hover:bg-amber-50 focus:bg-amber-50 focus:outline-hidden">
                        <span x-text="s.name"></span>
                        <span class="text-stone-600" x-text="'#' + s.id"></span>
                    </li>
                </template>
            </ul>
        </div>
        <button type="button" @click="refreshAll()"
                class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-white border border-stone-300 rounded hover:bg-stone-100 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">
            Refresh all previews
        </button>
    </div>
    <p id="tp-entity-hint-{{ tpEntityType }}" class="text-xs text-stone-600">
        Each template below ends with a live preview against this {{ tpEntityType }}. List header and footer previews render against this category itself. Previews run in an isolated sandbox, so interactive editors and API-backed widgets do not work in them.
    </p>
    {# WS6 finding 29: when the category has no members, the preview borrows an entity from outside it. Say so: the sample does not carry this category's meta. #}
    <p x-show="usingUnscopedSample" class="text-xs text-amber-900 font-mono px-2 py-1 bg-amber-50">
        This category has no entities yet, so the previews use one from elsewhere. It does not carry this category's metadata.
    </p>
</div>
