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
                filters: detail.filters || {},
                bulk_max: detail.bulk_max,
            };
            this.action = action;
            this.errors = {};
            this.result = null;
            this.submitting = false;
            this.formValues = {};
            if (action.params) {
                for (const param of action.params) {
                    // 'info' params render static help text and have no input value.
                    if (param.type === 'info') continue;
                    if (param.type === 'entity_ref') {
                        this.formValues[param.name] = this.resolveEntityRefDefault(param, action);
                        continue;
                    }
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

        resolveEntityRefDefault(param, action) {
            const def = param.default ?? 'trigger';
            const compatible = param.entity === action.entityType;
            let ids = [];
            if ((def === 'trigger' || def === 'both') && compatible && action.entityIds) {
                ids.push(...action.entityIds.map(Number));
            }
            if ((def === 'selection' || def === 'both') && compatible) {
                const sel = window.Alpine?.store('bulkSelection');
                if (sel && sel.selectedIds) {
                    for (const id of sel.selectedIds) {
                        if (!ids.includes(id)) ids.push(id);
                    }
                }
            }
            if (param.multi) return ids;
            return ids.length > 0 ? ids[0] : null;
        },

        effectiveFilters(param) {
            return param.filters || this.action?.filters || {};
        },

        openPickerFor(param) {
            const existing = param.multi
                ? (this.formValues[param.name] || [])
                : (this.formValues[param.name] != null ? [this.formValues[param.name]] : []);
            const self = this;
            Alpine.store('entityPicker').open({
                entityType: param.entity,
                existingIds: existing,
                lockedFilters: self.effectiveFilters(param),
                multiSelect: param.multi,
                onConfirm: (ids) => {
                    if (param.multi) {
                        const seen = new Set(self.formValues[param.name] || []);
                        const next = [...(self.formValues[param.name] || [])];
                        for (const id of ids) {
                            if (!seen.has(id)) { seen.add(id); next.push(id); }
                        }
                        self.formValues[param.name] = next;
                    } else {
                        self.formValues[param.name] = ids.length > 0 ? ids[0] : null;
                    }
                },
            });
        },

        removeEntityRefId(param, id) {
            if (param.multi) {
                this.formValues[param.name] = (this.formValues[param.name] || []).filter(x => x !== id);
            } else {
                this.formValues[param.name] = null;
            }
        },

        close() {
            this.isOpen = false;
            this.action = null;
        },

        isParamVisible(param) {
            if (!param.show_when) return true;
            for (const key of Object.keys(param.show_when)) {
                const expected = param.show_when[key];
                const actual = this.formValues[key];
                if (Array.isArray(expected)) {
                    if (!expected.includes(actual)) return false;
                } else {
                    if (actual !== expected) return false;
                }
            }
            return true;
        },

        async submit() {
            if (this.submitting) return;

            this.errors = {};
            if (this.action.params) {
                for (const param of this.action.params) {
                    if (param.type === 'info') continue;
                    if (!this.isParamVisible(param)) continue;
                    const val = this.formValues[param.name];
                    if (param.type === 'entity_ref') {
                        const count = param.multi ? (val || []).length : (val != null ? 1 : 0);
                        if (param.required && count === 0) {
                            this.errors[param.name] = `${param.label} is required`;
                        } else if (param.multi && param.min != null && count < param.min) {
                            this.errors[param.name] = `${param.label}: at least ${param.min} required`;
                        } else if (param.multi && param.max != null && param.max > 0 && count > param.max) {
                            this.errors[param.name] = `${param.label}: at most ${param.max} allowed`;
                        }
                        continue;
                    }
                    if (param.required && !val && val !== 0 && val !== false) {
                        this.errors[param.name] = `${param.label} is required`;
                    }
                }
            }
            if (Object.keys(this.errors).length > 0) return;

            // Strip values for params that are currently hidden by show_when so
            // a stale default (e.g. an aspect_ratio left at "4:3" after toggling
            // enhance_resolution off) doesn't leak into the request body.
            const visibleParams = {};
            if (this.action.params) {
                for (const param of this.action.params) {
                    if (param.type === 'info') continue;
                    if (!this.isParamVisible(param)) continue;
                    visibleParams[param.name] = this.formValues[param.name];
                }
            } else {
                Object.assign(visibleParams, this.formValues);
            }

            this.submitting = true;
            try {
                const resp = await fetch('/v1/jobs/action/run', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        plugin: this.action.plugin,
                        action: this.action.action,
                        entity_ids: this.action.entityIds.map(Number),
                        params: visibleParams,
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
