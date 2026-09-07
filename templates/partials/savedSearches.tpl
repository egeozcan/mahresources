<section x-data="savedSearches('{{ savedSearchView.Family }}')" x-cloak class="mb-4 min-w-0" aria-label="Saved searches"
         x-id="['saved-searches-panel', 'saved-search-name', 'saved-search-dialog']"
         @keydown.escape.stop.prevent="close()">
    <button type="button" x-ref="trigger" @click="toggle()" :aria-expanded="open" :aria-controls="$id('saved-searches-panel')"
            class="inline-flex items-center gap-2 rounded border border-stone-300 bg-white px-3 py-1.5 text-sm font-mono hover:bg-stone-100">
        Saved searches <span aria-hidden="true" x-text="open ? '▴' : '▾'"></span>
    </button>
    <p role="status" class="text-sm text-stone-600 mt-1" x-text="status"></p>
    <div x-show="open" :id="$id('saved-searches-panel')" class="mt-2 rounded border border-stone-300 bg-white p-3 space-y-3">
        <button type="button" x-ref="save" @click="startDialog()" :disabled="busy || loading"
                class="rounded bg-amber-700 px-3 py-1.5 text-sm font-mono text-white hover:bg-amber-800 disabled:opacity-50">Save current search</button>
        <p x-show="loading" role="status" class="text-sm text-stone-600">Loading saved searches…</p>
        <div x-show="error && !dialog" role="alert" class="text-sm text-red-700">
            <p x-text="error"></p>
            <button type="button" @click="load()" :disabled="loading || busy" class="underline">Reload saved searches</button>
        </div>
        <p x-show="!loading && !error && searches.length === 0" class="text-sm text-stone-600">No saved searches for this list yet.</p>
        <ul class="space-y-3">
            <template x-for="search in searches" :key="search.id">
                <li class="flex flex-wrap items-center gap-2 border-t border-stone-100 pt-2">
                    <a :href="search.url" class="min-w-0 break-words text-amber-800 underline" x-text="search.name"></a>
                    <span class="text-xs text-stone-500" x-text="search.layout"></span>
                    <div class="flex flex-wrap gap-2 text-xs w-full">
                        <button type="button" @click="startDialog(search)" :disabled="busy || loading" :aria-label="'Rename saved search: ' + search.name" class="underline">Rename</button>
                        <button type="button" @click="replace(search)" :disabled="busy || loading" :aria-label="'Replace saved search: ' + search.name" class="underline">Replace with current search</button>
                        <button type="button" @click="remove(search)" :disabled="busy || loading" :aria-label="'Delete saved search: ' + search.name" class="underline text-red-700">Delete</button>
                    </div>
                </li>
            </template>
        </ul>
    </div>
    <template x-teleport=".overlays">
        <div x-show="dialog" x-cloak class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
             @click.self="closeDialog()" @keydown.escape.stop.prevent="closeDialog()">
            <div role="dialog" aria-modal="true" :aria-labelledby="$id('saved-search-dialog')"
                 x-trap.inert.noreturn.noscroll="!!dialog"
                 class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
              <form @submit.prevent="save()" class="space-y-4">
                <h2 :id="$id('saved-search-dialog')" class="text-lg font-semibold" x-text="dialog"></h2>
                <p x-show="!selected" class="text-sm text-stone-600">Saves the applied filters, sort order, and view. Apply any filter edits before saving.</p>
                <div>
                    <label :for="$id('saved-search-name')" class="block text-sm font-medium">Name</label>
                    <input :id="$id('saved-search-name')" x-model="name" type="text" required maxlength="200" :disabled="busy"
                           class="mt-1 w-full rounded border border-stone-300 px-3 py-2">
                </div>
                <p x-show="error" role="alert" class="text-sm text-red-700" x-text="error"></p>
                <div class="flex justify-end gap-2">
                    <button type="button" @click="closeDialog()" :disabled="busy" class="rounded border border-stone-300 px-3 py-1.5 text-sm hover:bg-stone-100 disabled:opacity-50">Cancel</button>
                    <button type="submit" :disabled="busy || !name.trim()" class="rounded bg-amber-700 px-3 py-1.5 text-sm text-white hover:bg-amber-800 disabled:opacity-50" x-text="busy ? 'Saving…' : 'Save'"></button>
                </div>
              </form>
            </div>
        </div>
    </template>
</section>
