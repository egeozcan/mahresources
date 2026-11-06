{# The History button of one template slot: slot, label; tpCarrierId from the enclosing with. Edit forms only, since a new carrier has no revisions. #}
{# The status beside it announces an Apply from the dialog, since focus comes back to this button. It is cleared first, so applying the same text again is announced again. #}
{% if tpCarrierId %}
<span class="contents" x-data="{ note: '' }"
      @template-revision-write.document="if (($event.detail.fields || []).includes('{{ slot }}')) { const text = $event.detail.notice || ''; note = ''; setTimeout(() => { note = text }, 100) }">
    <button type="button" data-testid="slot-history-{{ slot }}" aria-label="History of {{ label }}"
            @click="window.dispatchEvent(new CustomEvent('open-slot-history', { detail: { slot: '{{ slot }}', label: '{{ label|escapejs }}', opener: $event.currentTarget } }))"
            class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-white border border-stone-300 rounded hover:bg-stone-100 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">History</button>
    <p role="status" data-testid="slot-applied-{{ slot }}" x-text="note"
       :class="note ? 'basis-full text-xs text-green-900 bg-green-50 border-l-4 border-green-700 px-3 py-2' : 'basis-full'"></p>
</span>
{% endif %}
