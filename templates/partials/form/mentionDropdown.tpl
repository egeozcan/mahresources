<div x-ref="mentionDropdown"
     x-show="mentionActive && mentionResults.length > 0"
     x-cloak
     :style="getDropdownStyle()"
     role="listbox"
     aria-label="Mention suggestions"
     class="bg-white border border-stone-300 rounded-lg shadow-lg max-h-60 overflow-y-auto">
    <template x-for="(result, index) in mentionResults" :key="result.type + ':' + result.id">
        <button type="button"
                :id="'mention-option-' + result.type + '-' + result.id"
                @click.prevent="selectMention(result)"
                @mouseenter="mentionSelectedIndex = index"
                :data-mention-selected="index === mentionSelectedIndex"
                :class="index === mentionSelectedIndex ? 'bg-amber-50' : ''"
                class="w-full text-left px-3 py-2 flex items-center gap-2 hover:bg-stone-50 cursor-pointer text-sm"
                role="option"
                :aria-selected="index === mentionSelectedIndex">
            <span class="flex-shrink-0" x-text="getIcon(result.type)" aria-hidden="true"></span>
            <span class="flex-1 min-w-0">
                <span class="font-medium truncate block" x-html="highlightMatch(result.name, mentionQuery)"></span>
                <span class="text-xs text-stone-500 truncate block" x-text="result.description" x-show="result.description"></span>
            </span>
            <span class="flex-shrink-0 text-xs font-mono px-1.5 py-0.5 rounded"
                  :class="{
                      'bg-blue-100 text-blue-700': result.type === 'note',
                      'bg-green-100 text-green-700': result.type === 'group',
                      'bg-yellow-100 text-yellow-700': result.type === 'tag',
                      'bg-indigo-100 text-indigo-700': result.type === 'resource',
                      'bg-purple-100 text-purple-700': result.type === 'category',
                      'bg-stone-100 text-stone-700': !['note','group','tag','resource','category'].includes(result.type)
                  }"
                  x-text="getLabel(result.type)">
            </span>
        </button>
    </template>
</div>
<div x-show="mentionActive && mentionLoading && mentionResults.length === 0" x-cloak
     class="bg-white border border-stone-300 rounded-lg shadow-lg p-3 text-sm text-stone-500"
     :style="getDropdownStyle()">
    Searching...
</div>
