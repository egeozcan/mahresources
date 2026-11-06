{# The History button of one template slot: slot, label; tpCarrierId from the enclosing with. Edit forms only, since a new carrier has no revisions. #}
{% if tpCarrierId %}
<span x-data class="inline-flex">
    <button type="button" data-testid="slot-history-{{ slot }}" aria-label="History of {{ label }}"
            @click="window.dispatchEvent(new CustomEvent('open-slot-history', { detail: { slot: '{{ slot }}', label: '{{ label|escapejs }}', opener: $event.currentTarget } }))"
            class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-white border border-stone-300 rounded hover:bg-stone-100 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">History</button>
</span>
{% endif %}
