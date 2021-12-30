<button type="button"
    @click.prevent="$store.bulkSelection.deselectAll()"
    class="
        inline-flex justify-center
        py-2 px-4 mt-3
        border border-transparent
        items-center
        shadow-sm text-sm font-medium rounded-md text-white bg-gray-500 hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500"
>
    {% if text %}{{ text }}{% else %}Deselect All{% endif %}
</button>