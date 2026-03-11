<template x-for="(result, index) in selectedResults">
    <p class="
            inline-flex rounded items-center py-0.5 pl-2 pr-0.5 text-xs font-mono font-medium bg-amber-100
            text-amber-800 my-0.5 mr-0.5
        ">
        <span class="break-all" x-text="getItemDisplayName(result)"></span>
        <button
                @click="selectedResults.splice(index, 1);"
                type="button"
                aria-label="Remove ${result.Name}"
                class="
                        flex-shrink-0 ml-0.5 h-4 w-4 rounded-md inline-flex items-center justify-center
                        text-amber-600 hover:bg-amber-200 hover:text-amber-700 focus:outline-none
                        focus:bg-amber-700 focus:text-white"
                tabindex="0"
                @keydown.enter.prevent="selectedResults.splice(index, 1); $event.target.closest('button').focus()"
                @keydown.space.prevent="selectedResults.splice(index, 1); $event.target.closest('button').focus()"
        >
            <span x-text="'Remove ' + result.Name" class="sr-only"></span>
            <svg class="h-2 w-2" stroke="currentColor" fill="none" viewBox="0 0 8 8">
                <path stroke-linecap="round" stroke-width="1.5" d="M1 1l6 6m0-6L1 7" />
            </svg>
        </button>
    </p>
</template>