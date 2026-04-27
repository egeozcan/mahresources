<div x-data="pluginActionModal()" x-cloak>
    <template x-if="isOpen">
        <div class="plugin-action-overlay" @click.self="close()" @keydown.escape.window="isOpen && close()">
            <div class="plugin-action-modal" role="dialog" aria-modal="true" aria-labelledby="plugin-action-modal-title" x-trap.noscroll="isOpen && !$store.entityPicker.isOpen">
                <header class="plugin-action-modal-header">
                    <h3 x-text="action?.label" id="plugin-action-modal-title" class="plugin-action-modal-title"></h3>
                    <button @click="close()" class="plugin-action-modal-close" aria-label="Close">&times;</button>
                </header>

                <template x-if="action?.description">
                    <p class="plugin-action-modal-desc" x-text="action.description"></p>
                </template>

                <template x-if="action?.confirm && !result">
                    <p class="plugin-action-modal-confirm" x-text="action.confirm"></p>
                </template>

                <template x-if="result">
                    <div class="plugin-action-modal-result" role="status">
                        <p x-text="result.message || 'Action completed successfully'"></p>
                    </div>
                </template>

                <template x-if="!result">
                    <form @submit.prevent="submit()">
                        <template x-if="errors._general">
                            <div class="plugin-action-modal-error" role="alert" x-text="errors._general"></div>
                        </template>

                        <template x-for="param in (action?.params || [])" :key="param.name">
                            <div class="plugin-action-modal-field" x-show="isParamVisible(param)" :class="{'plugin-action-modal-field--info': param.type === 'info'}">
                                <template x-if="param.type === 'info'">
                                    <div class="plugin-action-modal-info" role="note">
                                        <span x-show="param.label" class="plugin-action-modal-info-title" x-text="param.label"></span>
                                        <span class="plugin-action-modal-info-body" x-text="param.description"></span>
                                    </div>
                                </template>

                                <template x-if="param.type !== 'info'">
                                    <label :for="'plugin-param-' + param.name" class="plugin-action-modal-label">
                                        <span x-text="param.label"></span>
                                        <span x-show="param.required" class="plugin-action-modal-required" aria-hidden="true">*</span>
                                    </label>
                                </template>

                                <template x-if="param.type === 'text' || param.type === 'hidden'">
                                    <input :type="param.type" :id="'plugin-param-' + param.name"
                                           x-model="formValues[param.name]"
                                           :required="param.required"
                                           class="plugin-action-modal-input"
                                           :aria-invalid="errors[param.name] ? 'true' : null"
                                           :aria-describedby="errors[param.name] ? 'plugin-param-error-' + param.name : null">
                                </template>

                                <template x-if="param.type === 'textarea'">
                                    <textarea :id="'plugin-param-' + param.name"
                                              x-model="formValues[param.name]"
                                              :required="param.required"
                                              rows="3"
                                              class="plugin-action-modal-textarea"
                                              :aria-invalid="errors[param.name] ? 'true' : null"
                                              :aria-describedby="errors[param.name] ? 'plugin-param-error-' + param.name : null"></textarea>
                                </template>

                                <template x-if="param.type === 'number'">
                                    <input type="number" :id="'plugin-param-' + param.name"
                                           x-model.number="formValues[param.name]"
                                           :required="param.required"
                                           :min="param.min" :max="param.max" :step="param.step"
                                           class="plugin-action-modal-input"
                                           :aria-invalid="errors[param.name] ? 'true' : null"
                                           :aria-describedby="errors[param.name] ? 'plugin-param-error-' + param.name : null">
                                </template>

                                <template x-if="param.type === 'select'">
                                    <select :id="'plugin-param-' + param.name"
                                            x-model="formValues[param.name]"
                                            :required="param.required"
                                            class="plugin-action-modal-select"
                                            :aria-invalid="errors[param.name] ? 'true' : null"
                                            :aria-describedby="errors[param.name] ? 'plugin-param-error-' + param.name : null">
                                        <template x-if="!param.default">
                                            <option value="" disabled selected>-- Select --</option>
                                        </template>
                                        <template x-for="opt in (param.options || [])" :key="opt">
                                            <option :value="opt" x-text="opt"></option>
                                        </template>
                                    </select>
                                </template>

                                <template x-if="param.type === 'boolean'">
                                    <div class="plugin-action-modal-checkbox-wrap">
                                        <input type="checkbox" :id="'plugin-param-' + param.name"
                                               x-model="formValues[param.name]"
                                               class="plugin-action-modal-checkbox">
                                    </div>
                                </template>

                                <template x-if="param.type === 'entity_ref'">
                                    <div class="plugin-action-modal-entityref">
                                        <template x-if="effectiveFilters(param) && (effectiveFilters(param).content_types || effectiveFilters(param).category_ids || effectiveFilters(param).note_type_ids)">
                                            <div class="plugin-action-modal-entityref-filter-badge text-xs text-stone-500 mb-1">
                                                <template x-if="effectiveFilters(param).content_types">
                                                    <span>Showing only: <span x-text="effectiveFilters(param).content_types.join(', ')"></span></span>
                                                </template>
                                                <template x-if="effectiveFilters(param).category_ids && effectiveFilters(param).category_ids.length">
                                                    <span>Filtered by category</span>
                                                </template>
                                                <template x-if="effectiveFilters(param).note_type_ids && effectiveFilters(param).note_type_ids.length">
                                                    <span>Filtered by note type</span>
                                                </template>
                                            </div>
                                        </template>
                                        <div class="plugin-action-modal-entityref-chips flex flex-wrap gap-2 mb-2">
                                            <template x-for="id in (param.multi ? (formValues[param.name] || []) : (formValues[param.name] != null ? [formValues[param.name]] : []))" :key="id">
                                                <span class="inline-flex items-center gap-1 px-2 py-1 bg-stone-100 rounded text-sm">
                                                    <span x-text="'#' + id"></span>
                                                    <button type="button" @click="removeEntityRefId(param, id)" :aria-label="'Remove #' + id" class="text-stone-500 hover:text-stone-900">&times;</button>
                                                </span>
                                            </template>
                                        </div>
                                        <button type="button" @click="openPickerFor(param)"
                                                :aria-describedby="errors[param.name] ? 'plugin-param-error-' + param.name : null"
                                                class="btn btn-secondary text-sm">
                                            <span x-text="'Add ' + (param.entity === 'resource' ? 'resources' : param.entity === 'note' ? 'notes' : 'groups')"></span>
                                        </button>
                                    </div>
                                </template>

                                <template x-if="param.description && param.type !== 'info'">
                                    <span class="plugin-action-modal-help" x-text="param.description"></span>
                                </template>

                                <template x-if="errors[param.name]">
                                    <span class="plugin-action-modal-field-error" :id="'plugin-param-error-' + param.name" role="alert" x-text="errors[param.name]"></span>
                                </template>
                            </div>
                        </template>

                        <div class="plugin-action-modal-actions">
                            <button type="button" @click="close()" class="btn btn-secondary">Cancel</button>
                            <button type="submit" :disabled="submitting" class="btn btn-primary">
                                <span x-show="!submitting">Run</span>
                                <span x-show="submitting">Running...</span>
                            </button>
                        </div>
                    </form>
                </template>
            </div>
        </div>
    </template>
</div>
