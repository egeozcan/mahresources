<fieldset class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-3 gap-y-2" :disabled="$store.entityPicker.stepFor(view.id)?.status === 'confirming'">
    <legend class="sr-only">Filter results</legend>
    <template x-for="filter in $store.entityPicker.filtersFor(view.entity)" :key="filter.key">
        <div x-show="filter.section === 'basic' || view.more" :class="filter.kind === 'metadata' && 'sm:col-span-2 lg:col-span-3'">
            <template x-if="filter.kind === 'entity'">
                <div :data-selector-title="filter.label" x-id="['picker-filter', 'picker-options']"
                     x-data="dynamicEntitySelector({ entity: filter.entity, searchUrl: filter.endpoint, multiple: filter.multiple,
                         onChange: change => $store.entityPicker.applyFilterChange(filter.key, filter.multiple, change, view.id) })">
                    <label :for="$id('picker-filter')" class="block text-xs font-medium text-stone-700" x-text="filter.label"></label>
                    <div class="flex items-center gap-1 relative">
                        <input type="text" x-ref="autocompleter" x-bind="inputEvents" :id="$id('picker-filter')"
                               class="w-full min-w-0 text-sm border-stone-300 rounded mt-1" autocomplete="off"
                               role="combobox" aria-autocomplete="list" :aria-expanded="dropdownActive && results.length > 0"
                               :aria-controls="$id('picker-options')" :aria-activedescendant="selectedIndex >= 0 ? $id('picker-options') + '-' + selectedIndex : null">
                        {% include "/partials/form/entityBrowseButton.tpl" %}
                        <div popover="manual" x-ref="dropdown" :id="$id('picker-options')" role="listbox"
                             :aria-label="filter.label + ' suggestions'" class="selector-popover bg-white border border-stone-300 rounded shadow-lg max-h-60 overflow-auto p-1">
                            <template x-for="(result, index) in results" :key="result.ID">
                                <div role="option" :id="$id('picker-options') + '-' + index" :aria-selected="selectedIndex === index"
                                     class="p-2 text-sm cursor-pointer" :class="selectedIndex === index && 'bg-amber-100'"
                                     @mouseover="setActiveIndex(index)" @mousedown.prevent="startSelecting(); selectResult(result)" x-text="result.Name"></div>
                            </template>
                        </div>
                    </div>
                    <div class="flex flex-wrap gap-1 mt-1">
                        <template x-for="result in selectedResults" :key="result.ID">
                            <button type="button" class="text-xs bg-stone-100 rounded px-2 py-1" @click="removeItem(result)"
                                    :aria-label="'Remove ' + result.Name"><span x-text="result.Name"></span><span aria-hidden="true"> ×</span></button>
                        </template>
                    </div>
                </div>
            </template>
            <template x-if="filter.kind !== 'entity' && filter.kind !== 'metadata'">
                <div>
                    <label :for="'picker-' + view.id + '-' + filter.key" class="block text-xs font-medium text-stone-700" x-text="filter.label"></label>
                    <template x-if="filter.kind === 'checkbox'">
                        <input type="checkbox" :id="'picker-' + view.id + '-' + filter.key" :checked="Boolean(view.filters[filter.key])"
                               @change="$store.entityPicker.setFilter(filter.key, $event.target.checked, view.id)" class="rounded text-amber-700">
                    </template>
                    <template x-if="filter.kind !== 'checkbox'">
                        <input :type="filter.kind" :id="'picker-' + view.id + '-' + filter.key" :name="filter.key"
                               :value="view.filters[filter.key] ?? ''" :placeholder="filter.key === 'Name' ? 'Search by name...' : ''"
                               @input="$store.entityPicker.setFilter(filter.key, $event.target.value, view.id)"
                               class="w-full text-sm border-stone-300 rounded mt-1">
                    </template>
                </div>
            </template>
            <template x-if="filter.kind === 'metadata'">
                <div role="group" aria-label="Metadata filters" class="space-y-2">
                    <p class="text-xs font-medium text-stone-700">Metadata</p>
                    <template x-for="(field, index) in view.metaFields" :key="index">
                        <div class="flex flex-wrap gap-2">
                            <input type="text" x-model="field.name" :aria-label="'Metadata field ' + (index + 1) + ' name'" placeholder="Field name"
                                   @input="$nextTick(() => $store.entityPicker.applyMetaFilter(view.id))" class="min-w-0 flex-1 text-sm rounded border-stone-300">
                            <select x-model="field.operation" :aria-label="'Metadata field ' + (index + 1) + ' comparison operator'"
                                    @change="$nextTick(() => $store.entityPicker.applyMetaFilter(view.id))" class="text-sm rounded border-stone-300">
                                <option value="EQ">Equals</option><option value="LI">Contains</option><option value="NE">Not equal</option><option value="NL">Does not contain</option>
                                <option value="GT">Greater than</option><option value="GE">Greater or equal</option><option value="LT">Less than</option><option value="LE">Less or equal</option>
                            </select>
                            <input type="text" x-model="field.value" :aria-label="'Metadata field ' + (index + 1) + ' value'" placeholder="Value"
                                   @input="$nextTick(() => $store.entityPicker.applyMetaFilter(view.id))" class="min-w-0 flex-1 text-sm rounded border-stone-300">
                            <button type="button" @click="view.metaFields.splice(index, 1); $store.entityPicker.applyMetaFilter(view.id)"
                                    :aria-label="'Remove metadata field ' + (index + 1)" class="text-sm underline">Remove</button>
                        </div>
                    </template>
                    <button type="button" @click="view.metaFields.push({name:'',operation:'EQ',value:''})" class="text-sm underline text-amber-800">Add metadata filter</button>
                </div>
            </template>
        </div>
    </template>
</fieldset>
<button type="button" @click="view.more = !view.more" :aria-expanded="view.more" class="text-sm text-amber-800 underline mt-3"
        x-text="view.more ? 'Fewer filters' : 'More filters'"></button>
