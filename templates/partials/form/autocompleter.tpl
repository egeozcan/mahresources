{# `profile` names an explicit selector profile bridge and takes domain inputs (entity/usage) #}
{# instead of the legacy url/addUrl/filterEls flags. Both paths render identical markup, hidden #}
{# controls, ids and ARIA relationships, so a call site can be migrated on its own. #}
<div
        {% if profile %}
        x-data="{% if profile == 'single' %}singleEntitySelector{% elif profile == 'multi' %}multiEntitySelector{% elif profile == 'creatable' %}creatableEntitySelector{% elif profile == 'tag' %}tagFieldSelector{% endif %}({
        {% if entity %}entity: '{{ entity }}',{% endif %}
        {% if usage %}{% if profile == 'tag' %}usage: '{{ usage }}',{% else %}tagSuggestions: { usage: '{{ usage }}' },{% endif %}{% endif %}
        selected: {{ selectedItems|json }} || [],
        form: { name: '{{ elName }}', minimum: parseInt('{{ min }}') || 0 },
        {% if max %}maximum: parseInt('{{ max }}') || 0,{% endif %}
        {% if categoryDecoration %}categoryDecoration: true,{% endif %}
        {% if parameters %}parameters: () => ({{ parameters }}),{% endif %}
    })"
        {% else %}
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
        {% endif %}
        data-selector-field="{{ elName }}"
        data-selector-profile="{{ profile|default:'legacy' }}"
        class="relative w-full"
>
    {% if title %}
    <label class="block text-xs font-mono font-medium text-stone-600 mt-2" id="{{ id }}-label" for="{{ id }}">{{ title }}</label>
    {% endif %}
    {% include "/partials/form/formParts/errorMessage.tpl" %}
    <template x-if="!addModeForTag">
        <div>
            <input
                    id="{{ id }}"
                    x-ref="autocompleter"
                    type="text"
                    class="focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded mt-1"
                    x-bind="inputEvents"
                    x-init="setTimeout(() => { addModeForTag !== false && $el.focus(); }, 1)"
                    autocomplete="off"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-expanded="dropdownActive && (results.length > 0 || createCandidate)"
                    aria-controls="{{ id }}-listbox"
                    {% if title %}aria-labelledby="{{ id }}-label"{% endif %}
                    :aria-describedby="errorMessage ? '{{ id }}-error' : null"
                    aria-owns="{{ id }}-listbox"
                    :aria-activedescendant="selectedIndex >= 0 ? '{{ id }}-result-' + selectedIndex : null"
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
                    @click="addVal"
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
    <input type="hidden" name="{{ elName }}" value="" :disabled="selectedResults.length > 0">
</div>
