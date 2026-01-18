{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-gray-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
        {{ title }} {% if required %}<span class="text-red-500">*</span>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2">
        <div class="max-w-lg flex rounded-md shadow-sm">
            <input
                    value="{{ value }}"
                    {% if type %}type="{{ type }}"{% else %}type="text"{% endif %}
                    {% if required %}required aria-required="true"{% endif %}
                    name="{{ name }}"
                    id="{{ field_id }}"
                    autocomplete="off"
                    class="flex-1 block w-full focus:ring-indigo-500 focus:border-indigo-500 min-w-0 rounded-md sm:text-sm border-gray-300"
            >
        </div>
        {% if required %}
        <span class="text-sm text-gray-500" id="{{ field_id }}-description">Required</span>
        <script>
            document.getElementById("{{ field_id }}").setAttribute("aria-describedby", "{{ field_id }}-description");
        </script>
        {% endif %}
    </div>
</div>
{% endwith %}