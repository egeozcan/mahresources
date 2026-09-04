// Plugin blocks mounted in one Alpine pass enqueue into the same microtask, so
// the editor pays one HTTP round trip per note/mode instead of one per block.
const pendingRenderBatches = new Map();

function enqueueBatchRender(block, mode) {
    if (!block.noteId) return Promise.reject(new Error('Plugin block is missing its note id'));
    const key = `${block.noteId}:${mode}`;
    let batch = pendingRenderBatches.get(key);
    if (!batch) {
        batch = { noteId: block.noteId, mode, requests: new Map() };
        pendingRenderBatches.set(key, batch);
        queueMicrotask(() => flushBatch(key, batch));
    }
    return new Promise((resolve, reject) => {
        const waiters = batch.requests.get(block.id) || [];
        waiters.push({ resolve, reject });
        batch.requests.set(block.id, waiters);
    });
}

async function flushBatch(key, batch) {
    if (pendingRenderBatches.get(key) === batch) pendingRenderBatches.delete(key);
    try {
        const res = await fetch('/v1/plugins/block/render-batch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                noteId: batch.noteId,
                mode: batch.mode,
                blockIds: [...batch.requests.keys()],
            }),
        });
        if (!res.ok) throw new Error(await res.text());
        const payload = await res.json();
        for (const [id, waiters] of batch.requests) {
            const rendered = payload.renders?.[id];
            const message = payload.errors?.[id];
            for (const waiter of waiters) {
                if (message) waiter.reject(new Error(message));
                else if (typeof rendered === 'string') waiter.resolve(rendered);
                else waiter.reject(new Error('Plugin block render was not returned'));
            }
        }
    } catch (err) {
        for (const waiters of batch.requests.values()) {
            for (const waiter of waiters) waiter.reject(err);
        }
    }
}

export function blockPlugin(block, getEditMode, getLabel = null) {
    return {
        block,
        renderedHtml: '',
        renderError: null,
        renderLoading: false,
        _lastMode: null,
        _lastContentKey: null,
        _lastStateKey: null,

        get editMode() {
            return getEditMode();
        },

        get label() {
            return getLabel ? getLabel() : 'plugin block';
        },

        async loadRender() {
            const mode = this.editMode ? 'edit' : 'view';
            const contentKey = JSON.stringify(this.block.content);
            const stateKey = JSON.stringify(this.block.state);

            // Skip if nothing changed
            if (mode === this._lastMode && contentKey === this._lastContentKey && stateKey === this._lastStateKey) {
                return;
            }
            this._lastMode = mode;
            this._lastContentKey = contentKey;
            this._lastStateKey = stateKey;

            this.renderLoading = true;
            this.renderError = null;

            try {
                this.renderedHtml = await enqueueBatchRender(this.block, mode);
            } catch (err) {
                this.renderError = err.message;
            } finally {
                this.renderLoading = false;
            }
        }
    };
}
