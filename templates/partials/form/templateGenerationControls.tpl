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
                data-testid="generate-button-{{ name }}"
                :disabled="generating"
                :aria-busy="generating.toString()"
                class="inline-flex items-center px-3 py-1.5 border border-stone-300 rounded-md text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer whitespace-nowrap">
            <span x-text="generating ? 'Generating…' : 'Generate'"></span>
        </button>
        <template x-if="(generatedContent || generatedSlots) && (!generatedValid || generationStatus === 'Generated content is ready.')">
            <button type="button"
                    @click="applyGenerated()"
                    data-testid="generate-apply-{{ name }}"
                    class="px-3 py-1.5 text-sm font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 cursor-pointer whitespace-nowrap">
                Use anyway
            </button>
        </template>
    </div>
    <div id="{{ field_id }}-genmsg" class="mt-1 space-y-1" aria-live="polite">
        <template x-if="generationStatus">
            <p data-testid="generate-status-{{ name }}" role="status" class="text-xs text-stone-600 font-mono" x-text="generationStatus"></p>
        </template>
        <template x-if="generationError">
            <p data-testid="generate-error-{{ name }}" role="alert" class="text-xs text-red-700 font-mono" x-text="generationError"></p>
        </template>
    </div>
</div>
