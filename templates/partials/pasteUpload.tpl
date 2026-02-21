{# Paste/Drop Upload Modal #}
<div x-data
     x-show="$store.pasteUpload.isOpen"
     x-cloak
     x-transition:enter="transition ease-out duration-200"
     x-transition:enter-start="opacity-0"
     x-transition:enter-end="opacity-100"
     x-transition:leave="transition ease-in duration-150"
     x-transition:leave-start="opacity-100"
     x-transition:leave-end="opacity-0"
     class="fixed inset-0 z-50 overflow-y-auto"
     role="dialog"
     aria-modal="true"
     aria-labelledby="paste-upload-title"
     @keydown.escape.window="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()">
    {# Backdrop #}
    <div class="fixed inset-0 bg-black bg-opacity-50 transition-opacity"
         tabindex="-1"
         @click="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()"></div>

    {# Modal content #}
    <div class="flex min-h-full items-center justify-center p-4">
        <div class="relative bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[80vh] flex flex-col"
             @click.stop
             x-trap.noscroll="$store.pasteUpload.isOpen">
            {# Header #}
            <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200">
                <h2 id="paste-upload-title" class="text-lg font-semibold text-gray-900">
                    Upload to <span x-text="$store.pasteUpload.context?.name || 'Unknown'"></span>
                </h2>
                <button @click="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()"
                        :disabled="$store.pasteUpload.state === 'uploading'"
                        class="text-gray-400 hover:text-gray-600 disabled:opacity-50 disabled:cursor-not-allowed"
                        aria-label="Close">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                </button>
            </div>

            {# Body (scrollable) #}
            <div class="flex-1 overflow-y-auto p-4 space-y-4">
                {# ARIA live region for screen readers #}
                <span class="sr-only" aria-live="polite" aria-atomic="true"
                      x-text="$store.pasteUpload.state === 'uploading'
                          ? $store.pasteUpload.uploadProgress
                          : $store.pasteUpload.items.length + ' items ready to upload'"></span>

                {# Success banner #}
                <div x-show="$store.pasteUpload.state === 'success'"
                     x-cloak
                     class="p-3 bg-green-50 border border-green-200 rounded-md text-sm text-green-700 flex items-center gap-2"
                     role="status">
                    <svg class="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/>
                    </svg>
                    <span x-text="$store.pasteUpload.uploadProgress"></span>
                </div>

                {# Error message #}
                <div x-show="$store.pasteUpload.errorMessage"
                     x-cloak
                     class="p-3 bg-red-50 border border-red-200 rounded-md text-sm text-red-700"
                     role="alert">
                    <span x-text="$store.pasteUpload.errorMessage"></span>
                </div>

                {# Item rows #}
                <template x-for="(item, index) in $store.pasteUpload.items" :key="index">
                    <div class="flex items-center gap-3 p-3 border rounded-lg transition-colors duration-200"
                         :class="{
                             'border-red-300 bg-red-50': item.error && item.error !== 'done',
                             'border-green-300 bg-green-50': item.error === 'done',
                             'border-gray-200': !item.error
                         }">
                        {# Preview column #}
                        <div class="w-16 h-16 flex-shrink-0 bg-gray-100 rounded overflow-hidden flex items-center justify-center">
                            <template x-if="item.type === 'image'">
                                <img :src="item.previewUrl" :alt="item.name" class="w-full h-full object-cover">
                            </template>
                            <template x-if="item.type === 'file'">
                                <svg class="w-8 h-8 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
                                </svg>
                            </template>
                            <template x-if="item.type === 'text' || item.type === 'html'">
                                <div class="w-full h-full p-1 text-xs text-gray-500 overflow-hidden leading-tight"
                                     x-text="(item._snippet || '').substring(0, 80)"></div>
                            </template>
                        </div>

                        {# Name input #}
                        <div class="flex-1 min-w-0">
                            <input type="text"
                                   x-model="item.name"
                                   :disabled="$store.pasteUpload.state === 'uploading'"
                                   :aria-label="'Name for item ' + (index + 1)"
                                   class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500 disabled:bg-gray-100 disabled:cursor-not-allowed">
                            {# Per-item error text #}
                            <template x-if="item.error && item.error !== 'done'">
                                <p class="mt-1 text-xs text-red-600">
                                    <span x-text="item.error"></span>
                                    <template x-if="item.errorResourceId">
                                        <span> &mdash;
                                            <a :href="'/resource?id=' + item.errorResourceId"
                                               class="text-blue-600 hover:text-blue-800 underline font-medium"
                                               target="_blank"
                                               @click.stop>View existing resource</a>
                                        </span>
                                    </template>
                                </p>
                            </template>
                        </div>

                        {# Remove button #}
                        <button @click="$store.pasteUpload.removeItem(index)"
                                :disabled="$store.pasteUpload.state === 'uploading'"
                                class="flex-shrink-0 p-1 text-gray-400 hover:text-red-600 disabled:opacity-50 disabled:cursor-not-allowed"
                                :aria-label="'Remove ' + item.name">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                            </svg>
                        </button>
                    </div>
                </template>

                {# Shared metadata section #}
                <div x-show="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.state !== 'success' && $store.pasteUpload.items.length > 0"
                     class="space-y-3 pt-2 border-t border-gray-200">
                    <p class="text-sm font-medium text-gray-700">Shared metadata</p>

                    {# Tags autocompleter #}
                    <div x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/tags',
                             addUrl: '/v1/tag',
                             standalone: true,
                         })"
                         x-effect="$store.pasteUpload.tags = selectedResults.map(r => r.ID)"
                         class="relative w-full">
                        <label class="block text-xs text-gray-500 mb-1">Tags</label>
                        <div class="relative">
                            <template x-if="!addModeForTag">
                                <input x-ref="autocompleter"
                                       type="text"
                                       x-bind="inputEvents"
                                       class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                       placeholder="Search tags..."
                                       autocomplete="off">
                            </template>
                            <template x-if="addModeForTag">
                                <div class="flex gap-2 items-stretch">
                                    <button type="button"
                                            class="flex-1 px-2 py-1.5 text-sm font-medium text-white bg-green-700 rounded hover:bg-green-800 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-green-500"
                                            x-text="'Add ' + addModeForTag + '?'"
                                            x-init="setTimeout(() => $el.focus(), 1)"
                                            @keydown.escape.prevent="exitAdd"
                                            @keydown.enter.prevent="addVal"
                                            @click="addVal"></button>
                                    <button type="button"
                                            class="px-2 py-1.5 text-sm text-gray-600 bg-gray-100 rounded hover:bg-gray-200"
                                            @click="exitAdd">Cancel</button>
                                </div>
                            </template>
                            {% include "/partials/form/formParts/dropDownResults.tpl" with action="pushVal" id="paste-upload-tags" title="Tags" %}
                            {% include "/partials/form/formParts/dropDownSelectedResults.tpl" %}
                        </div>
                    </div>

                    {# Category autocompleter #}
                    <div x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/resourceCategories',
                             max: 1,
                             standalone: true,
                         })"
                         x-effect="$store.pasteUpload.categoryId = selectedResults[0]?.ID || null"
                         class="relative w-full">
                        <label class="block text-xs text-gray-500 mb-1">Category</label>
                        <div class="relative">
                            <input x-ref="autocompleter"
                                   type="text"
                                   x-bind="inputEvents"
                                   class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                   placeholder="Search categories..."
                                   autocomplete="off">
                            {% include "/partials/form/formParts/dropDownResults.tpl" with action="pushVal" id="paste-upload-category" title="Category" %}
                            {% include "/partials/form/formParts/dropDownSelectedResults.tpl" %}
                        </div>
                    </div>

                    {# Series autocompleter #}
                    <div x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/seriesList',
                             addUrl: '/v1/series/create',
                             max: 1,
                             standalone: true,
                         })"
                         x-effect="$store.pasteUpload.seriesId = selectedResults[0]?.ID || null"
                         class="relative w-full">
                        <label class="block text-xs text-gray-500 mb-1">Series</label>
                        <div class="relative">
                            <template x-if="!addModeForTag">
                                <input x-ref="autocompleter"
                                       type="text"
                                       x-bind="inputEvents"
                                       class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                       placeholder="Search or create series..."
                                       autocomplete="off">
                            </template>
                            <template x-if="addModeForTag">
                                <div class="flex gap-2 items-stretch">
                                    <button type="button"
                                            class="flex-1 px-2 py-1.5 text-sm font-medium text-white bg-green-700 rounded hover:bg-green-800 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-green-500"
                                            x-text="'Add ' + addModeForTag + '?'"
                                            x-init="setTimeout(() => $el.focus(), 1)"
                                            @keydown.escape.prevent="exitAdd"
                                            @keydown.enter.prevent="addVal"
                                            @click="addVal"></button>
                                    <button type="button"
                                            class="px-2 py-1.5 text-sm text-gray-600 bg-gray-100 rounded hover:bg-gray-200"
                                            @click="exitAdd">Cancel</button>
                                </div>
                            </template>
                            {% include "/partials/form/formParts/dropDownResults.tpl" with action="pushVal" id="paste-upload-series" title="Series" %}
                            {% include "/partials/form/formParts/dropDownSelectedResults.tpl" %}
                        </div>
                    </div>
                </div>
            </div>

            {# Footer #}
            <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
                {# Left: status text #}
                <span class="text-sm text-gray-600">
                    <template x-if="$store.pasteUpload.state === 'uploading'">
                        <span x-text="$store.pasteUpload.uploadProgress"></span>
                    </template>
                    <template x-if="$store.pasteUpload.state === 'success'">
                        <span class="text-green-600" x-text="$store.pasteUpload.uploadProgress"></span>
                    </template>
                    <template x-if="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.state !== 'success'">
                        <span x-text="$store.pasteUpload.items.length + ' item' + ($store.pasteUpload.items.length !== 1 ? 's' : '')"></span>
                    </template>
                </span>

                {# Right: Cancel + Upload buttons #}
                <div class="flex gap-2">
                    <button @click="$store.pasteUpload.state !== 'uploading' && $store.pasteUpload.close()"
                            :disabled="$store.pasteUpload.state === 'uploading'"
                            type="button"
                            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed">
                        Cancel
                    </button>
                    <button @click="$store.pasteUpload.upload()"
                            :disabled="$store.pasteUpload.state === 'uploading' || $store.pasteUpload.items.length === 0"
                            type="button"
                            class="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
                        {# Spinner SVG during uploading #}
                        <svg x-show="$store.pasteUpload.state === 'uploading'"
                             x-cloak
                             class="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        <span x-text="$store.pasteUpload.state === 'error' ? 'Retry' : 'Upload'"></span>
                    </button>
                </div>
            </div>
        </div>
    </div>
</div>

{# Info toast (OUTSIDE the modal, at bottom of page) #}
<div x-data
     x-show="$store.pasteUpload.infoMessage"
     x-cloak
     x-transition:enter="transition ease-out duration-300"
     x-transition:enter-start="opacity-0 translate-y-2"
     x-transition:enter-end="opacity-100 translate-y-0"
     x-transition:leave="transition ease-in duration-200"
     x-transition:leave-start="opacity-100 translate-y-0"
     x-transition:leave-end="opacity-0 translate-y-2"
     class="fixed bottom-20 left-1/2 -translate-x-1/2 z-50 px-4 py-2 bg-gray-800 text-white text-sm rounded-lg shadow-lg"
     role="status"
     aria-live="polite">
    <span x-text="$store.pasteUpload.infoMessage"></span>
</div>
