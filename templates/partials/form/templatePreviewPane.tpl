{# categoryId = ID of the category/noteType being edited (null on the create form): entity search and the default pick are restricted to it. #}
<div class="mt-6 border-t border-stone-200 pt-5"
     x-data="templatePreview({ entityType: '{{ entityType }}', previewPath: '{{ previewPath }}', categoryId: {{ categoryId|default:"null" }} })">
    <div class="border border-stone-300 rounded-md overflow-hidden">
        <div class="flex flex-wrap items-end gap-3 p-3 border-b border-stone-200 bg-stone-50">
            <h3 class="w-full text-sm font-mono font-semibold text-stone-700">Live preview</h3>

            <div class="relative" x-show="!isCarrierSlot()">
                <label for="tp-entity-{{ entityType }}" class="block text-xs font-mono text-stone-600 mb-0.5">Preview against</label>
                <input id="tp-entity-{{ entityType }}" type="text"
                       x-model="query" @input="onSearchInput()" @focus="open = suggestions.length > 0"
                       autocomplete="off" role="combobox" aria-autocomplete="list" aria-controls="tp-suggestions-{{ entityType }}"
                       :aria-expanded="open ? 'true' : 'false'"
                       class="w-56 text-sm rounded border border-stone-300 px-2 py-1 focus:outline-none focus:ring-2 focus:ring-amber-600"
                       placeholder="Search {{ entityType }}…">
                <ul x-show="open" @click.away="open = false" x-transition
                    id="tp-suggestions-{{ entityType }}" role="listbox"
                    class="absolute z-20 mt-1 w-56 max-h-52 overflow-auto bg-white border border-stone-300 rounded shadow-md text-sm">
                    <template x-for="s in suggestions" :key="s.id">
                        <li role="option" tabindex="0"
                            @click="pick(s)" @keydown.enter.prevent="pick(s)"
                            class="px-2 py-1 cursor-pointer hover:bg-amber-50 focus:bg-amber-50 focus:outline-none">
                            <span x-text="s.name"></span>
                            <span class="text-stone-500" x-text="'#' + s.id"></span>
                        </li>
                    </template>
                </ul>
            </div>

            <div>
                <label for="tp-slot-{{ entityType }}" class="block text-xs font-mono text-stone-600 mb-0.5">Slot</label>
                <select id="tp-slot-{{ entityType }}" x-model="slot" @change="onSlotChange()"
                        class="text-sm rounded border border-stone-300 px-2 py-1 focus:outline-none focus:ring-2 focus:ring-amber-600">
                    <template x-for="s in slots" :key="s.name">
                        <option :value="s.name" x-text="s.label"></option>
                    </template>
                </select>
            </div>

            <span x-show="isCarrierSlot()" class="text-xs font-mono text-stone-500 self-end pb-1.5">Previewed against this category itself.</span>

            <button type="button" @click="refresh()"
                    class="inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-600 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">
                Refresh
            </button>
            <span x-show="loading" role="status" class="text-xs text-stone-500 font-mono">Rendering…</span>
        </div>

        <p x-show="error" x-text="error" role="alert" class="text-xs text-red-600 font-mono px-3 py-2"></p>

        <p class="text-[11px] text-stone-600 px-3 pt-2">
            Rendered in an isolated sandbox — interactive editors and API-backed widgets are non-functional in preview.
        </p>

        <iframe x-ref="frame" sandbox="allow-scripts" title="Template slot preview"
                class="w-full h-96 bg-white border-0"></iframe>

        <div x-show="issues.length" class="border-t border-stone-200 px-3 py-2 space-y-0.5">
            <template x-for="(iss, idx) in issues" :key="idx">
                <div class="text-xs font-mono"
                     :class="{
                        'text-red-600': iss.severity === 'error',
                        'text-amber-700': iss.severity === 'warning',
                        'text-stone-500': iss.severity === 'info'
                     }">
                    <span class="uppercase" x-text="iss.severity"></span>
                    <span> — </span>
                    <span x-text="iss.message"></span>
                </div>
            </template>
        </div>
    </div>
</div>
