export function pluginSettings(pluginName) {
    return {
        pluginName,
        saved: false,
        error: '',

        async saveSettings(event) {
            this.saved = false;
            this.error = '';

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
                    const data = await response.json();
                    if (data.errors) {
                        this.error = data.errors.map(e => e.message).join(', ');
                    } else {
                        this.error = 'Failed to save settings';
                    }
                    return;
                }

                this.saved = true;
                setTimeout(() => { this.saved = false; }, 3000);
            } catch (err) {
                this.error = err.message;
            }
        }
    };
}
