<div class="mb-2 border border-stone-200 rounded-md p-2 bg-stone-50" aria-label="Generate {{ title }} from natural language">
    <label for="{{ field_id }}-genprompt" class="sr-only">Describe the {{ title }} to generate</label>
    {# flex-wrap + min-w-0: the prompt textarea is flex-1 and the buttons are #}
    {# whitespace-nowrap, so at 390px the row was 932px wide and the Generate  #}
    {# button sat at x=880 (findings 19/101).                                  #}
    <div class="flex flex-wrap items-start gap-2">
        <textarea id="{{ field_id }}-genprompt"
                  x-model="generationPrompt"
                  data-testid="generate-prompt-{{ name }}"
                  rows="1"
                  @keydown.enter.meta.prevent="generateFromPrompt()"
                  :aria-invalid="generationError ? 'true' : 'false'"
                  aria-describedby="{{ field_id }}-genmsg"
                  class="flex-1 min-w-0 basis-full sm:basis-0 border border-stone-300 rounded-md px-2 py-1 text-sm focus:ring-amber-600 focus:border-amber-600"
                  placeholder="Describe what to generate…"></textarea>
        <button type="button"
                @click="generateFromPrompt()"
                x-ref="generateButton"
                data-testid="generate-button-{{ name }}"
                :disabled="generating"
                :aria-busy="generating.toString()"
                class="inline-flex items-center px-3 py-1.5 border border-stone-300 rounded-md text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer whitespace-nowrap">
            <span x-text="generating ? 'Generating…' : 'Generate'"></span>
        </button>
    </div>
    <div id="{{ field_id }}-genmsg" class="mt-1 space-y-1" aria-live="polite">
        {# Always mounted: focus waits here while a sent request runs (Generate is disabled), #}
        {# so it must not disappear when the text clears. Empty and boxless when idle.        #}
        <p data-testid="generate-status-{{ name }}" x-ref="generateStatus" tabindex="-1" role="status" class="text-xs text-stone-600 font-mono empty:hidden focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600" x-text="generationStatus"></p>
        <template x-if="groundingChanged">
            <button type="button" @click="generateFromPrompt()" data-testid="generate-review-again-{{ name }}"
                    class="px-2 py-1 text-xs font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Review again</button>
        </template>
        <template x-if="generationError">
            <p data-testid="generate-error-{{ name }}" role="alert" class="text-xs text-red-700 font-mono" x-text="generationError"></p>
        </template>
    </div>
    {# A pending draft is shown as text only (x-text, never x-html) and reaches no editor before Apply. #}
    <template x-if="hasDraft()">
        <section class="mt-2 rounded-md border border-stone-300 bg-white p-2 space-y-2" data-testid="generate-review-{{ name }}" aria-labelledby="{{ field_id }}-genreview">
            <p id="{{ field_id }}-genreview" class="text-xs font-semibold font-mono text-stone-800">Generated draft (not applied)</p>
            <template x-for="field in generatedFields" :key="field.name">
                <div class="min-w-0" :data-testid="'generate-review-field-' + field.name">
                    <p class="text-xs font-mono font-medium text-stone-700" x-text="field.name"></p>
                    <p x-show="draftWarnings(field.text, field.css).length" class="text-xs font-mono text-amber-800"
                       x-text="'Runs code once applied: ' + draftWarnings(field.text, field.css).join(', ') + ' (highlighted).'"></p>
                    <pre tabindex="0" role="region" :aria-label="'Draft text of ' + field.name" :data-testid="'generate-review-text-' + field.name" class="mt-1 max-h-48 overflow-auto rounded border border-stone-200 bg-stone-50 p-2 text-xs font-mono whitespace-pre-wrap break-all focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"><template x-for="(segment, i) in draftSegments(field.text || '(empty)', field.css)" :key="i"><span :class="segment.flagged ? 'bg-amber-200 text-stone-900 outline outline-1 outline-amber-700' : ''" x-text="segment.text"></span></template></pre>
                </div>
            </template>
            <p x-show="draftBlocked()" class="text-xs font-mono text-red-700">This draft cannot be applied: a template and its CSS are applied together, and the reply was cut off before both arrived.</p>
            <div class="flex flex-wrap gap-2">
                <button type="button"
                        x-show="!draftBlocked()"
                        @click="applyGenerated()"
                        data-testid="generate-apply-{{ name }}"
                        class="px-3 py-1.5 text-sm font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer whitespace-nowrap">
                    Apply
                </button>
                <button type="button"
                        @click="discardFromPanel()"
                        data-testid="generate-discard-{{ name }}"
                        class="px-3 py-1.5 text-sm font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer whitespace-nowrap">
                    Discard
                </button>
            </div>
        </section>
    </template>
</div>
