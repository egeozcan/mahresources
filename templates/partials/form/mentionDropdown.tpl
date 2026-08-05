{# Finding 133: the textarea that owns this dropdown declares role=combobox but #}
{# had no aria-controls to point at, because this element had no id. The id is #}
{# derived from the field so several mention textareas on one page (the note #}
{# block editor renders one per text block) do not collide. `field_id` is set by #}
{# createFormTextareaInput.tpl; blockEditor.tpl passes mentionListboxId. #}
{# dynamicListboxId=true is the block-editor case: the id has to come from the #}
{# Alpine scope (one mention textarea per text block), so it is bound rather than #}
{# interpolated server-side. #}
{% with mention_listbox_id=mentionListboxId|default:field_id|default:"mention" %}
<div x-ref="mentionDropdown"
     {% if dynamicListboxId %}:id="'block-' + block.id + '-mention-listbox'"{% else %}id="{{ mention_listbox_id }}-mention-listbox"{% endif %}
     x-show="mentionActive && mentionResults.length > 0"
     x-cloak
     :style="getDropdownStyle()"
     role="listbox"
     aria-label="Mention suggestions"
     class="bg-white border border-stone-300 rounded-lg shadow-lg max-h-60 overflow-y-auto">
    <template x-for="(result, index) in mentionResults" :key="result.type + ':' + result.id">
        {# Deferred-work item 8: a <button> is natively focusable, so without      #}
        {# tabindex="-1" every option sat in the tab order. The owning textarea    #}
        {# drives this list with aria-activedescendant and never yields focus      #}
        {# (mentionTextarea.js refocuses it after a pick, and its keydown handler  #}
        {# only knows Arrow/Enter/Escape), so Tab walked into the listbox and DOM  #}
        {# focus diverged from the option aria says is active. The house pattern   #}
        {# is adminExport.tpl and adminImport.tpl: tabindex="-1" on the button.    #}
        <button type="button"
                tabindex="-1"
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
{% endwith %}
