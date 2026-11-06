{# The slot History dialog (src/components/slotHistory.js), included once per edit form. tpApiPrefix, tpCarrierId, tpEntityType, tpPreviewPath come from the enclosing with. #}
{% if tpCarrierId %}
<div x-data="slotHistory({ apiPrefix: '{{ tpApiPrefix }}', carrierId: {{ tpCarrierId }}, entityType: '{{ tpEntityType }}', previewPath: '{{ tpPreviewPath }}' })">
    {# x-if must stay the outer template: with x-teleport outside, the subtree never registers. #}
    <template x-if="isOpen">
        <template x-teleport=".overlays">
            <div class="slot-history-overlay fixed inset-0 z-50 flex items-start sm:items-center justify-center bg-black/50 p-2 sm:p-4"
                 @keydown.escape.prevent.stop="close()">
                <div x-init="mount($el)" role="dialog" aria-modal="true" aria-labelledby="slot-history-title"
                     data-testid="slot-history-dialog"
                     class="relative flex max-h-[95vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl bg-white shadow-2xl ring-1 ring-black/5">
                    <div class="flex items-center justify-between gap-3 border-b border-stone-200 bg-stone-50 px-4 py-3">
                        <h2 id="slot-history-title" tabindex="-1" data-slot-history-heading
                            class="font-mono text-lg font-semibold text-stone-900 rounded focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"
                            x-text="'History of ' + label"></h2>
                        <button type="button" @click="close()"
                                class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-100 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Close</button>
                    </div>

                    <div class="grid min-h-0 flex-1 grid-cols-1 overflow-y-auto md:grid-cols-[16rem_minmax(0,1fr)]">
                        <fieldset class="border-b border-stone-200 p-4 md:border-b-0 md:border-r space-y-2 min-w-0">
                            <legend class="sr-only">Revisions that changed this slot</legend>
                            <p class="text-xs text-stone-600" aria-hidden="true">Revisions that changed this slot, newest first.</p>
                            <p x-show="loading" data-testid="slot-history-loading" class="text-sm text-stone-600">Loading revisions…</p>
                            <p x-show="error" x-text="error" role="alert" class="border-l-4 border-t-4 border-red-700 bg-red-50 p-2 text-sm text-red-900"></p>
                            <p x-show="!loading && !error && revisions.length === 0" class="text-sm text-stone-600">This slot has not changed since history began.</p>
                            <template x-for="rev in revisions" :key="rev.number">
                                <label class="flex items-start gap-2 rounded px-2 py-1.5 text-sm cursor-pointer hover:bg-amber-50"
                                       :class="selected === rev.number && 'bg-amber-50'">
                                    <input type="radio" name="slot-history-revision" :value="rev.number"
                                           :checked="selected === rev.number" @change="select(rev.number)"
                                           :data-testid="'slot-history-rev-' + rev.number"
                                           class="mt-0.5 text-amber-700 focus:ring-amber-600">
                                    <span class="min-w-0">
                                        <span class="font-mono font-semibold" x-text="'#' + rev.number"></span>
                                        <span class="text-stone-700" x-text="sourceText(rev)"></span>
                                        <span class="block text-xs text-stone-600"><time :datetime="rev.createdAt" x-text="when(rev)"></time></span>
                                        <span x-show="rev.label" class="block text-xs text-stone-800 break-words" x-text="rev.label"></span>
                                    </span>
                                </label>
                            </template>
                        </fieldset>

                        <div class="min-w-0 space-y-4 p-4" :aria-busy="detailLoading.toString()">
                            <p x-show="!loading && revisions.length && selected === null" class="text-sm text-stone-600">Choose a revision to compare it with the editor and preview it.</p>
                            <p x-show="detailLoading" class="text-sm text-stone-600">Loading the revision…</p>
                            <p x-show="detailError" x-text="detailError" role="alert" class="border-l-4 border-t-4 border-red-700 bg-red-50 p-2 text-sm text-red-900"></p>

                            <template x-if="detail">
                                <div class="space-y-4">
                                    <div class="flex flex-wrap items-center gap-4 text-sm">
                                        <div role="radiogroup" aria-label="Diff layout" class="flex items-center gap-4">
                                            <label class="inline-flex items-center gap-1"><input type="radio" name="slot-history-layout" value="unified" x-model="layout" data-testid="slot-history-layout-unified" class="text-amber-700 focus:ring-amber-600"> Unified</label>
                                            <label class="inline-flex items-center gap-1"><input type="radio" name="slot-history-layout" value="side" x-model="layout" data-testid="slot-history-layout-side" class="text-amber-700 focus:ring-amber-600"> Side by side</label>
                                        </div>
                                        <label x-show="hasCss()" class="inline-flex items-center gap-1 text-stone-700">
                                            <input type="checkbox" x-model="includeCss" @change="onIncludeCssChange()" data-testid="slot-history-include-css"
                                                   class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                                            Include its CSS
                                        </label>
                                    </div>

                                    <p class="text-xs text-stone-600" x-text="'Left: revision #' + detail.number + '. Right: the editor now, unsaved changes included.'"></p>
                                    <p x-show="isSame()" data-testid="slot-history-same" class="text-sm text-stone-700">Same as the editor. Applying it changes nothing.</p>

                                    <template x-for="field in diffFields" :key="field.field">
                                        <div class="space-y-1">
                                            <h3 class="text-sm font-mono font-medium text-stone-700" x-text="field.field + (field.same ? ' (no change)' : '')"></h3>
                                            <template x-if="!field.same && layout === 'unified'">
                                                <div class="max-h-72 overflow-auto rounded border border-stone-200 font-mono text-xs" tabindex="0" role="region"
                                                     :aria-label="'Unified diff of ' + field.field" :data-testid="'slot-history-unified-' + field.field">
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
                                            <template x-if="!field.same && layout === 'side'">
                                                <div class="max-h-72 overflow-auto rounded border border-stone-200" tabindex="0" role="region" :aria-label="'Side-by-side diff of ' + field.field">
                                                    <table class="w-full table-fixed font-mono text-xs" :data-testid="'slot-history-side-' + field.field">
                                                        <caption class="sr-only" x-text="'Side-by-side diff of ' + field.field"></caption>
                                                        <thead>
                                                            <tr class="bg-stone-100 text-left text-stone-700">
                                                                <th scope="col" class="w-1/2 px-2 py-1 font-semibold" x-text="'Revision #' + detail.number"></th>
                                                                <th scope="col" class="w-1/2 px-2 py-1 font-semibold">Editor now</th>
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

                                    <section aria-labelledby="slot-history-preview-heading" class="space-y-2">
                                        <h3 id="slot-history-preview-heading" class="text-sm font-mono font-semibold text-stone-700" x-text="'Preview of revision #' + detail.number"></h3>
                                        <p x-show="isCssSlot()" data-testid="slot-history-css-note" class="text-sm text-stone-700">CSS has no markup of its own to preview. Every slot preview on the form uses it.</p>
                                        <template x-if="!isCssSlot()">
                                            <div class="space-y-2">
                                                <p x-show="previewLoading" class="text-xs text-stone-600 font-mono">Rendering…</p>
                                                <p x-show="previewError" x-text="previewError" role="alert" class="text-xs text-red-700 font-mono"></p>
                                                {# A picture of the old version, not something to use: inert and out of the tab order, so the dialog's focus containment stays exact. #}
                                                <iframe x-init="bindFrame($el)" sandbox="allow-scripts" inert tabindex="-1"
                                                        :title="'Preview of revision #' + detail.number"
                                                        class="w-full h-64 rounded border border-stone-200 bg-white"></iframe>
                                                <ul x-show="renderErrors.length" class="space-y-1 border-l-4 border-red-700 bg-red-50 px-3 py-2" aria-label="Render errors">
                                                    <template x-for="(re, idx) in renderErrors" :key="idx">
                                                        <li class="text-xs font-mono text-red-800"><span class="font-semibold" x-text="re.label"></span><span x-text="re.detail ? ': ' + re.detail : ''"></span></li>
                                                    </template>
                                                </ul>
                                                <ul x-show="issues.length" class="space-y-0.5" aria-label="Template issues">
                                                    <template x-for="(iss, idx) in issues" :key="idx">
                                                        <li class="text-xs font-mono text-stone-700"><span class="uppercase" x-text="iss.severity"></span>: <span x-text="iss.message"></span></li>
                                                    </template>
                                                </ul>
                                            </div>
                                        </template>
                                    </section>
                                </div>
                            </template>
                        </div>
                    </div>

                    <div class="flex flex-wrap items-center justify-end gap-2 border-t border-stone-200 bg-stone-50 px-4 py-3">
                        <p class="mr-auto text-xs text-stone-600">Apply fills the editor. Nothing is saved until you press Save, and undo in the editor still works.</p>
                        <button type="button" @click="close()"
                                class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-medium font-mono rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-stone-500 cursor-pointer">Cancel</button>
                        <button type="button" x-show="detail" @click="apply()" :disabled="!canApply()"
                                x-text="detail ? 'Apply #' + detail.number + ' to the editor' : 'Apply'"
                                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 cursor-pointer"></button>
                    </div>
                </div>
            </div>
        </template>
    </template>
</div>
{% endif %}
