<div
        class="w-full"
        x-data="freeFields({
            fields: {{ fields|json }} || [],
            name: '{{ name }}',
            url: '{{ url }}',
            jsonOutput: '{{ jsonOutput }}' || false,
            id: '{{ id }}',
            title: '{{ fieldsTitle }}' || 'Meta',
            fromJSON: {{ fromJSON|json }} || '',
        })"
>
    <p x-text="title" class="block text-sm font-medium text-gray-700 mt-3"></p>
    <template x-if="jsonOutput">
        <input type="hidden" :name="name" :value="jsonText">
    </template>
    <template x-if="url">
        <datalist id="listData_{{ id }}">
            <template x-for="(field, index) in remoteFields" :key="index">
                <option :value="field.Key" />
            </template>
        </datalist>
    </template>
    <template x-for="(field, index) in fields" :key="index">
        <div class="w-full border border-gray-200 p-4 rounded-md mt-2">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                    <label :for="'field_label_' + index" class="block text-sm font-medium text-gray-700">Label</label>
                    <input
                        x-model="field.label"
                        :id="'field_label_' + index"
                        type="text"
                        class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                        placeholder="User-friendly label"
                    >
                </div>
                <div>
                    <label :for="'field_name_' + index" class="block text-sm font-medium text-gray-700">Name</label>
                    <input
                        x-model="field.name"
                        :id="'field_name_' + index"
                        type="text"
                        list="listData_{{ id }}"
                        class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                        placeholder="Technical field name (e.g., my_custom_field)"
                    >
                </div>
                <div>
                    <label :for="'field_type_' + index" class="block text-sm font-medium text-gray-700">Type</label>
                    <select
                        x-model="field.type"
                        @change="field.options = {}" // Reset options on type change
                        :id="'field_type_' + index"
                        class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                    >
                        <option value="text">Text</option>
                        <option value="number">Number</option>
                        <option value="rating">Rating</option>
                        <option value="reference">Reference</option>
                    </select>
                </div>
            </div>

            <!-- Conditional options based on type -->
            <div x-show="field.type === 'number' || field.type === 'rating'" class="mt-4 grid grid-cols-2 gap-4">
                <div>
                    <label :for="'field_options_min_' + index" class="block text-sm font-medium text-gray-700">Min</label>
                    <input
                        x-model.number="field.options.min"
                        :id="'field_options_min_' + index"
                        type="number"
                        class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                    >
                </div>
                <div>
                    <label :for="'field_options_max_' + index" class="block text-sm font-medium text-gray-700">Max</label>
                    <input
                        x-model.number="field.options.max"
                        :id="'field_options_max_' + index"
                        type="number"
                        class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                    >
                </div>
            </div>

            <div x-show="field.type === 'reference'" class="mt-4">
                <div>
                    <label :for="'field_options_referencedEntity_' + index" class="block text-sm font-medium text-gray-700">Referenced Entity</label>
                    <input
                        x-model="field.options.referencedEntity"
                        :id="'field_options_referencedEntity_' + index"
                        type="text"
                        class="mt-1 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md"
                        placeholder="e.g., User, Product"
                    >
                </div>
            </div>

            <button
                type="button"
                @click.prevent="fields.splice(index, 1)"
                class="mt-2 inline-flex items-center px-2 py-1 border border-transparent text-xs font-medium rounded shadow-sm text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
            >
                Remove
            </button>
        </div>
    </template>
    <button
        type="button"
        @click.prevent="fields.push({name: '', label: '', type: 'text', options: {}})"
        class="
            mt-2 inline-flex items-center
            px-2 py-1
            border border-gray-300 rounded-md
            shadow-sm text-xs font-medium text-white bg-green-800 hover:bg-green-500
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-700">
        Add Field
    </button>
</div>