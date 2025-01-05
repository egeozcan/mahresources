{% with id=getName(name) %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-gray-200 sm:pt-5">
    <label for="{{ id }}" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
        {{ title }} {% if required %}<span class="text-red-500">*</span>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2">
        <textarea
                id="{{ id }}"
                name="{{ name }}"
                rows="3"
                {% if required %}required aria-required="true"{% endif %}
                class="{% if big %}{% else %}max-w-lg{% endif %} shadow-sm block w-full focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md"
        >{{ value }}</textarea>
        {% if required %}
        <span class="text-sm text-gray-500" id="{{ id }}-description">Required</span>
        <script>
            document.getElementById("{{ id }}").setAttribute("aria-describedby", "{{ id }}-description");
        </script>
        {% endif %}
    </div>
</div>
{% endwith %}