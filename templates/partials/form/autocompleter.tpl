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
    <label class="block text-sm font-mono font-medium text-stone-700 mt-3" id="{{ id }}-label" for="{{ id }}">{{ title }}</label>
    {% endif %}
    {% include "/partials/form/formParts/errorMessage.tpl" %}
    <template x-if="!addModeForTag">
        <div>
            <input
                    id="{{ id }}"
                    x-ref="autocompleter"
                    type="text"
                    class="shadow-sm focus:ring-amber-600 focus:border-amber-600 block w-full sm:text-sm border-stone-300 rounded-md mt-2"
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
                    border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700
                    hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600
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
                    border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-red-700
                    hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600
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