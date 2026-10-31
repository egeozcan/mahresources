{# Template reuse tools. Include with carrier = category | resourceCategory | noteType. Client-side form-filling only; nothing saves until submit. #}
<fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-4"
          x-data="templateBundle({ carrier: '{{ carrier }}' })"
          data-testid="template-bundle-tools">
    <legend class="text-base font-semibold font-mono text-stone-800 px-2">Reuse &amp; Presets</legend>
    {# What this form applied; each entry posts with the save as revisionApplied. #}
    <template x-for="entry in appliedEntries" :key="entry">
        <input type="hidden" name="revisionApplied" :value="entry">
    </template>
    <p x-show="revisionNotice" x-text="revisionNotice" role="status" tabindex="-1" x-ref="revisionNotice" data-testid="revision-load-notice"
       class="border-l-4 border-t-4 p-3 text-sm focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"
       :class="revisionNoticeKind === 'warn' ? 'border-red-700 bg-red-50 text-red-900' : 'border-green-700 bg-green-50 text-green-900'"></p>
    <p class="text-sm text-stone-600">Fill this form from an existing template, a preset, or an exported bundle. Nothing is saved until you submit.</p>

    <div class="space-y-4">
        <div>
            <label class="block text-sm font-mono font-medium text-stone-700 mb-1" for="tb-preset">Start from preset</label>
            <div class="flex flex-wrap gap-2">
                <select id="tb-preset" x-model="presetChoice"
                        class="flex-1 min-w-0 basis-full sm:basis-0 max-w-full rounded-md border-stone-300 text-sm font-mono focus:border-amber-500 focus:ring-amber-500">
                    <option value="">Choose a preset…</option>
                    <template x-for="p in presetOptions" :key="p.name">
                        <option :value="p.name" x-text="p.title || p.name"></option>
                    </template>
                </select>
                <button type="button" @click="applyPreset()" :disabled="!presetChoice"
                        data-testid="tb-apply-preset"
                        class="shrink-0 inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer">Apply</button>
            </div>
        </div>

        <div>
            <label class="block text-sm font-mono font-medium text-stone-700 mb-1" for="tb-copy">Copy from existing</label>
            <div class="flex flex-wrap gap-2">
                <select id="tb-copy" x-model="copyChoice"
                        class="flex-1 min-w-0 basis-full sm:basis-0 max-w-full rounded-md border-stone-300 text-sm font-mono focus:border-amber-500 focus:ring-amber-500">
                    <option value="">Choose a source…</option>
                    <template x-for="(cfg, key) in carriers" :key="key">
                        <optgroup :label="cfg.label + (key === carrier ? ' (this type)' : '')">
                            <template x-for="src in sources[key]" :key="key + ':' + (src.ID || src.id)">
                                <option :value="key + ':' + (src.ID || src.id)" x-text="src.Name || src.name"></option>
                            </template>
                        </optgroup>
                    </template>
                </select>
                <button type="button" @click="copyFrom()" :disabled="!copyChoice"
                        class="shrink-0 inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer">Copy</button>
            </div>
        </div>
    </div>

    <div class="border-t border-stone-200 pt-4">
        <label class="block text-sm font-mono font-medium text-stone-700 mb-1" for="tb-generate-prompt">Generate whole template from a description</label>
        <p class="text-xs text-stone-500 mb-2">First shows exactly what would go to the configured AI provider (this description, the metadata schema, a sample entity), and sends it only when you approve. A planning step settles one design, then each section is written separately, so this can take up to a minute. The draft then waits for review; nothing is filled until you press Apply.</p>
        <fieldset class="mb-3 rounded-md border border-stone-200 p-3" data-testid="template-bundle-sections" x-show="sections.length">
            <legend class="px-1 text-xs font-mono font-medium text-stone-700">Sections to generate (leave all unticked to let the generator choose)</legend>
            <div class="mt-1 grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-2 lg:grid-cols-3">
                <template x-for="section in sections" :key="section.field">
                    <label class="flex min-w-0 items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" :value="section.field" x-model="selectedSections"
                               :data-testid="'template-bundle-section-' + section.field"
                               class="shrink-0 rounded border-stone-300 text-amber-700 focus:outline-hidden focus-visible:ring-2 focus-visible:ring-offset-1 focus-visible:ring-amber-600">
                        <span class="min-w-0 break-words" x-text="section.label"></span>
                    </label>
                </template>
            </div>
            <p class="mt-2 text-xs text-stone-500">Each section is written together with its CSS companion. Shared CSS only gains new rules after your existing CSS.</p>
        </fieldset>
        <div class="flex flex-wrap gap-2">
            <textarea id="tb-generate-prompt" x-model="generationPrompt" rows="2" x-ref="bundlePrompt"
                      data-testid="template-bundle-generate-prompt"
                      class="flex-1 min-w-0 rounded-md border-stone-300 text-sm focus:border-amber-500 focus:ring-amber-500"
                      placeholder="e.g. A recipe card with the cooking time, a photo, and a list of ingredients"></textarea>
            <button type="button" @click="generateBundle()" :disabled="generating"
                    x-ref="bundleGenerate"
                    data-testid="template-bundle-generate-button"
                    :aria-busy="generating.toString()"
                    class="shrink-0 self-start inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer">
                <span x-text="generating ? 'Generating…' : 'Generate'"></span>
            </button>
        </div>
        {# A generated draft is held here until Apply; nothing reaches an editor before. #}
        <template x-if="pendingDraft">
            <section class="mt-3 rounded-md border border-stone-300 bg-white p-3 space-y-3" data-testid="template-bundle-review" aria-labelledby="tb-review-heading">
                <p id="tb-review-heading" class="text-sm font-semibold font-mono text-stone-800">Generated draft (not applied)</p>
                <p x-show="pendingDraft.explanation" class="text-xs text-stone-600" x-text="pendingDraft.explanation"></p>
                <p x-show="pendingDraft.failedSlots.length" data-testid="template-bundle-failed-slots" role="status" class="text-xs font-mono text-amber-800"
                   x-text="'Could not generate: ' + pendingDraft.failedSlots.join(', ') + '. ' + (pendingDraft.failedSlots.length === 1 ? 'It was' : 'They were') + ' left as ' + (pendingDraft.failedSlots.length === 1 ? 'it is' : 'they are') + '; generate ' + (pendingDraft.failedSlots.length === 1 ? 'it' : 'them') + ' on their own.'"></p>
                <ul x-show="pendingDraft.issues.length" class="text-xs font-mono text-red-700 list-disc pl-5" data-testid="template-bundle-review-issues">
                    <template x-for="(issue, i) in pendingDraft.issues" :key="i">
                        <li x-text="issue.message"></li>
                    </template>
                </ul>
                <ul class="space-y-2">
                    <template x-for="field in pendingDraft.fields" :key="field">
                        <li :data-testid="'template-bundle-review-slot-' + field" class="min-w-0">
                            <p class="text-xs font-mono font-medium text-stone-700" x-text="field"></p>
                            <ul x-show="(pendingDraft.slotIssues[field] || []).length" class="text-xs font-mono text-red-700 list-disc pl-5">
                                <template x-for="(issue, i) in (pendingDraft.slotIssues[field] || [])" :key="i">
                                    <li x-text="issue.message"></li>
                                </template>
                            </ul>
                            <p x-show="draftWarnings(pendingDraft.slots[field], field.endsWith('CSS')).length" class="text-xs font-mono text-amber-800"
                               x-text="'Runs code once applied: ' + draftWarnings(pendingDraft.slots[field], field.endsWith('CSS')).join(', ') + ' (highlighted).'"></p>
                            <p x-show="pendingDraft.blocked.includes(field)" class="text-xs font-mono text-red-700">Not applied: its companion did not arrive.</p>
                            <pre tabindex="0" role="region" :aria-label="'Draft text of ' + field" :data-testid="'template-bundle-review-text-' + field" class="mt-1 max-h-40 overflow-auto rounded border border-stone-200 bg-stone-50 p-2 text-xs font-mono whitespace-pre-wrap break-all focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"><template x-for="(segment, i) in draftSegments(pendingDraft.slots[field] || '(empty)', field.endsWith('CSS'))" :key="i"><span :class="segment.flagged ? 'bg-amber-200 text-stone-900 outline outline-1 outline-amber-700' : ''" x-text="segment.text"></span></template></pre>
                        </li>
                    </template>
                </ul>
                <div class="flex flex-wrap gap-2">
                    <button type="button" x-show="canApplyDraft()" @click="applyPendingDraft()" data-testid="template-bundle-apply"
                            class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Apply</button>
                    <button type="button" @click="discardPendingDraft()" data-testid="template-bundle-discard"
                            class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Discard</button>
                </div>
            </section>
        </template>
    </div>

    <div class="flex flex-wrap items-center gap-3 pt-1">
        <button type="button" @click="exportBundle()"
                class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">
            Export bundle
        </button>
        <label class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus-within:ring-2 focus-within:ring-offset-1 focus-within:ring-amber-600 cursor-pointer">
            Import bundle
            <input type="file" accept="application/json,.json" class="sr-only" @change="importBundle($event)">
        </label>
    </div>

    {# Always mounted: focus waits here while a sent generation runs (Generate is disabled). #}
    <p x-text="message" role="status" x-ref="bundleStatus" tabindex="-1" data-testid="template-bundle-status"
       class="text-xs font-mono empty:hidden focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"
       :class="messageKind === 'warn' ? 'text-amber-700' : 'text-stone-500'"></p>
    <template x-if="groundingChanged">
        <button type="button" @click="generateBundle()" data-testid="template-bundle-review-again"
                class="px-2 py-1 text-xs font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Review again</button>
    </template>
</fieldset>
