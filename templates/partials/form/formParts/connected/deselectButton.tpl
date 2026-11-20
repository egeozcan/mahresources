<button type="button"
    data-bulk-deselect-all
    @click.prevent="$selection.deselectAll()"
    class="bulk-selection-btn"
>
    {% if text %}{{ text }}{% else %}Deselect All{% endif %}
</button>