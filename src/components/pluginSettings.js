/**
 * How long the "Saved!" status stands before the reload navigates away from it.
 * Same 1500ms as the sibling plugin surface (src/components/pluginActionModal.js),
 * so the two "a plugin admin action succeeded" confirmations behave alike.
 * Exported so the test asserts the scheduling, not the number.
 */
export const SAVED_ANNOUNCEMENT_MS = 1500;

export function pluginSettings(pluginName) {
    return {
        pluginName,
        saved: false,
        error: '',
        /** Timer id of the pending post-save reload; see saveSettings. */
        _reloadTimer: null,

        async saveSettings(event) {
            this.saved = false;
            this.error = '';
            // The price of delaying the reload below: a save made inside that window
            // would otherwise be cut short by the *previous* save's navigation, and the
            // page would come back carrying the value the reader had just replaced.
            if (this._reloadTimer !== null) {
                clearTimeout(this._reloadTimer);
                this._reloadTimer = null;
            }

            const form = event.target;
            const formData = new FormData(form);
            const values = {};

            // Build values object from form
            for (const [key, value] of formData.entries()) {
                if (key === 'name') continue;
                values[key] = value;
            }

            // Handle checkboxes (unchecked ones aren't in FormData)
            form.querySelectorAll('input[type="checkbox"]').forEach(cb => {
                values[cb.name] = cb.checked;
            });

            // Handle number fields
            form.querySelectorAll('input[type="number"]').forEach(input => {
                if (values[input.name] !== undefined && values[input.name] !== '') {
                    values[input.name] = parseFloat(values[input.name]);
                }
            });

            try {
                const response = await fetch(`/v1/plugin/settings?name=${encodeURIComponent(this.pluginName)}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(values),
                });

                if (!response.ok) {
                    const contentType = response.headers.get('Content-Type') || '';
                    if (contentType.includes('application/json')) {
                        const data = await response.json();
                        if (data.errors) {
                            this.error = data.errors.map(e => e.message).join(', ');
                            return;
                        }
                    }
                    this.error = `Failed to save settings (${response.status})`;
                    return;
                }

                // Finding 136: the save was reported as failed because the page
                // still showed the old value. The report blames the POST response
                // for "re-rendering the page with the OLD value"; it re-renders
                // nothing — it answers {"ok":true} to this fetch. What the reader
                // sees is the page as it was rendered *before* the save, and a
                // plugin's injected output (footer, header, sidebar, its own
                // pages) is server-rendered from these very settings. Only the
                // server can produce it, exactly as with the inline description
                // editor, so an explicit save reloads. The reload backs the
                // "Saved!" status with something stronger: the plugin's own output
                // changing in front of the reader.
                this.saved = true;
                // Final review of 2026-07-31, finding 4: this line used to be followed
                // immediately by window.location.reload(). reload() is asynchronous, so
                // Alpine's microtask-scheduled effect does run and the status text can
                // paint for a few hundred ms — the defect is not that nothing is
                // rendered. It is that the navigation has already begun in this same
                // task, so the live-region mutation is not reliably observed before the
                // document is torn down, and the announcement this code believes it
                // makes is effectively never delivered.
                //
                // The reload stays, for the reason above. The navigation is what waits.
                this._reloadTimer = setTimeout(() => {
                    this._reloadTimer = null;
                    window.location.reload();
                }, SAVED_ANNOUNCEMENT_MS);
            } catch (err) {
                this.error = err.message;
            }
        }
    };
}
