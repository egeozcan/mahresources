import { focusedElement } from '../utils/focus.js';

export function pluginActionModal() {
    return {
        isOpen: false,
        action: null,
        formValues: {},
        errors: {},
        submitting: false,
        result: null,
        // The control the reader pressed to get here. Captured at open, because by
        // the time this modal hands over to the jobs panel its own Run button has
        // focus and is about to be removed by the x-if — see submit().
        _opener: null,
        // Which submission the modal is currently showing. A run is an await away from
        // its own continuation, and the modal is reusable in the meantime.
        _submitSeq: 0,

        init() {
            window.addEventListener('plugin-action-open', (e) => this.open(e.detail));
        },

        open(detail) {
            // `returnFocusTo` when the caller knows better than focus does. A card
            // action menu closes itself before dispatching, so its menu item is still
            // connected but `display:none` — restoring to it silently fails and the
            // reader is dropped somewhere else entirely. The menu's own trigger button
            // is still on screen, and is where they came from.
            this._opener = detail?.returnFocusTo ?? focusedElement();
            // Any request still in flight now belongs to a modal that no longer exists.
            this._submitSeq++;
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
                // For single-entity placement entityIds has one element; for bulk it has all
                // selected IDs. Spreading the full array is intentional — the caller limits
                // to ids[0] for multi=false params below.
                ids.push(...action.entityIds.map(Number));
            }
            if ((def === 'selection' || def === 'both') && compatible) {
                const sel = window.Alpine?.store('bulkSelection');
                if (sel && sel.selectedIds) {
                    for (const id of sel.selectedIds) {
                        const numId = Number(id);
                        if (!ids.includes(numId)) ids.push(numId);
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
            // The run this continuation speaks for. Cancel stays enabled while a
            // request is in flight, so the reader can close this modal and open another
            // action before the first answer arrives — and the first answer then
            // called close() on the *second* modal, discarding a half-filled form and
            // opening the jobs panel for a run the reader had abandoned.
            const seq = ++this._submitSeq;
            const superseded = () => seq !== this._submitSeq;
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

                if (superseded()) return;

                if (!resp.ok) {
                    this.errors._general = data.error || 'Action failed';
                    return;
                }

                if (data.job_id || data.job_ids) {
                    // The opener travels with the request to open the panel. Closing
                    // this modal and asking for the panel in the same tick leaves the
                    // panel reading `document.activeElement`, which is this modal's Run
                    // button — still connected for one more tick, then torn down by the
                    // x-if. The panel's restore rejects a detached node and falls back
                    // to its own header trigger, so the reader ended a plugin action
                    // somewhere they had never been.
                    const opener = this._opener;
                    this.close();
                    // Asked for after this modal has actually gone, not in the same
                    // tick as close(). The panel declines to open underneath an open
                    // dialog — two aria-modal dialogs at once is the defect either way
                    // round — and until Alpine flushes, the x-if still has this one
                    // mounted, so a synchronous request would be refused by the guard
                    // that exists to protect the reader from it.
                    this.$nextTick(() => {
                        window.dispatchEvent(new CustomEvent('jobs-panel-open', {
                            detail: { returnFocusTo: opener },
                        }));
                    });
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
                if (superseded()) return;
                this.errors._general = err.message;
            } finally {
                // Not unconditionally: clearing it from a superseded run would unlock
                // the Run button of a submission that is still in flight.
                if (!superseded()) this.submitting = false;
            }
        },
    };
}
