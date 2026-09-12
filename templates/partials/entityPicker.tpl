{# One shared dialog; retained filter steps are hidden, never replaced beneath a child. #}
<div x-data x-show="$store.entityPicker.isOpen" x-cloak
     class="fixed inset-0 overflow-y-auto entity-picker-overlay-top"
     role="dialog" aria-modal="true" aria-labelledby="entity-picker-title" @keydown.stop @keyup.stop
     @keydown.escape.window.capture="if ($store.entityPicker.isOpen) { $event.preventDefault(); $event.stopImmediatePropagation(); $store.entityPicker.escape(); }">
    <div class="fixed inset-0 bg-black/50" aria-hidden="true" @click="$store.entityPicker.close()"></div>
    <div class="flex min-h-full items-center justify-center p-3">
        <div class="relative bg-white rounded-lg shadow-xl w-full max-w-5xl max-h-[90vh] flex flex-col"
             @click.stop x-trap.noscroll.noreturn="$store.entityPicker.isOpen">
            <div class="flex items-center gap-3 px-4 py-3 border-b border-stone-200">
                <button type="button" x-show="$store.entityPicker.steps.length > 1" @click="$store.entityPicker.back()" class="text-sm underline">Back</button>
                <h2 id="entity-picker-title" class="text-lg font-semibold text-stone-900 flex-1" x-text="$store.entityPicker.title"></h2>
                <button type="button" @click="$store.entityPicker.close()" aria-label="Close" class="w-9 h-9 text-stone-700 text-xl">×</button>
            </div>
            <div class="overflow-y-auto min-h-0 p-4 space-y-4">
                <template x-for="view in $store.entityPicker.views" :key="view.id">
                    <section :data-picker-step="view.id" :hidden="view.id !== $store.entityPicker.currentStep?.id"
                             :inert="view.id !== $store.entityPicker.currentStep?.id">
                        <template x-if="view.legacy && view.entity === 'resource'">
                            <div class="flex gap-2 mb-3" role="group" aria-label="Resource source">
                                <button type="button" :disabled="!view.legacy.noteId || $store.entityPicker.confirming" :aria-pressed="$store.entityPicker.activeTab === 'note'"
                                        @click="$store.entityPicker.setActiveTab('note')" class="border px-3 py-2 rounded text-sm">Note's Resources</button>
                                <button type="button" :disabled="$store.entityPicker.confirming" :aria-pressed="$store.entityPicker.activeTab === 'all'"
                                        @click="$store.entityPicker.setActiveTab('all')" class="border px-3 py-2 rounded text-sm">All Resources</button>
                            </div>
                        </template>
                        {% include "/partials/entityPickerFilters.tpl" %}
                    </section>
                </template>
                <div role="status" aria-live="polite" class="text-sm text-stone-600">
                    <span x-show="$store.entityPicker.loading">Loading results…</span>
                    <span x-show="!$store.entityPicker.loading" x-text="($store.entityPicker.currentStep?.items.length || 0) + ' results · Page ' + ($store.entityPicker.currentStep?.page || 1)"></span>
                </div>
                <div x-show="$store.entityPicker.error" role="alert" class="text-sm text-red-800">
                    <p x-text="$store.entityPicker.error"></p>
                    <button type="button" @click="$store.entityPicker.retry()" class="underline">Retry</button>
                </div>
                <template x-for="(warning, index) in [...new Set($store.entityPicker.currentStep?.warnings || [])]" :key="index">
                    <p class="text-sm text-amber-800" x-text="warning"></p>
                </template>
                <p x-show="$store.entityPicker.currentStep?.status === 'ready' && !$store.entityPicker.displayResults.length" class="text-sm text-stone-600">No results match these filters.</p>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3" :aria-busy="$store.entityPicker.loading">
                    <template x-for="item in $store.entityPicker.displayResults" :key="$store.entityPicker.currentStep.id + ':' + item.value.ID">
                        <div :data-picker-id="item.value.ID" class="flex items-start gap-3 border rounded p-3 min-w-0"
                             :class="$store.entityPicker.isSelected(item.value.ID) ? 'border-amber-700 bg-amber-50' : 'border-stone-200'"
                             @click="if (!$event.target.closest('a,button,input,select,textarea,label') && !$store.entityPicker.rowDisabled(item.value.ID)) $store.entityPicker.toggleSelection(item.value)">
                            <input :type="$store.entityPicker.multiSelect ? 'checkbox' : 'radio'" name="entity-picker-choice"
                                   :aria-label="'Select ' + item.value.Name"
                                   :checked="$store.entityPicker.isSelected(item.value.ID) || (($store.entityPicker.multiSelect || !$store.entityPicker.selectionCount) && $store.entityPicker.isAlreadyAdded(item.value.ID))"
                                   :disabled="$store.entityPicker.rowDisabled(item.value.ID)"
                                   @change="$store.entityPicker.toggleSelection(item.value)" class="mt-1 flex-shrink-0 text-amber-700 focus:ring-amber-700">
                            <div class="min-w-0 flex-1">
                                <div x-effect="$store.entityPicker.renderResult($el, item.html)"><div x-ignore class="entity-picker-result"></div></div>
                                <span x-show="$store.entityPicker.isAlreadyAdded(item.value.ID)" class="text-xs text-stone-600">Already selected</span>
                            </div>
                        </div>
                    </template>
                </div>
                <nav aria-label="Result pages" class="flex items-center justify-between gap-3">
                    <button type="button" @click="$store.entityPicker.previousPage()" :disabled="($store.entityPicker.currentStep?.page || 1) <= 1 || $store.entityPicker.loading || $store.entityPicker.confirming"
                            class="border rounded px-3 py-2 text-sm disabled:opacity-50" aria-label="Previous page">Previous</button>
                    <span class="text-sm" x-text="'Page ' + ($store.entityPicker.currentStep?.page || 1)"></span>
                    <button type="button" @click="$store.entityPicker.nextPage()" :disabled="!$store.entityPicker.currentStep?.hasNext || $store.entityPicker.loading || $store.entityPicker.confirming"
                            class="border rounded px-3 py-2 text-sm disabled:opacity-50" aria-label="Next page">Next</button>
                </nav>
            </div>
            <div class="border-t border-stone-200 px-4 py-3 space-y-2">
                <div class="flex flex-wrap gap-2 max-h-20 overflow-y-auto">
                    <template x-for="value in $store.entityPicker.currentStep?.pending || []" :key="value.ID">
                        <button type="button" @click="$store.entityPicker.toggleSelection(value)" :disabled="$store.entityPicker.confirming"
                                :aria-label="'Remove pending ' + value.Name" class="text-xs bg-stone-100 rounded px-2 py-1"><span x-text="value.Name"></span> <span aria-hidden="true">×</span></button>
                    </template>
                </div>
                <p x-show="$store.entityPicker.capacityReached" class="text-sm text-stone-600">Selection limit reached. Remove a pending choice to add another.</p>
                <div class="flex items-center justify-between gap-3">
                    <span role="status" class="text-sm text-stone-600" x-text="$store.entityPicker.selectionCount + ' pending'"></span>
                    <div class="flex gap-2">
                        <button type="button" @click="$store.entityPicker.close()" class="border rounded px-3 py-2 text-sm">Cancel</button>
                        <button type="button" @click="$store.entityPicker.confirm()" :disabled="!$store.entityPicker.selectionCount || $store.entityPicker.confirming"
                                class="bg-amber-700 text-white rounded px-3 py-2 text-sm disabled:opacity-50" aria-label="Confirm selection"
                                x-text="$store.entityPicker.confirming ? 'Checking…' : 'Confirm'"></button>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
