<button type="button" data-testid="bulk-compare-action" class="bulk-action-btn"
        :disabled="!!unavailableReason()" @click="compare('{{ bulkEntity }}')">Compare</button>
