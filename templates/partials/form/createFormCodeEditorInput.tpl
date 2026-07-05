{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">
        {{ title }}
        {% if description %}<p class="text-xs text-stone-500 mt-0.5 font-sans font-normal">{{ description }}</p>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2"
         x-data="codeEditor({ mode: '{{ mode }}', dbType: '{{ dbType }}', label: '{{ title }}', shortcodes: {% if shortcodes %}true{% else %}false{% endif %}, generate: {% if generate %}true{% else %}false{% endif %} })">
        <input type="hidden" id="{{ field_id }}" name="{{ name }}" x-ref="hiddenInput" value="{{ value }}">
        {% if generate %}
        <div class="mb-2 border border-stone-200 rounded-md p-2 bg-stone-50" aria-label="Generate {{ title }} from natural language">
            <label for="{{ field_id }}-genprompt" class="sr-only">Describe the {{ title }} to generate</label>
            <div class="flex items-start gap-2">
                <textarea id="{{ field_id }}-genprompt"
                          x-model="generationPrompt"
                          data-testid="generate-prompt-{{ name }}"
                          rows="1"
                          @keydown.enter.meta.prevent="generateFromPrompt()"
                          :aria-invalid="generationError ? 'true' : 'false'"
                          aria-describedby="{{ field_id }}-genmsg"
                          class="flex-1 border border-stone-300 rounded-md px-2 py-1 text-sm focus:ring-amber-600 focus:border-amber-600"
                          placeholder="Describe what to generate…"></textarea>
                <button type="button"
                        @click="generateFromPrompt()"
                        data-testid="generate-button-{{ name }}"
                        :disabled="generating"
                        :aria-busy="generating.toString()"
                        class="inline-flex items-center px-3 py-1.5 border border-stone-300 rounded-md text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer whitespace-nowrap">
                    <span x-text="generating ? 'Generating…' : 'Generate'"></span>
                </button>
                <template x-if="generatedContent && generatedValid === false">
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
        {% endif %}
        {% if mode == "json" or mode == "html" %}
        <div class="flex items-center justify-end mb-1">
            <button type="button"
                    @click="formatContent()"
                    class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-600 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer"
                    aria-label="Format {{ mode|upper }} content">
                Format {{ mode|upper }}
            </button>
        </div>
        {% endif %}
        <div x-ref="editorContainer" class="border border-stone-300 rounded-md overflow-hidden"></div>
        {% if mode == "json" or mode == "html" %}
        <div class="mt-1 min-h-[1.25rem]">
            <p x-show="formatError"
               x-transition
               class="text-xs text-red-600 font-mono"
               role="alert"
               x-text="formatError"></p>
        </div>
        {% endif %}
    </div>
</div>
{% endwith %}