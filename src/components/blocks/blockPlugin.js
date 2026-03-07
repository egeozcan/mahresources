// src/components/blocks/blockPlugin.js
export function blockPlugin(block, getEditMode) {
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

            const pluginName = this.block.type.split(':')[1];
            this.renderLoading = true;
            this.renderError = null;

            try {
                const res = await fetch(
                    `/v1/plugins/${encodeURIComponent(pluginName)}/block/render?blockId=${this.block.id}&mode=${mode}`
                );
                if (!res.ok) {
                    throw new Error(await res.text());
                }
                this.renderedHtml = await res.text();
            } catch (err) {
                this.renderError = err.message;
            } finally {
                this.renderLoading = false;
            }
        }
    };
}
