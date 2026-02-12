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
                        class="cursor-pointer p-2 flex block w-full rounded text-gray-900"
                        :class="{'bg-blue-500 !text-white': index === selectedIndex}"
                        @mousedown="{{ action }}"
                        @mouseover="selectedIndex = index;"
                        tabindex="-1"
                    >
                        <span
                                x-text="getItemDisplayName(result)"
                                class="overflow-ellipsis overflow-hidden"
                                :title="getItemDisplayName(result)"
                        ></span>
                    </div>
            </template>
        </div>
    </div>
</div>
