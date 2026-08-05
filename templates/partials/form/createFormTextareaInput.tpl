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
                    {# Deferred-work item 8: this used to declare role="combobox".    #}
                    {# On a <textarea> that override replaces the implicit textbox    #}
                    {# role and takes aria-multiline="true" with it, so the field     #}
                    {# stopped announcing as multiline. Adding aria-multiline back is #}
                    {# NOT the fix: ARIA 1.2 has no multiline combobox and the        #}
                    {# attribute is not in axe's allowed set for role=combobox, so it #}
                    {# would trade this for an aria-allowed-attr failure (wcag2a,     #}
                    {# wcag412, impact critical). The native textbox role is kept     #}
                    {# instead — it allows aria-activedescendant, aria-autocomplete   #}
                    {# and aria-multiline, and aria-controls/-owns/-haspopup are      #}
                    {# global. :aria-expanded had to go with the role: it is neither  #}
                    {# global nor allowed on a textbox. The open/closed state it      #}
                    {# carried is not lost — mentionTextarea.js announces the result  #}
                    {# count through its live region on every search.                 #}
                    aria-autocomplete="list"
                    aria-haspopup="listbox"
                    {# Finding 133: this named none of the listbox it drives, so      #}
                    {# assistive tech could not follow the suggestion list.           #}
                    {# autocompleter.tpl, in this directory, sets both aria-controls  #}
                    {# and aria-owns; this matches it.                                #}
                    aria-controls="{{ field_id }}-mention-listbox"
                    aria-owns="{{ field_id }}-mention-listbox"
                    :aria-activedescendant="activeDescendantId"
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
