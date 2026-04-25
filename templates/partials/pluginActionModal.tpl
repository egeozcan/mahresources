<div x-data="pluginActionModal()" x-cloak>
    <template x-if="isOpen">
        <div class="plugin-action-overlay" @click.self="close()" @keydown.escape.window="isOpen && close()">
            <div class="plugin-action-modal" role="dialog" aria-modal="true" aria-labelledby="plugin-action-modal-title" x-trap.noscroll="isOpen">
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
                            <div class="plugin-action-modal-field" x-show="isParamVisible(param)">
                                <label :for="'plugin-param-' + param.name" class="plugin-action-modal-label">
                                    <span x-text="param.label"></span>
                                    <span x-show="param.required" class="plugin-action-modal-required" aria-hidden="true">*</span>
                                </label>

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
