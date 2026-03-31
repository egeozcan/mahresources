<div
    x-data="schemaSearchFields({
        elName: '{{ elName }}',
        existingMetaQuery: {{ existingMetaQuery|json }} || [],
        initialCategories: {{ initialCategories|json }} || [],
        id: '{{ id }}',
    })"
    @multiple-input.window="if ($event.detail.name === '{{ elName }}') handleCategoryChange($event.detail.value)"
    class="w-full"
    role="group"
    aria-label="Schema fields"
>
    <span class="sr-only" aria-live="polite" aria-atomic="true"
          x-text="hasFields ? fields.length + ' schema filter fields available' : (fieldsCleared ? 'Schema filter fields cleared' : '')"></span>
    <template x-if="hasFields">
        <div class="flex flex-col gap-2 w-full">
            <template x-for="(field, fIdx) in fields" :key="field.path">
                <div class="w-full" :data-field-path="field.path">
                    <!-- Hidden inputs for form submission -->
                    <template x-for="(hidden, hIdx) in getHiddenInputs(field)" :key="fIdx + '-h-' + hIdx">
                        <input type="hidden" name="MetaQuery" :value="hidden.value">
                    </template>

                    <!-- Boolean: three-state radio -->
                    <template x-if="field.type === 'boolean'">
                        <fieldset class="w-full" :aria-label="field.label.replace(/ › /g, ', ')">
                            <legend
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                            ></legend>
                            <div class="flex gap-3 mt-1">
                                <label class="text-sm flex items-center gap-1">
                                    <input type="radio" :name="id + '-bool-' + field.path" value="any"
                                           x-model="field.boolValue">
                                    Any
                                </label>
                                <label class="text-sm flex items-center gap-1">
                                    <input type="radio" :name="id + '-bool-' + field.path" value="true"
                                           x-model="field.boolValue">
                                    Yes
                                </label>
                                <label class="text-sm flex items-center gap-1">
                                    <input type="radio" :name="id + '-bool-' + field.path" value="false"
                                           x-model="field.boolValue">
                                    No
                                </label>
                            </div>
                        </fieldset>
                    </template>

                    <!-- Enum ≤ 6: checkboxes -->
                    <template x-if="field.enum && field.enum.length <= 6">
                        <fieldset class="w-full" :aria-label="field.label.replace(/ › /g, ', ')">
                            <legend
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                            ></legend>
                            <div class="flex flex-wrap gap-x-3 gap-y-1 mt-1">
                                <template x-for="enumVal in field.enum" :key="enumVal">
                                    <label class="text-sm flex items-center gap-1">
                                        <input type="checkbox" :value="enumVal"
                                               x-model="field.enumValues">
                                        <span x-text="enumVal"></span>
                                    </label>
                                </template>
                            </div>
                        </fieldset>
                    </template>

                    <!-- Enum > 6: multi-select dropdown -->
                    <template x-if="field.enum && field.enum.length > 6">
                        <fieldset class="w-full" :aria-label="field.label.replace(/ › /g, ', ')">
                            <legend
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                            ></legend>
                            <select multiple
                                    x-model="field.enumValues"
                                    :id="id + '-enum-' + field.path"
                                    :aria-label="field.label.replace(/ › /g, ', ')"
                                    class="w-full text-sm border-stone-300 rounded mt-1 focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                                    :size="Math.min(field.enum.length, 6)"
                            >
                                <template x-for="enumVal in field.enum" :key="enumVal">
                                    <option :value="enumVal" x-text="enumVal"></option>
                                </template>
                            </select>
                        </fieldset>
                    </template>

                    <!-- String / Number / Integer: text or number input with operator -->
                    <template x-if="!field.enum && field.type !== 'boolean'">
                        <div class="w-full">
                            <label
                                :for="id + '-' + field.path"
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                                :aria-label="field.label.replace(/ › /g, ', ')"
                            ></label>
                            <div class="flex gap-1 items-center w-full mt-1">
                                <!-- Collapsed operator symbol (clickable) -->
                                <template x-if="!field.showOperator">
                                    <button
                                        type="button"
                                        data-operator-toggle
                                        @click="toggleOperator(field)"
                                        class="text-xs text-stone-400 hover:text-amber-700 underline cursor-pointer flex-shrink-0 w-5 text-center focus:outline-none focus:ring-1 focus:ring-amber-600 rounded"
                                        :aria-label="'Change operator, currently ' + getSymbol(field)"
                                        :title="'Operator: ' + getSymbol(field)"
                                        x-text="getSymbol(field)"
                                    ></button>
                                </template>
                                <!-- Expanded operator dropdown -->
                                <template x-if="field.showOperator">
                                    <select
                                        x-model="field.operator"
                                        @change="selectOperator(field)"
                                        :aria-label="'Operator for ' + field.label"
                                        class="flex-shrink-0 w-16 text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                                    >
                                        <template x-for="op in field.operators" :key="op.code">
                                            <option :value="op.code" x-text="op.label"></option>
                                        </template>
                                    </select>
                                </template>
                                <input
                                    :type="(field.type === 'number' || field.type === 'integer') ? 'number' : 'text'"
                                    :step="field.type === 'integer' ? '1' : 'any'"
                                    x-model="field.value"
                                    :id="id + '-' + field.path"
                                    class="flex-grow w-full text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                                >
                            </div>
                        </div>
                    </template>
                </div>
            </template>
        </div>
    </template>
</div>
