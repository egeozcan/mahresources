<button type="submit" class="
    inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-2 px-4
    {% endif %}
    border border-transparent
    shadow-sm text-sm font-mono font-medium rounded-md
    text-white bg-amber-700 hover:bg-amber-800
    focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
    {% if text %}{{ text }}{% else %}New{% endif %}
</button>