{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">
        {{ title }}
        {% if description %}<p class="text-xs text-stone-500 mt-0.5 font-sans font-normal">{{ description }}</p>{% endif %}
    </label>
    {# min-w-0: this column holds a CodeMirror editor and a nowrap button row, so #}
    {# without it the block's min-content width propagates up and overflows the    #}
    {# viewport (findings 19/101).                                                 #}
    <div class="mt-1 sm:mt-0 sm:col-span-2 min-w-0"
         x-data="codeEditor({ mode: '{{ mode }}', dbType: '{{ dbType }}', label: '{{ title }}', shortcodes: {% if shortcodes %}true{% else %}false{% endif %}, generate: {% if generate %}true{% else %}false{% endif %} })">
        <input type="hidden" id="{{ field_id }}" name="{{ name }}" x-ref="hiddenInput" value="{{ value }}">
        {% if generate %}
        {% include "/partials/form/templateGenerationControls.tpl" %}
        {% endif %}
        {% if mode == "json" or mode == "html" %}
        <div class="flex items-center justify-end mb-1">
            <button type="button"
                    @click="formatContent()"
                    class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-600 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer"
                    aria-label="Format {{ mode|upper }} content">
                Format {{ mode|upper }}
            </button>
        </div>
        {% endif %}
        {# overflow-x:auto rather than hidden: a long template line is content the  #}
        {# author needs to reach, and clipping it made it unreachable (findings      #}
        {# 19/101). CodeMirror's own scroller handles the vertical axis.             #}
        <div x-ref="editorContainer" class="border border-stone-300 rounded-md overflow-hidden max-w-full"></div>
        {% if mode == "json" or mode == "html" %}
        <div class="mt-1 min-h-[1.25rem]">
            <p x-show="formatError"
               x-transition
               class="text-xs text-red-600 font-mono"
               role="alert"
               x-text="formatError"></p>
            {% if mode == "json" %}
            {# Findings 17/93, a11y half: the automatic JSON lint was gutter       #}
            {# markers and an underlined range, which a screen reader cannot see.  #}
            {# This is the same diagnosis in text, and it is what the editor's      #}
            {# aria-describedby points at — codeEditor.js derives the same id from  #}
            {# the field name. role=status/aria-live=polite rather than alert:      #}
            {# unlike the Format JSON button's error, this fires while typing, and  #}
            {# an assertive region would interrupt on every pause.                  #}
            <p id="json-lint-{{ name }}"
               data-testid="json-lint-{{ name }}"
               x-show="lintError"
               x-cloak
               class="text-xs text-red-600 font-mono"
               role="status"
               aria-live="polite"
               x-text="lintError ? 'Invalid JSON: ' + lintError : ''"></p>
            {% endif %}
        </div>
        {% endif %}
    </div>
</div>
{% endwith %}