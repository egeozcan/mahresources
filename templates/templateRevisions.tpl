{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-8"
     x-data="templateRevisions({ carrierType: '{{ carrierType }}', id: {{ carrierID }}, apiPrefix: '{{ apiPrefix }}', editUrl: '{{ editUrl|escapejs }}' })"
     data-testid="template-revisions">
    <p class="flex flex-wrap gap-4 text-sm">
        <a href="{{ displayUrl }}" class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700 rounded focus:outline-hidden focus:ring-2 focus:ring-amber-600">Back to {{ carrierNoun|lower }} {{ carrierName }}</a>
        <a href="{{ editUrl }}" class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700 rounded focus:outline-hidden focus:ring-2 focus:ring-amber-600">Edit {{ carrierNoun|lower }}</a>
    </p>
    <p class="text-sm text-stone-600">Every save that changes a template, schema or section layout records a revision. Loading a revision fills the edit form without saving, so you can review it first.</p>

    <p id="label-edit-reason" class="sr-only">Save or cancel the open label first.</p>
    <p role="status" data-testid="revisions-status" class="text-sm text-stone-700 empty:hidden" x-text="status"></p>

    <section aria-labelledby="revisions-heading" class="space-y-3">
        <h2 id="revisions-heading" class="text-lg font-semibold font-mono text-stone-900">Revisions</h2>
        <p x-show="loading" class="text-sm text-stone-600">Loading revisions...</p>
        <p x-show="error" x-text="error" role="alert" data-testid="revisions-error" class="border-l-4 border-t-4 border-red-700 bg-red-50 p-3 text-sm text-red-900"></p>
        <p x-show="!loading && !error && revisions.length === 0" data-testid="revisions-empty" class="text-sm text-stone-600">No revisions yet. One is recorded the first time this {{ carrierNoun|lower }}'s templates change.</p>

        <div x-show="revisions.length > 0" class="overflow-x-auto" tabindex="0" role="region" aria-label="Revision list">
            <table class="w-full text-left text-sm" data-testid="revisions-table">
                <caption class="sr-only">Template revisions of {{ carrierName }}, newest first</caption>
                <thead>
                    <tr class="border-b border-stone-300 text-stone-700">
                        <th scope="col" class="px-2 py-2 font-semibold">Revision</th>
                        <th scope="col" class="px-2 py-2 font-semibold">Saved</th>
                        <th scope="col" class="px-2 py-2 font-semibold">User</th>
                        <th scope="col" class="px-2 py-2 font-semibold">Source</th>
                        <th scope="col" class="px-2 py-2 font-semibold">Label</th>
                        <th scope="col" class="px-2 py-2 font-semibold">Changed slots</th>
                        <th scope="col" class="px-2 py-2 font-semibold">Actions</th>
                    </tr>
                </thead>
                <tbody>
                    <template x-for="rev in revisions" :key="rev.number">
                        <tr class="border-b border-stone-200 align-top" :class="isCompared(rev) && 'bg-amber-50'" :data-testid="'revision-row-' + rev.number">
                            <th scope="row" class="px-2 py-2 font-mono font-semibold" x-text="'#' + rev.number"></th>
                            <td class="px-2 py-2"><time :datetime="rev.createdAt" x-text="when(rev)"></time></td>
                            <td class="px-2 py-2" x-text="rev.createdByUserId ? 'User ' + rev.createdByUserId : 'System'"></td>
                            <td class="px-2 py-2">
                                <span :data-testid="'revision-source-' + rev.number" x-text="sourceText(rev)"></span>
                                <span x-show="(rev.applied || []).length" class="block text-xs text-stone-600">
                                    Applied:
                                    <span :data-testid="'revision-applied-' + rev.number" x-text="(rev.applied || []).join(', ')"></span>
                                </span>
                            </td>
                            <td class="px-2 py-2">
                                <template x-if="editingLabel !== rev.number">
                                    <span :data-testid="'revision-label-' + rev.number" x-text="rev.label || ''"></span>
                                </template>
                                <template x-if="editingLabel === rev.number">
                                    <form class="flex flex-wrap items-center gap-2" @submit.prevent="saveLabel(rev)" @keydown.escape.prevent="cancelLabelEdit(rev.number)">
                                        <label class="sr-only" :for="'revision-label-input-' + rev.number" x-text="'Label for revision ' + rev.number"></label>
                                        <input type="text" x-model="labelDraft" :aria-invalid="labelTooLong.toString()" :aria-describedby="labelTooLong ? 'revision-label-error-' + rev.number : null"
                                               :id="'revision-label-input-' + rev.number" :data-label-input="rev.number"
                                               :data-testid="'revision-label-input-' + rev.number"
                                               :readonly="savingLabel" :aria-busy="savingLabel.toString()"
                                               class="w-48 rounded-md border-stone-300 text-sm focus:border-amber-500 focus:ring-amber-500">
                                        <p x-show="labelTooLong" :id="'revision-label-error-' + rev.number" role="alert" data-testid="revision-label-error" class="basis-full text-xs text-red-700">A label can be at most 200 characters.</p>
                                        <button type="submit" :disabled="savingLabel || labelTooLong" :data-testid="'revision-label-save-' + rev.number"
                                                class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 cursor-pointer">Save</button>
                                        <button type="button" @click="cancelLabelEdit(rev.number)"
                                                class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Cancel</button>
                                    </form>
                                </template>
                            </td>
                            <td class="px-2 py-2 text-xs font-mono" :data-testid="'revision-slots-' + rev.number" x-text="slotsText(rev)"></td>
                            <td class="px-2 py-2">
                                <div class="flex flex-wrap gap-2">
                                    <button type="button" @click="compareWithCurrent(rev.number)"
                                            :aria-label="'Compare revision ' + rev.number + ' with the current state'"
                                            :data-testid="'revision-compare-' + rev.number"
                                            class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Compare with current</button>
                                    <button type="button" @click="startLabelEdit(rev)" :disabled="savingLabel || (editingLabel !== null && editingLabel !== rev.number)"
                                            :aria-describedby="editingLabel !== null && editingLabel !== rev.number ? 'label-edit-reason' : null"
                                            :aria-label="'Edit label of revision ' + rev.number"
                                            :data-label-button="rev.number" :data-testid="'revision-label-edit-' + rev.number"
                                            class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 cursor-pointer">Edit label</button>
                                    <a :href="loadWholeUrl(rev.number)"
                                       :aria-label="'Load revision ' + rev.number + ' into the editor'"
                                       :data-testid="'revision-load-' + rev.number"
                                       class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600">Load into editor</a>
                                </div>
                            </td>
                        </tr>
                    </template>
                </tbody>
            </table>
        </div>
    </section>

    <section x-show="revisions.length > 0" aria-labelledby="revision-diff-heading" class="space-y-4" :aria-busy="diffLoading.toString()">
        <h2 id="revision-diff-heading" tabindex="-1" data-revision-diff-heading data-testid="revision-diff-heading"
            class="text-lg font-semibold font-mono text-stone-900 focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600 rounded"
            x-text="diff ? 'Changes from revision #' + diff.from + ' to ' + rightLabel().toLowerCase() : 'Compare revisions'"></h2>

        <fieldset class="flex flex-wrap items-end gap-4">
            <legend class="sr-only">Revisions to compare</legend>
            <div>
                <label for="revision-from" class="block text-sm font-mono font-medium text-stone-700 mb-1">From</label>
                <select id="revision-from" x-model.number="from" @change="onSelectionChange()" data-testid="revision-from"
                        class="rounded-md border-stone-300 text-sm font-mono focus:border-amber-500 focus:ring-amber-500">
                    <option value="">Choose a revision</option>
                    <template x-for="rev in revisions" :key="rev.number">
                        <option :value="rev.number" x-text="'#' + rev.number + (rev.label ? ' - ' + rev.label : '')"></option>
                    </template>
                </select>
            </div>
            <div>
                <label for="revision-to" class="block text-sm font-mono font-medium text-stone-700 mb-1">To</label>
                <select id="revision-to" x-model.number="to" @change="onSelectionChange()" data-testid="revision-to"
                        class="rounded-md border-stone-300 text-sm font-mono focus:border-amber-500 focus:ring-amber-500">
                    <option value="">Current state</option>
                    <template x-for="rev in revisions" :key="rev.number">
                        <option :value="rev.number" x-text="'#' + rev.number + (rev.label ? ' - ' + rev.label : '')"></option>
                    </template>
                </select>
            </div>
            <div role="radiogroup" aria-label="Diff layout" class="flex items-center gap-4 text-sm">
                <label class="inline-flex items-center gap-1"><input type="radio" value="unified" x-model="layout" data-testid="revision-layout-unified" class="text-amber-700 focus:ring-amber-600"> Unified</label>
                <label class="inline-flex items-center gap-1"><input type="radio" value="side" x-model="layout" data-testid="revision-layout-side" class="text-amber-700 focus:ring-amber-600"> Side by side</label>
            </div>
        </fieldset>

        <p x-show="diffError" x-text="diffError" role="alert" class="border-l-4 border-t-4 border-red-700 bg-red-50 p-3 text-sm text-red-900"></p>
        <p x-show="!from" class="text-sm text-stone-600">Choose a revision above, or press Compare with current on a row. Slots that did not change are hidden.</p>
        <p x-show="diff && diff.slots.length === 0" data-testid="revision-diff-empty" class="text-sm text-stone-600">These two states have no differences.</p>

        <template x-if="diff">
            <div class="space-y-6" data-testid="revision-diff">
                <template x-for="slot in diff.slots" :key="slot.slot">
                    <section class="border-l-4 border-t-4 border-amber-700 bg-white p-4 shadow-sm space-y-3" :aria-label="'Changes in ' + slot.slot" :data-testid="'revision-diff-slot-' + slot.slot">
                        <div class="flex flex-wrap items-center justify-between gap-3">
                            <h3 class="font-mono font-semibold text-stone-900" x-text="slot.slot"></h3>
                            <div class="flex flex-wrap items-center gap-3 text-sm">
                                <label x-show="slot.hasCss" class="inline-flex items-center gap-1 text-stone-700">
                                    <input type="checkbox" :checked="cssIncluded(slot.slot)" @change="setCssIncluded(slot.slot, $event.target.checked)"
                                           :data-testid="'revision-slot-css-' + slot.slot"
                                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                                    Include its CSS
                                </label>
                                <a :href="loadSlotUrl(diff.from, slot.slot, slot.hasCss)" :data-testid="'revision-slot-load-from-' + slot.slot"
                                   :aria-label="'Load ' + slot.slot + ' from revision ' + diff.from + ' into the editor'"
                                   class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600"
                                   x-text="'Load from #' + diff.from"></a>
                                <a x-show="diff.to" :href="loadSlotUrl(diff.to || 0, slot.slot, slot.hasCss)" :data-testid="'revision-slot-load-to-' + slot.slot"
                                   :aria-label="'Load ' + slot.slot + ' from revision ' + diff.to + ' into the editor'"
                                   class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600"
                                   x-text="'Load from #' + diff.to"></a>
                            </div>
                        </div>
                        <template x-for="field in slot.fields" :key="field.field">
                            <div class="space-y-1">
                                <h4 x-show="field.field !== slot.slot" class="text-sm font-mono font-medium text-stone-700" x-text="field.field"></h4>
                                <template x-if="layout === 'unified'">
                                    <div class="overflow-x-auto rounded border border-stone-200 font-mono text-xs" tabindex="0" role="region"
                                         :aria-label="'Unified diff of ' + field.field" :data-testid="'revision-unified-' + field.field">
                                        <template x-for="(row, i) in field.unified" :key="i">
                                            <div>
                                                <template x-if="row.gap">
                                                    <div class="bg-stone-100 px-2 py-0.5 italic text-stone-700" x-text="gapText(row)"></div>
                                                </template>
                                                <template x-if="!row.gap">
                                                    <div class="flex px-2 py-px" :class="rowClass(row.op)" :data-op="row.op">
                                                        <span aria-hidden="true" class="w-4 shrink-0 select-none whitespace-pre" x-text="marker(row.op)"></span>
                                                        <span class="sr-only" x-text="srText(row.op)"></span>
                                                        <span class="whitespace-pre-wrap break-all min-h-[1em]" x-text="row.text"></span>
                                                    </div>
                                                </template>
                                            </div>
                                        </template>
                                    </div>
                                </template>
                                <template x-if="layout === 'side'">
                                    <div class="overflow-x-auto rounded border border-stone-200" tabindex="0" role="region" :aria-label="'Side-by-side diff of ' + field.field">
                                        <table class="w-full table-fixed font-mono text-xs" :data-testid="'revision-side-' + field.field">
                                            <caption class="sr-only" x-text="'Side-by-side diff of ' + field.field"></caption>
                                            <thead>
                                                <tr class="bg-stone-100 text-left text-stone-700">
                                                    <th scope="col" class="w-1/2 px-2 py-1 font-semibold" x-text="'Revision #' + diff.from"></th>
                                                    <th scope="col" class="w-1/2 px-2 py-1 font-semibold" x-text="rightLabel()"></th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                <template x-for="(row, i) in field.sideBySide" :key="i">
                                                    <tr class="align-top">
                                                        <template x-if="row.gap">
                                                            <td colspan="2" class="bg-stone-100 px-2 py-0.5 italic text-stone-700" x-text="gapText(row)"></td>
                                                        </template>
                                                        <template x-if="!row.gap">
                                                            <td class="px-2 py-px" :class="row.left ? rowClass(row.left.op) : 'bg-stone-50'">
                                                                <span aria-hidden="true" class="select-none whitespace-pre" x-text="row.left ? marker(row.left.op) : ''"></span>
                                                                <span class="sr-only" x-text="row.left ? srText(row.left.op) : ''"></span>
                                                                <span class="whitespace-pre-wrap break-all" x-text="row.left ? row.left.text : ''"></span>
                                                            </td>
                                                        </template>
                                                        <template x-if="!row.gap">
                                                            <td class="px-2 py-px" :class="row.right ? rowClass(row.right.op) : 'bg-stone-50'">
                                                                <span aria-hidden="true" class="select-none whitespace-pre" x-text="row.right ? marker(row.right.op) : ''"></span>
                                                                <span class="sr-only" x-text="row.right ? srText(row.right.op) : ''"></span>
                                                                <span class="whitespace-pre-wrap break-all" x-text="row.right ? row.right.text : ''"></span>
                                                            </td>
                                                        </template>
                                                    </tr>
                                                </template>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                            </div>
                        </template>
                    </section>
                </template>
            </div>
        </template>
    </section>
</div>
{% endblock %}
