<div x-data x-show="[...$store.bulkSelection.selectedIds].length + 1 !== $store.bulkSelection.elements.length" x-collapse>
    <button type="button"
        @click.prevent="$store.bulkSelection.selectAll()"
        class="
            inline-flex justify-center
            py-2 px-4 mt-3
            border border-transparent
            items-center
            shadow-sm text-xs font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600"
    >
        {% if text %}{{ text }}{% else %}Select All{% endif %}
    </button>
</div>