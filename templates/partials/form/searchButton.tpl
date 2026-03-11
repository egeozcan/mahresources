{% if not small %}<div class="sticky bottom-12 bg-stone-50 pt-3 z-10 w-full">{% endif %}
<button type="submit" class="
    w-full inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-1.5 px-3
    {% endif %}
    border border-transparent
    text-xs font-mono font-semibold tracking-wide rounded text-white
    {% if danger %}
        bg-red-700 hover:bg-red-800 focus:ring-red-600
    {% else %}
        bg-amber-700 hover:bg-amber-800 focus:ring-amber-600
    {% endif %}
    focus:outline-none focus:ring-2 focus:ring-offset-1
    transition-colors duration-100">
    {% if text %}{% autoescape off %}{{ text }}{% endautoescape %}{% else %}Apply Filters{% endif %}
</button>
{% if not small %}</div>{% endif %}