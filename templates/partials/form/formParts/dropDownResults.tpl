<div
    x-ref="dropdown"
    popover="manual"
    class="border bg-white shadow-xl rounded max-h-80 overflow-y-auto p-0 m-0"
    role="region"
    aria-label="{{ title|default:'Dropdown options' }}"
    style="position: fixed;"
>
    <div class="p-3">
        <div
            x-ref="list"
            id="{{ id }}-listbox"
            role="listbox"
            :aria-label="'{{ title|default:"Select an option" }}'"
        >
            <template x-for="(result, index) in results" :key="index">
                    <div
                        role="option"
                        :id="'{{ id }}-result-' + index"
                        :aria-selected="index === selectedIndex"
                        class="cursor-pointer p-2 flex block w-full rounded text-stone-900"
                        :class="{'bg-amber-700 !text-white': index === selectedIndex}"
                        {# A click names one rendered row, so it commits by identity; the index-based action would drop it while a newer search is in flight. #}
                        @mousedown="startSelecting(); setActiveIndex(index); selectResult(result)"
                        @mouseover="setActiveIndex(index)"
                        tabindex="-1"
                    >
                        <span
                                x-text="getItemDisplayName(result)"
                                class="overflow-ellipsis overflow-hidden"
                                :title="getItemDisplayName(result)"
                        ></span>
                    </div>
            </template>
            <!-- "Create X" row for the no-match case (only for a creatable profile) -->
            <template x-if="createCandidate">
                    <div
                        role="option"
                        :id="'{{ id }}-result-' + results.length"
                        :aria-selected="selectedIndex === results.length"
                        class="cursor-pointer p-2 flex block w-full rounded text-stone-900 border-t border-stone-200"
                        :class="{'bg-amber-700 !text-white': selectedIndex === results.length}"
                        @mousedown="startSelecting(); setActiveIndex(results.length); {{ action }}($event)"
                        tabindex="-1"
                    >
                        <span x-text="'Create &quot;' + createCandidate + '&quot;'" class="overflow-ellipsis overflow-hidden"></span>
                    </div>
            </template>
        </div>
    </div>
</div>