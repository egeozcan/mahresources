<fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-3"
          data-testid="metadata-index-editor" x-data="metadataIndexEditor({{ metadataIndexes|json }})">
    <legend class="text-base font-semibold font-mono text-stone-800 px-2">Indexed metadata keys</legend>
    <p class="text-sm text-stone-600">Speed up queries on frequently used metadata. Enter keys such as <code>score</code> or <code>camera.iso</code>, without the <code>meta.</code> prefix. Save this form to apply changes.</p>
    <input type="hidden" name="MetadataIndexes" :value="serialized" />
    <p x-show="error" x-cloak role="alert" class="text-sm text-red-700" x-text="error"></p>
    <button x-show="error" x-cloak type="button" @click="entries = []; error = ''" class="text-sm text-red-700 underline">Clear invalid index definitions</button>
    <template x-for="(entry, i) in entries" :key="i">
        <div class="flex flex-wrap gap-2 items-center">
            <input type="text" x-model="entry.key" placeholder="score or camera.iso" :aria-label="'Metadata key for index ' + (i + 1)" maxlength="128" class="border border-stone-300 rounded px-2 py-1 text-sm flex-1 min-w-0 font-mono" />
            <select x-model="entry.kind" :aria-label="'Kind for index ' + (i + 1)" class="border border-stone-300 rounded px-2 py-1 text-sm">
                <option value="numeric">Numeric</option><option value="text">Text equality</option>
            </select>
            <button type="button" @click="entries.splice(i, 1)" :aria-label="'Remove index ' + (i + 1)" class="text-sm text-red-700 underline">Remove</button>
        </div>
    </template>
    <button type="button" @click="entries.push({key: '', kind: 'numeric'})" :disabled="entries.length >= 32 || !!error" class="text-sm text-amber-700 underline disabled:opacity-50">Add indexed key</button>
    <p class="text-xs text-stone-500">Numeric indexes support equality and ranges. Text indexes support case-insensitive equality. Builds run in the background; SQLite builds may delay writes. Categories using the same key share an index. Removing a key here keeps that index while another category still uses it.</p>
</fieldset>
<script>
window.metadataIndexEditor = function (initial) {
    let entries = [], error = '';
    try {
        entries = JSON.parse(initial || '[]');
        if (!Array.isArray(entries)) throw new Error('Expected a list of indexed keys');
        if (entries.some(entry => !entry || typeof entry.key !== 'string' || !['numeric', 'text'].includes(entry.kind))) {
            throw new Error('Each indexed key needs a key and a numeric or text kind');
        }
    } catch (e) { entries = []; error = 'Invalid index definitions: ' + e.message; }
    return {
        entries, error,
        get serialized() { return this.error ? initial : JSON.stringify(this.entries); },
    };
};
</script>
