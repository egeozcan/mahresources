<button type="submit" class="
    inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-2 px-4 mt-3
    {% endif %}
    border border-transparent
    shadow-sm text-sm font-mono font-medium rounded-md text-white
    {% if danger %}
        bg-red-700 hover:bg-red-800 focus:ring-red-600
    {% else %}
        bg-amber-700 hover:bg-amber-800 focus:ring-amber-600
    {% endif %}
    focus:outline-none focus:ring-2 focus:ring-offset-2 ">
    {% if text %}{% autoescape off %}{{ text }}{% endautoescape %}{% else %}Apply Filters{% endif %}
</button>