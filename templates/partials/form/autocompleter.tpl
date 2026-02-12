<div
        x-data="autocompleter({
        selectedResults: {{ selectedItems|json }} || [],
        min: parseInt('{{ min }}') || 0,
        max: parseInt('{{ max }}') || 0,
        ownerId: parseInt('{{ ownerId }}') || 0,
        url: '{{ url }}',
        addUrl: '{{ addUrl }}',
        elName: '{{ elName }}',
        filterEls: '{{ filterEls }}' || [],
        extraInfo: '{{ extraInfo }}',
        sortBy: '{{ sortBy }}',
    })"
        class="relative w-full"
>
    {% if title %}
    <label class="block text-sm font-medium text-gray-700 mt-3" id="{{ id }}-label" for="{{ id }}">{{ title }}</label>
    {% endif %}
    {% include "/partials/form/formParts/errorMessage.tpl" %}
    <template x-if="addModeForTag == ''">
        <div>
            <input
                    id="{{ id }}"
                    x-ref="autocompleter"
                    type="text"
                    class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-2"
                    x-bind="inputEvents"
                    x-init="setTimeout(() => { addModeForTag !== false && $el.focus(); }, 1)"
                    autocomplete="off"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-expanded="dropdownActive && results.length > 0"
                    aria-controls="{{ id }}-listbox"
                    {% if title %}aria-labelledby="{{ id }}-label"{% endif %}
                    :aria-describedby="errorMessage ? '{{ id }}-error' : null"
                    aria-owns="{{ id }}-listbox"
                    :aria-activedescendant="selectedIndex >= 0 && results[selectedIndex] ? '{{ id }}-result-' + selectedIndex : null"
            >
            {% include "/partials/form/formParts/dropDownResults.tpl" with action="pushVal" %}
            {% include "/partials/form/formParts/dropDownSelectedResults.tpl" %}
        </div>
    </template>
    <template x-if="addModeForTag">
        <div class="flex gap-2 items-stretch justify-between mt-2">
            <button
                    type="button"
                    class="
                    border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-green-700
                    hover:bg-green-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500
                    inline-flex justify-center items-center py-1 px-2"
                    x-text="'Add ' + addModeForTag + '?'"
                    x-init="setTimeout(() => $el.focus(), 1)"
                    @keydown.escape.prevent="exitAdd"
                    @keydown.enter.prevent="addVal"
                    @keyup.prevent=""
            ></button>
            <button
                    type="button"
                    class="
                    border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-red-600
                    hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500
                    inline-flex justify-center items-center py-1 px-2"
                    x-ref="cancelAdd"
                    @click="exitAdd"
                    @keydown.escape.prevent="exitAdd"
            >Cancel</button>
        </div>
    </template>
    <template x-for="(result, index) in selectedResults">
        <input type="hidden" name="{{ elName }}" :value="result.ID">
    </template>
</div>
