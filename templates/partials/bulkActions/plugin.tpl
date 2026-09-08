<button type="button" class="bulk-action-btn mt-3 px-3 py-1.5 border rounded-md disabled:opacity-50"
        :disabled="!!unavailableReason()" @click="runPlugin()">{{ action.Label }}</button>
