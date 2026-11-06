{# Live preview of one template slot, the last child of its cluster: slot, label; tpEntityType, tpPreviewPath, tpCarrierId from the enclosing with. #}
{# It renders lazily, against the entity chosen in templatePreviewContext.tpl (src/components/slotPreview.js). #}
<section class="rounded-md border border-stone-300 overflow-hidden" data-slot-preview="{{ slot }}" aria-labelledby="sp-heading-{{ slot }}"
         x-data="slotPreview({ entityType: '{{ tpEntityType }}', slot: '{{ slot }}', previewPath: '{{ tpPreviewPath }}', categoryId: {{ tpCarrierId|default:"null" }} })">
    <div class="flex flex-wrap items-center gap-2 px-3 py-2 border-b border-stone-200 bg-stone-50">
        <h2 id="sp-heading-{{ slot }}" class="text-xs font-mono font-semibold text-stone-700 mr-auto">Preview of {{ label }}</h2>
        <span x-show="loading" role="status" class="text-xs text-stone-600 font-mono">Rendering…</span>
        <button type="button" @click="refresh()" aria-label="Refresh the preview of {{ label }}"
                class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-white border border-stone-300 rounded hover:bg-stone-100 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Refresh</button>
        {% include "/partials/form/slotHistoryButton.tpl" %}
    </div>

    <p x-show="waiting" class="text-xs text-stone-600 font-mono px-3 py-2">Choosing an entity to preview against…</p>
    <p x-show="error" x-text="error" role="alert" class="text-xs text-red-700 font-mono px-3 py-2"></p>

    <iframe x-ref="frame" sandbox="allow-scripts" title="Preview of {{ label }}" class="w-full h-64 bg-white border-0"></iframe>

    {# Always present and given text, not revealed: a live region that merely appears is not reliably announced. #}
    <p class="sr-only" aria-live="polite"
       x-text="renderErrors.length ? renderErrors.length + (renderErrors.length === 1 ? ' render error' : ' render errors') + ' in the preview, listed below it.' : ''"></p>

    {# A failed shortcode shows only its name in the frame; the reason is in a hover-only title attribute. List each one in full. #}
    <div x-show="renderErrors.length" class="border-t border-red-200 bg-red-50 px-3 py-2">
        <h3 class="text-xs font-mono font-semibold text-red-800 mb-1">Render errors</h3>
        <ul class="space-y-1">
            <template x-for="(re, idx) in renderErrors" :key="idx">
                <li class="text-xs font-mono text-red-800">
                    <span class="font-semibold" x-text="re.label"></span><span x-text="(re.count > 1 ? ' (×' + re.count + ')' : '') + (re.detail ? ': ' : '')"></span><span class="whitespace-pre-wrap break-words" x-text="re.detail"></span>
                </li>
            </template>
        </ul>
    </div>

    <ul x-show="issues.length" class="border-t border-stone-200 px-3 py-2 space-y-0.5">
        <template x-for="(iss, idx) in issues" :key="idx">
            <li class="text-xs font-mono"
                :class="{ 'text-red-700': iss.severity === 'error', 'text-amber-800': iss.severity === 'warning', 'text-stone-600': iss.severity === 'info' }">
                <span class="uppercase" x-text="iss.severity"></span>: <span x-text="iss.message"></span>
            </li>
        </template>
    </ul>
</section>
