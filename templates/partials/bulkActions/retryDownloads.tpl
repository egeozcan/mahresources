    <div class="bulk-action-item">
        <button type="button"
                @click="$store.downloads.retrySelected($selection)"
                :aria-disabled="$store.downloads.busy"
                aria-label="Retry selected downloads"
                data-testid="downloads-bulk-retry"
                class="bulk-action-btn">
            Retry
        </button>
    </div>
