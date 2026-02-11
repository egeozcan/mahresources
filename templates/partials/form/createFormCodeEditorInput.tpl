{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-gray-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
        {{ title }}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2"
         x-data="codeEditor({ mode: '{{ mode }}', dbType: '{{ dbType }}', label: '{{ title }}' })">
        <input type="hidden" name="{{ name }}" x-ref="hiddenInput" value="{{ value }}">
        <div x-ref="editorContainer" class="border border-gray-300 rounded-md overflow-hidden"></div>
    </div>
</div>
{% endwith %}
