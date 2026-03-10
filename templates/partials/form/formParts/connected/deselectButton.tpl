<button type="button"
    @click.prevent="$store.bulkSelection.deselectAll()"
    class="
        inline-flex justify-center
        py-2 px-4 mt-3
        border border-transparent
        items-center
        shadow-sm  text-xs font-mono rounded-md text-white bg-stone-500 hover:bg-stone-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-stone-500"
>
    {% if text %}{{ text }}{% else %}Deselect All{% endif %}
</button>