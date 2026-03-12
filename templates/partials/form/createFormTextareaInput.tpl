{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">
        {{ title }} {% if required %}<span class="text-red-700">*</span>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2">
        {% if mentionTypes %}
        <div class="relative" x-data="mentionTextarea('{{ mentionTypes }}')">
            <textarea
                    x-ref="mentionInput"
                    id="{{ field_id }}"
                    name="{{ name }}"
                    rows="3"
                    @input="onInput($event)"
                    @keydown="onKeydown($event)"
                    {% if required %}required aria-required="true"{% endif %}
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-expanded="mentionActive && mentionResults.length > 0"
                    aria-haspopup="listbox"
                    class="{% if big %}{% else %}max-w-lg{% endif %} shadow-sm block w-full focus:ring-amber-600 focus:border-amber-600 sm:text-sm border-stone-300 rounded-md"
            >{{ value }}</textarea>
            {% include "/partials/form/mentionDropdown.tpl" %}
        </div>
        {% else %}
        <textarea
                id="{{ field_id }}"
                name="{{ name }}"
                rows="3"
                {% if required %}required aria-required="true"{% endif %}
                class="{% if big %}{% else %}max-w-lg{% endif %} shadow-sm block w-full focus:ring-amber-600 focus:border-amber-600 sm:text-sm border-stone-300 rounded-md"
        >{{ value }}</textarea>
        {% endif %}
        {% if required %}
        <span class="text-sm font-sans text-stone-500" id="{{ field_id }}-description">Required</span>
        <script>
            document.getElementById("{{ field_id }}").setAttribute("aria-describedby", "{{ field_id }}-description");
        </script>
        {% endif %}
    </div>
</div>
{% endwith %}
