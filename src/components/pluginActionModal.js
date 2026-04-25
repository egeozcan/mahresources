export function pluginActionModal() {
    return {
        isOpen: false,
        action: null,
        formValues: {},
        errors: {},
        submitting: false,
        result: null,

        init() {
            window.addEventListener('plugin-action-open', (e) => this.open(e.detail));
        },

        open(detail) {
            const action = {
                plugin: detail.plugin,
                action: detail.action,
                label: detail.label,
                description: detail.description,
                entityIds: detail.entityIds,
                entityType: detail.entityType,
                async: detail.async,
                params: detail.params,
                confirm: detail.confirm,
            };
            this.action = action;
            this.errors = {};
            this.result = null;
            this.submitting = false;
            this.formValues = {};
            if (action.params) {
                for (const param of action.params) {
                    this.formValues[param.name] = param.default ?? (param.type === 'boolean' ? false : '');
                }
            }
            this.isOpen = true;
            // Focus trap: focus the first input after render
            this.$nextTick(() => {
                const firstInput = this.$root.querySelector('input, textarea, select');
                if (firstInput) firstInput.focus();
            });
        },

        close() {
            this.isOpen = false;
            this.action = null;
        },

        isParamVisible(param) {
            if (!param.show_when) return true;
            for (const key of Object.keys(param.show_when)) {
                if (this.formValues[key] !== param.show_when[key]) return false;
            }
            return true;
        },

        async submit() {
            if (this.submitting) return;

            this.errors = {};
            if (this.action.params) {
                for (const param of this.action.params) {
                    if (!this.isParamVisible(param)) continue;
                    if (param.required && !this.formValues[param.name] && this.formValues[param.name] !== 0 && this.formValues[param.name] !== false) {
                        this.errors[param.name] = `${param.label} is required`;
                    }
                }
            }
            if (Object.keys(this.errors).length > 0) return;

            this.submitting = true;
            try {
                const resp = await fetch('/v1/jobs/action/run', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        plugin: this.action.plugin,
                        action: this.action.action,
                        entity_ids: this.action.entityIds.map(Number),
                        params: this.formValues,
                    }),
                });

                const data = await resp.json();

                if (!resp.ok) {
                    this.errors._general = data.error || 'Action failed';
                    return;
                }

                if (data.job_id || data.job_ids) {
                    this.close();
                    window.dispatchEvent(new CustomEvent('jobs-panel-open'));
                } else if (data.redirect) {
                    window.location.href = data.redirect;
                } else {
                    this.result = data;
                    setTimeout(() => {
                        this.close();
                        window.location.reload();
                    }, 1500);
                }
            } catch (err) {
                this.errors._general = err.message;
            } finally {
                this.submitting = false;
            }
        },
    };
}
