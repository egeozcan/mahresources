{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">
        {{ title }} {% if required %}<span class="text-red-700">*</span>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2">
        <div class="max-w-lg flex rounded-md shadow-sm">
            <input
                    value="{{ value }}"
                    {% if type %}type="{{ type }}"{% else %}type="text"{% endif %}
                    {% if required %}required aria-required="true"{% endif %}
                    {% if pattern %}pattern="{{ pattern }}"{% endif %}
                    {% if patternHint %}title="{{ patternHint }}"{% endif %}
                    name="{{ name }}"
                    id="{{ field_id }}"
                    autocomplete="off"
                    {% if description or required %}aria-describedby="{{ field_id }}-description"{% endif %}
                    class="flex-1 block w-full focus:ring-amber-600 focus:border-amber-600 min-w-0 rounded-md sm:text-sm border-stone-300"
            >
        </div>
        {# Finding 157: `description` was accepted by the caller and rendered by #}
        {# nobody, so the template-partial Name field's kebab-case rule -- passed #}
        {# in since the field was written -- could only be learned by being #}
        {# rejected. aria-describedby is now an attribute rather than a script. #}
        {% if description or required %}
        <span class="text-sm font-sans text-stone-500" id="{{ field_id }}-description">{% if description %}{{ description }}{% if required %} {% endif %}{% endif %}{% if required %}Required{% endif %}</span>
        {% endif %}
    </div>
</div>
{% endwith %}
