{# MRQL filter bar (package 5): `entity` is the page's entity type; the current value + fail-closed error come from parsedQuery.MRQL / mrqlError. #}
<div
    class="mrql-bar mb-3"
    x-id="['mrql-bar']"
    x-data="mrqlBar({ entity: '{{ entity }}', value: '{{ parsedQuery.MRQL|escapejs }}', error: '{{ mrqlError|escapejs }}' })"
>
    <div
        x-show="!formCompatible"
        x-cloak
        class="mb-2 flex flex-wrap items-center justify-between gap-2 rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900"
        role="status"
    >
        <span>The MRQL editor and sidebar form cannot represent the same filters. The form is disabled.</span>
        <button type="button" @click="useFormValues()" class="rounded bg-amber-700 px-3 py-1.5 text-white hover:bg-amber-800">
            Use form values
        </button>
        <span class="w-full text-xs">Using the form values will remove the MRQL-only filters.</span>
    </div>
    <form role="search" class="flex flex-wrap items-start gap-2" @submit.prevent="submit()">
        <div class="relative flex-1 min-w-0">
            <label class="sr-only" :for="$id('mrql-bar') + '-input'">Filter these {{ entity }}s with an MRQL expression</label>
            <input
                x-ref="input"
                :id="$id('mrql-bar') + '-input'"
                type="text"
                name="mrql"
                x-model="query"
                @input="onInput()"
                @keydown.arrow-down.prevent="navigateDown()"
                @keydown.arrow-up.prevent="navigateUp()"
                @keydown.enter="onEnter($event)"
                @keydown.escape.prevent="closeSuggestions()"
                @blur="onBlur()"
                role="combobox"
                aria-autocomplete="list"
                :aria-expanded="open ? 'true' : 'false'"
                :aria-controls="$id('mrql-bar') + '-listbox'"
                :aria-activedescendant="activeDescendant()"
                :aria-describedby="error ? ($id('mrql-bar') + '-error') : null"
                :aria-invalid="error ? 'true' : 'false'"
                placeholder="Filter, e.g. tags = &quot;vacation&quot; AND created &gt; -30d"
                autocomplete="off"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
                class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring focus:border-blue-400"
            >
            <ul
                x-show="open"
                x-cloak
                :id="$id('mrql-bar') + '-listbox'"
                role="listbox"
                aria-label="MRQL filter suggestions"
                class="absolute z-30 mt-1 w-full max-h-64 overflow-auto bg-white border border-gray-300 rounded shadow-lg text-sm"
            >
                <template x-for="(s, i) in suggestions" :key="i">
                    <li
                        :id="$id('mrql-bar') + '-opt-' + i"
                        role="option"
                        :aria-selected="i === selectedIndex ? 'true' : 'false'"
                        :data-selected="i === selectedIndex"
                        @mousedown.prevent="applySuggestion(i)"
                        @mouseenter="selectedIndex = i"
                        class="px-3 py-1.5 cursor-pointer flex items-baseline gap-2"
                        :class="i === selectedIndex ? 'bg-blue-100' : ''"
                    >
                        <span class="font-mono" x-text="s.value"></span>
                        <span class="text-xs text-gray-500" x-show="s.label" x-text="s.label"></span>
                    </li>
                </template>
            </ul>
        </div>
        <button type="submit" class="px-3 py-2 text-sm rounded bg-blue-600 text-white hover:bg-blue-700">Filter</button>
        <a :href="editorLink()" class="px-2 py-2 text-sm text-blue-600 underline whitespace-nowrap self-center">Edit in MRQL editor</a>
    </form>
    <p
        x-show="error"
        x-cloak
        :id="$id('mrql-bar') + '-error'"
        class="mt-1 text-sm text-red-600"
        role="alert"
        x-text="error"
    ></p>
</div>
