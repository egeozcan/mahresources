<template x-if="{{ condition }}">
    <div class="absolute mt-1 w-full border bg-white shadow-xl rounded z-50 max-h-80 overflow-x-auto z-50">
        <div class="p-3">
            <div x-ref="list">
                <template x-for="(result, index) in results" :key="index">
                        <div
                            role="option"
                            :id="'{{ id }}-result-' + index"
                            :aria-selected="index === selectedIndex"
                            class="cursor-pointer p-2 flex block w-full rounded"
                            :class="{'bg-blue-500': index === selectedIndex}"
                            @mousedown="{{ action }}"
                            @mouseover="selectedIndex = index;"
                            tabindex="-1"
                        >
                            <span
                                    x-text="getItemDisplayName(result)"
                                    class="overflow-ellipsis overflow-hidden"
                                    :title="getItemDisplayName(result)"
                            ></span>
                        </span>
                </template>
            </div>
        </div>
    </div>
</template>
