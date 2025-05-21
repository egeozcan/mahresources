{% raw %}<div
    x-data="customDataFields({
        definitions: {{ definitions|json|default:'[]' }},
        initialMeta: {{ metaData|json|default:'{}' }},
        inputName: '{{ name|default:'Meta' }}'
    })"
    class="space-y-4"
>
    <input type="hidden" :name="inputName" :value="jsonOutput">

    <template x-for="fieldDef in definitions" :key="fieldDef.name">
        <div class="p-4 border border-gray-200 rounded-md">
            <label :for="'custom_field_' + fieldDef.name" class="block text-sm font-medium text-gray-700" x-text="fieldDef.label || fieldDef.name"></label>
            <p x-show="fieldDef.options && fieldDef.options.description" class="text-xs text-gray-500" x-text="fieldDef.options.description"></p>

            <template x-if="fieldDef.type === 'text'">
                <input
                    type="text"
                    :name="'custom_field_input_' + fieldDef.name"
                    :id="'custom_field_' + fieldDef.name"
                    x-model="values[fieldDef.name]"
                    class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                >
            </template>

            <template x-if="fieldDef.type === 'number' || fieldDef.type === 'rating'">
                <input
                    type="number"
                    :name="'custom_field_input_' + fieldDef.name"
                    :id="'custom_field_' + fieldDef.name"
                    x-model.number="values[fieldDef.name]"
                    :min="fieldDef.options && fieldDef.options.min"
                    :max="fieldDef.options && fieldDef.options.max"
                    class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                >
            </template>

            <template x-if="fieldDef.type === 'reference'">
                <!-- Basic text input for reference, can be enhanced later -->
                <input
                    type="text"
                    :name="'custom_field_input_' + fieldDef.name"
                    :id="'custom_field_' + fieldDef.name"
                    x-model="values[fieldDef.name]"
                    class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                    :placeholder="fieldDef.options && fieldDef.options.referencedEntity ? 'ID of ' + fieldDef.options.referencedEntity : 'Reference ID'"
                >
            </template>

            <!-- Add other types here: e.g., boolean (checkbox), select (from options) -->

        </div>
    </template>

    <p x-show="definitions.length === 0" class="text-sm text-gray-500">
        No custom fields defined for this category, or category not selected.
    </p>
</div>{% endraw %}
