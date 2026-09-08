    <div class="px-4 order-2">
        <span class="block text-sm font-mono font-medium text-stone-700 mt-3">Retry Selected</span>
        <button type="button"
                @click="$store.downloads.retrySelected()"
                :aria-disabled="$store.downloads.busy"
                aria-label="Retry selected downloads"
                data-testid="downloads-bulk-retry"
                class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500 aria-disabled:opacity-50 aria-disabled:cursor-not-allowed">
            Retry
        </button>
    </div>
