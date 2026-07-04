{# Template reuse tools. Include with carrier = category | resourceCategory | noteType. Client-side form-filling only; nothing saves until submit. #}
<fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-4"
          x-data="templateBundle({ carrier: '{{ carrier }}' })"
          data-testid="template-bundle-tools">
    <legend class="text-base font-semibold font-mono text-stone-800 px-2">Reuse &amp; Presets</legend>
    <p class="text-sm text-stone-600">Fill this form from an existing template, a preset, or an exported bundle. Nothing is saved until you submit.</p>

    <div class="space-y-4">
        <div>
            <label class="block text-sm font-mono font-medium text-stone-700 mb-1" for="tb-preset">Start from preset</label>
            <div class="flex gap-2">
                <select id="tb-preset" x-model="presetChoice"
                        class="flex-1 min-w-0 rounded-md border-stone-300 text-sm font-mono focus:border-amber-500 focus:ring-amber-500">
                    <option value="">Choose a preset…</option>
                    <template x-for="p in presetOptions" :key="p.name">
                        <option :value="p.name" x-text="p.title || p.name"></option>
                    </template>
                </select>
                <button type="button" @click="applyPreset()" :disabled="!presetChoice"
                        class="shrink-0 inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-white bg-amber-600 border border-transparent rounded-md hover:bg-amber-700 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer">Apply</button>
            </div>
        </div>

        <div>
            <label class="block text-sm font-mono font-medium text-stone-700 mb-1" for="tb-copy">Copy from existing</label>
            <div class="flex gap-2">
                <select id="tb-copy" x-model="copyChoice"
                        class="flex-1 min-w-0 rounded-md border-stone-300 text-sm font-mono focus:border-amber-500 focus:ring-amber-500">
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
                        class="shrink-0 inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer">Copy</button>
            </div>
        </div>
    </div>

    <div class="flex flex-wrap items-center gap-3 pt-1">
        <button type="button" @click="exportBundle()"
                class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">
            Export bundle
        </button>
        <label class="inline-flex items-center px-3 py-1.5 text-sm font-mono font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded-md hover:bg-stone-200 focus-within:ring-2 focus-within:ring-offset-1 focus-within:ring-amber-600 cursor-pointer">
            Import bundle
            <input type="file" accept="application/json,.json" class="sr-only" @change="importBundle($event)">
        </label>
    </div>

    <p x-show="message" x-transition x-text="message" role="status"
       class="text-xs font-mono"
       :class="messageKind === 'warn' ? 'text-amber-700' : 'text-stone-500'"></p>
</fieldset>
