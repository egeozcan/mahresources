<template x-for="(result, index) in selectedResults">
    <p class="
            inline-flex rounded-md items-center py-0.5 pl-2.5 pr-1 text-sm font-medium bg-indigo-100
            text-indigo-700 my-1 mr-1
        ">
        <span class="break-all" x-text="getItemDisplayName(result)"></span>
        <button
                @click="selectedResults.splice(index, 1);"
                type="button"
                aria-label="Remove ${result.Name}"
                class="
                        flex-shrink-0 ml-0.5 h-4 w-4 rounded-md inline-flex items-center justify-center
                        text-indigo-400 hover:bg-indigo-200 hover:text-indigo-500 focus:outline-none 
                        focus:bg-indigo-500 focus:text-white"
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
