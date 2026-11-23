{# The review page's two dialogs, driven by $store.reductionReview.dialog ('apply' or 'shortcuts'). #}
{# Teleported into .overlays like every other modal, under an x-if (the reverse order silently     #}
{# never registers refs). The store makes the rest of the page inert and keeps Tab inside with      #}
{# containFocus, as confirmDialog does; a backdrop click dismisses nothing.                         #}
<template x-if="$store.reductionReview.dialog">
    <template x-teleport=".overlays">
        <div class="confirm-dialog-overlay fixed inset-0 flex items-center justify-center bg-black/50 p-2 sm:p-4"
             x-cloak
             @keydown.escape.window="$store.reductionReview.escapeDialog()">
            <div x-init="$store.reductionReview._mountDialog($el)"
                 tabindex="-1"
                 class="relative bg-white rounded-xl shadow-2xl w-full max-w-lg max-h-[92dvh] overflow-y-auto ring-1 ring-black/5"
                 :role="$store.reductionReview.dialog === 'apply' ? 'alertdialog' : 'dialog'"
                 aria-modal="true"
                 :aria-labelledby="'reduction-dialog-title'"
                 :aria-describedby="$store.reductionReview.dialog === 'apply' ? 'reduction-dialog-body' : null"
                 :aria-busy="$store.reductionReview.applying ? 'true' : 'false'"
                 data-testid="reduction-dialog">

                {# Apply, confirming. #}
                <template x-if="$store.reductionReview.dialog === 'apply' && !$store.reductionReview.applying">
                    <div>
                        <div class="px-5 pt-5 pb-3">
                            <h2 id="reduction-dialog-title" class="text-lg font-medium font-mono text-stone-900">
                                Apply <span x-text="$store.reductionReview.checkedCount"></span> Cluster<span x-text="$store.reductionReview.checkedCount === 1 ? '' : 's'"></span>?
                            </h2>
                            <div id="reduction-dialog-body" class="mt-2 text-sm text-stone-800 space-y-2" data-testid="reduction-apply-figures">
                                <template x-for="tier in $store.reductionReview.tierSummaries()" :key="tier.key">
                                    <p data-testid="reduction-apply-tier">
                                        <strong x-text="tier.headline"></strong>
                                        <span x-text="tier.detail"></span>
                                    </p>
                                </template>
                                <template x-if="$store.reductionReview.figures.keepsSmaller > 0">
                                    <p class="text-amber-900" data-testid="reduction-apply-keeps-smaller">
                                        <span x-text="$store.reductionReview.figures.keepsSmaller"></span>
                                        approved Cluster<span x-text="$store.reductionReview.figures.keepsSmaller === 1 ? '' : 's'"></span>
                                        keep<span x-text="$store.reductionReview.figures.keepsSmaller === 1 ? 's' : ''"></span>
                                        a lower-resolution copy than one they remove.
                                        <a class="underline" :href="$store.reductionReview.keepsSmallerHref()">Review them</a>
                                    </p>
                                </template>
                                <p data-testid="reduction-apply-disk">This does not free disk space. To release space, delete versions you no longer need from the kept copy's Versions panel. The deleted-files backup is emptied only by whoever runs the server.</p>
                                <p class="font-medium">There is no undo.</p>
                            </div>
                        </div>
                        <div class="flex justify-end gap-2 px-5 py-4 bg-stone-50 border-t border-stone-100">
                            <button type="button" autofocus data-testid="reduction-apply-cancel"
                                    @click="$store.reductionReview.closeDialog()"
                                    class="reduction-btn">Cancel</button>
                            <button type="button" data-testid="reduction-apply-confirm"
                                    @click="$store.reductionReview.confirmApply()"
                                    class="reduction-btn reduction-btn--danger"
                                    x-text="'Apply ' + $store.reductionReview.checkedCount + ' Cluster' + ($store.reductionReview.checkedCount === 1 ? '' : 's')"></button>
                        </div>
                    </div>
                </template>

                {# Apply, running: the page is blocked until the response, and the server finishes the batch whatever happens here. #}
                <template x-if="$store.reductionReview.dialog === 'apply' && $store.reductionReview.applying">
                    <div class="px-5 py-5" data-testid="reduction-applying">
                        <h2 id="reduction-dialog-title" class="text-lg font-medium font-mono text-stone-900">
                            Applying <span x-text="$store.reductionReview.applyingCount"></span> Cluster<span x-text="$store.reductionReview.applyingCount === 1 ? '' : 's'"></span>&hellip;
                        </h2>
                        <p id="reduction-dialog-body" class="mt-2 text-sm text-stone-800" role="status">You can leave this page; the server finishes the batch.</p>
                        <progress class="reduction-progress-bar mt-3" aria-label="Applying"></progress>
                    </div>
                </template>

                {# Keyboard shortcuts. #}
                <template x-if="$store.reductionReview.dialog === 'shortcuts'">
                    <div data-testid="reduction-shortcuts-dialog">
                        <div class="px-5 pt-5 pb-3">
                            <h2 id="reduction-dialog-title" class="text-lg font-medium font-mono text-stone-900">Keyboard shortcuts</h2>
                            <p class="mt-2 text-sm text-stone-700">They work while focus is inside the Clusters, never in a form field or with a modifier key.</p>
                            <dl class="reduction-shortcut-list">
                                <dt><kbd>j</kbd> / <kbd>k</kbd></dt><dd>Next / previous Cluster</dd>
                                <dt><kbd>a</kbd></dt><dd>Approve or unapprove the Cluster</dd>
                                <dt><kbd>s</kbd></dt><dd>Skip the Cluster, or reopen a skipped one</dd>
                                <dt><kbd>x</kbd></dt><dd>Show the copies of a large Cluster</dd>
                                <dt><kbd>?</kbd></dt><dd>This list</dd>
                            </dl>
                            <label class="flex items-center gap-2 mt-3 text-sm text-stone-800">
                                <input type="checkbox" data-testid="reduction-shortcuts-off" class="reduction-control rounded border-stone-300 text-amber-700"
                                       :checked="!$store.reductionReview.shortcutsOn"
                                       @change="$store.reductionReview.setShortcuts(!$event.target.checked)">
                                Turn off keyboard shortcuts
                            </label>
                        </div>
                        <div class="flex justify-end gap-2 px-5 py-4 bg-stone-50 border-t border-stone-100">
                            <button type="button" autofocus data-testid="reduction-shortcuts-close"
                                    @click="$store.reductionReview.closeDialog()" class="reduction-btn">Close</button>
                        </div>
                    </div>
                </template>
            </div>
        </div>
    </template>
</template>
