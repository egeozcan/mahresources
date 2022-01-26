<button type="submit" class="
    inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-2 px-4 mt-3
    {% endif %}
    border border-transparent
    shadow-sm text-sm font-medium rounded-md text-white
    {% if danger %}
        bg-red-600 hover:bg-red-700 focus:ring-red-500
    {% else %}
        bg-indigo-600 hover:bg-indigo-700 focus:ring-indigo-500
    {% endif %}
    focus:outline-none focus:ring-2 focus:ring-offset-2 ">
    {% if text %}{{ text }}{% else %}Search{% endif %}
</button>
