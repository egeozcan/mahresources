{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">
        {{ title }}
        {% if description %}<p class="text-xs text-stone-500 mt-0.5 font-sans font-normal">{{ description }}</p>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2"
         x-data="codeEditor({ mode: '{{ mode }}', dbType: '{{ dbType }}', label: '{{ title }}' })">
        <input type="hidden" id="{{ field_id }}" name="{{ name }}" x-ref="hiddenInput" value="{{ value }}">
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
        <div x-ref="editorContainer" class="border border-stone-300 rounded-md overflow-hidden"></div>
        {% if mode == "json" or mode == "html" %}
        <div class="mt-1 min-h-[1.25rem]">
            <p x-show="formatError"
               x-transition
               class="text-xs text-red-600 font-mono"
               role="alert"
               x-text="formatError"></p>
        </div>
        {% endif %}
    </div>
</div>
{% endwith %}