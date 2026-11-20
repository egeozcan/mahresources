<button type="button" class="bulk-action-btn"
        :disabled="!!unavailableReason()" @click="runPlugin()">{{ action.Label }}</button>
