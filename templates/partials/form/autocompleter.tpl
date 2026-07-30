{# `profile` names the explicit selector profile this field uses and takes domain inputs #}
{# (entity/usage) rather than endpoints. Every profile renders identical markup, hidden #}
{# controls, ids and ARIA relationships. #}
<div
        x-data="{% if profile == 'single' %}singleEntitySelector{% elif profile == 'multi' %}multiEntitySelector{% elif profile == 'creatable' %}creatableEntitySelector{% elif profile == 'tag' %}tagFieldSelector{% endif %}({
        {% if entity %}entity: '{{ entity }}',{% endif %}
        {% if usage %}{% if profile == 'tag' %}usage: '{{ usage }}',{% else %}tagSuggestions: { usage: '{{ usage }}' },{% endif %}{% endif %}
        selected: {{ selectedItems|json }} || [],
        form: { name: '{{ elName }}', minimum: parseInt('{{ min }}') || 0 },
        {% if max %}maximum: parseInt('{{ max }}') || 0,{% endif %}
        {% if categoryDecoration %}categoryDecoration: true,{% endif %}
        {% if parameters %}parameters: () => ({{ parameters }}),{% endif %}
    })"
        data-selector-field="{{ elName }}"
        data-selector-profile="{{ profile }}"
        class="relative w-full"
>
    {# Finding 58: min=1 already told this partial the field is required — the #}
    {# relation-type form passes it for both From Category and To Category, and the #}
    {# server rejects a submit without them — but nothing was ever marked. Only the #}
    {# Name field (a different partial) rendered the "*"/Required pair, so a #}
    {# screen-reader user got no warning before submitting and no aria-invalid #}
    {# afterwards. The marker is derived from min rather than a new parameter so #}
    {# every existing call site that already declares a minimum gets it. #}
    {% with isRequired=min|default:0 %}
    {% if title %}
    <label class="block text-xs font-mono font-medium text-stone-600 mt-2" id="{{ id }}-label" for="{{ id }}">{{ title }}{% if isRequired %} <span class="text-red-700" aria-hidden="true">*</span>{% endif %}</label>
    {% endif %}
    {% if isRequired %}<span class="text-xs font-sans text-stone-500" id="{{ id }}-required">Required</span>{% endif %}
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
                    {% if isRequired %}aria-required="true"{% endif %}
                    {# aria-invalid is bound, not static: it has to become true when the #}
                    {# server rejects the submit and errorMessage is populated. #}
                    :aria-invalid="errorMessage ? 'true' : null"
                    :aria-describedby="errorMessage ? '{{ id }}-error' : {% if isRequired %}'{{ id }}-required'{% else %}null{% endif %}"
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
{% endwith %}
