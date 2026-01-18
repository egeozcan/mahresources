<button type="submit" class="
    inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-2 px-4
    {% endif %}
    border border-transparent
    shadow-sm text-sm font-medium rounded-md
    text-white bg-green-700 hover:bg-green-800
    focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500">
    {% if text %}{{ text }}{% else %}New{% endif %}
</button>
