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
        role="group"
        :aria-label="title"
>
    <p x-text="title" id="{{ id }}-title" class="block text-sm font-medium text-gray-700 mt-3"></p>
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
        <div class="w-full" role="group" :aria-label="'Field ' + (index + 1)">
            <template x-if="field.name && field.value && !jsonOutput">
                <input type="hidden" :name="name + '.' + index" :value="generateParamNameForMeta(field)">
            </template>
            <div class="relative w-full flex gap-2">
                <input
                    x-model="field.name"
                    list="listData_{{ id }}"
                    type="text"
                    :aria-label="'Field ' + (index + 1) + ' name'"
                    :id="'{{ id }}-field-' + index + '-name'"
                    class="flex-shrink w-full shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md mt-2"
                >
                <template x-if="!jsonOutput">
                    <select
                            x-model="field.operation"
                            :aria-label="'Field ' + (index + 1) + ' comparison operator'"
                            :id="'{{ id }}-field-' + index + '-op'"
                            class="flex-shrink w-full shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md mt-2"
                    >
                        <option value="EQ" aria-label="equals">=</option>
                        <option value="LI" aria-label="like, contains">LIKE</option>
                        <option value="NE" aria-label="not equals">&lt;&gt;</option>
                        <option value="NL" aria-label="not like, does not contain">NOT LIKE</option>
                        <option value="GT" aria-label="greater than">&gt;</option>
                        <option value="GE" aria-label="greater than or equal">&gt;=</option>
                        <option value="LT" aria-label="less than">&lt;</option>
                        <option value="LE" aria-label="less than or equal">&lt;=</option>
                    </select>
                </template>
                <input
                    type="text"
                    x-model="field.value"
                    :aria-label="'Field ' + (index + 1) + ' value'"
                    :id="'{{ id }}-field-' + index + '-value'"
                    class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block sm:text-sm border-gray-300 rounded-md mt-2"
                    :class="jsonOutput && 'w-full'"
                >
            </div>
        </div>
    </template>
    <button
        type="button"
        @click.prevent="fields.push({name:'',operation:'', value:''})"
        aria-label="Add new field"
        class="
            mt-2 inline-flex items-center
            px-2 py-1
            border border-gray-300 rounded-md
            shadow-sm text-xs font-medium text-white bg-green-700 hover:bg-green-800
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-700">
        Add Field
    </button>
</div>