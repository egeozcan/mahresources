<template x-if="{{ condition }}">
    <div class="absolute mt-1 w-full border bg-white shadow-xl rounded z-50">
        <div class="p-3">
            <div x-ref="list">
                <template x-for="(result, index) in results" :key="index">
                        <span
                                :active="false"
                                class="cursor-pointer p-2 flex block w-full rounded"
                                :class="{'bg-blue-500': index === selectedIndex}"
                                @mousedown="{{ action }}"
                                @mouseover="selectedIndex = index;"
                        >
                            <span
                                    x-text="result.Name"
                                    class="overflow-ellipsis overflow-hidden"
                                    :title="result.Name"
                            ></span>
                        </span>
                </template>
            </div>
        </div>
    </div>
</template>