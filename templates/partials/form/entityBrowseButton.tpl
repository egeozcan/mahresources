<button type="button" x-cloak data-entity-browse @click="openEntityBrowser($event)"
        :disabled="browserDisabled" :aria-label="browseLabel" :title="browseLabel"
        class="inline-flex items-center justify-center flex-shrink-0 w-9 h-9 mt-1 bg-white border border-stone-300 rounded text-stone-700 hover:bg-stone-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-700 disabled:opacity-50">
    <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4">
        <circle cx="10.5" cy="10.5" r="6.5"></circle><path d="m16 16 4.5 4.5"></path>
    </svg>
</button>
