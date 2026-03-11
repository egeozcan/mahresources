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
    <p x-text="title" id="{{ id }}-title" class="block text-xs font-mono font-medium text-stone-600 mt-2"></p>
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
                    class="flex-shrink w-full focus:ring-1 focus:ring-amber-600 focus:border-amber-600 text-sm border-stone-300 rounded mt-1"
                >
                <template x-if="!jsonOutput">
                    <select
                            x-model="field.operation"
                            :aria-label="'Field ' + (index + 1) + ' comparison operator'"
                            :id="'{{ id }}-field-' + index + '-op'"
                            class="flex-shrink w-full focus:ring-1 focus:ring-amber-600 focus:border-amber-600 text-sm border-stone-300 rounded mt-1"
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
                    class="focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block text-sm border-stone-300 rounded mt-1"
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
            mt-1.5 inline-flex items-center gap-0.5
            text-xs font-mono font-medium text-stone-500 hover:text-amber-700
            focus:outline-none focus:text-amber-700 transition-colors duration-100">
        + Add Field
    </button>
</div>