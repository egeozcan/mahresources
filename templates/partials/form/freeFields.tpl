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
        <div class="w-full">
            <template x-if="field.name && field.value && !jsonOutput">
                <input type="hidden" :name="name + '.' + index" :value="generateParamNameForMeta(field)">
            </template>
            <div class="relative w-full flex gap-2">
                <input
                    x-model="field.name"
                    list="listData_{{ id }}"
                    type="text"
                    class="flex-shrink w-full shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md mt-2"
                >
                <template x-if="!jsonOutput">
                    <select
                            x-model="field.operation"
                            class="flex-shrink w-full shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md mt-2"
                    >
                        <option value="EQ">=</option>
                        <option value="LI">LIKE</option>
                        <option value="NE">&lt;&gt;</option>
                        <option value="NL">NOT LIKE</option>
                        <option value="GT">&gt;</option>
                        <option value="GE">&gt;=</option>
                        <option value="LT">&lt;</option>
                        <option value="LE">&lt;=</option>
                    </select>
                </template>
                <input
                    type="text"
                    x-model="field.value"
                    class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block sm:text-sm border-gray-300 rounded-md mt-2"
                    :class="jsonOutput && 'w-full'"
                >
            </div>
        </div>
    </template>
    <button
        type="button"
        @click.prevent="fields.push({name:'',operation:'', value:''})"
        class="
            mt-2 inline-flex items-center
            px-2 py-1
            border border-gray-300 rounded-md
            shadow-sm text-xs font-medium text-white bg-green-800 hover:bg-green-500
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-700">
        Add Field
    </button>
</div>