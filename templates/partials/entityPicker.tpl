{# Generic Entity Picker Modal #}
<div x-show="$store.entityPicker.isOpen"
     x-cloak
     class="fixed inset-0 z-50 overflow-y-auto"
     role="dialog"
     aria-modal="true"
     aria-labelledby="entity-picker-title"
     @keydown.escape.window="$store.entityPicker.close()">
    {# Backdrop #}
    <div class="fixed inset-0 bg-black bg-opacity-50 transition-opacity"
         @click="$store.entityPicker.close()"></div>

    {# Modal content #}
    <div class="flex min-h-full items-center justify-center p-4">
        <div class="relative bg-white rounded-lg shadow-xl w-full max-w-3xl max-h-[80vh] flex flex-col"
             @click.stop
             x-trap.noscroll="$store.entityPicker.isOpen">
            {# Header #}
            <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200">
                <h2 id="entity-picker-title" class="text-lg font-semibold text-gray-900">
                    Select <span x-text="$store.entityPicker.config?.entityLabel || 'Items'"></span>
                </h2>
                <button @click="$store.entityPicker.close()"
                        class="text-gray-400 hover:text-gray-600"
                        aria-label="Close">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                </button>
            </div>

            {# Tabs (if configured) #}
            <template x-if="$store.entityPicker.config?.tabs">
                <div class="flex border-b border-gray-200 px-4" role="tablist">
                    <template x-for="tab in $store.entityPicker.config.tabs" :key="tab.id">
                        <button @click="$store.entityPicker.setActiveTab(tab.id)"
                                :class="$store.entityPicker.activeTab === tab.id ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'"
                                class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
                                :disabled="tab.id === 'note' && !$store.entityPicker.noteId"
                                :class="{ 'opacity-50 cursor-not-allowed': tab.id === 'note' && !$store.entityPicker.noteId }"
                                role="tab"
                                :aria-selected="$store.entityPicker.activeTab === tab.id">
                            <span x-text="tab.label"></span>
                            <span x-show="tab.id === 'note' && $store.entityPicker.hasTabResults"
                                  class="ml-1 text-xs bg-gray-100 px-1.5 py-0.5 rounded"
                                  x-text="$store.entityPicker.tabResults.note?.length || 0"></span>
                        </button>
                    </template>
                </div>
            </template>

            {# Filters #}
            <div x-show="!$store.entityPicker.config?.tabs || $store.entityPicker.activeTab === 'all'"
                 class="px-4 py-3 border-b border-gray-200 space-y-2">
                {# Search #}
                <div>
                    <input type="text"
                           x-model="$store.entityPicker.searchQuery"
                           @input="$store.entityPicker.onSearchInput()"
                           placeholder="Search by name..."
                           class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-blue-500 focus:border-blue-500">
                </div>
                {# Dynamic filters based on config #}
                <template x-if="$store.entityPicker.config?.filters?.length > 0">
                    <div class="flex gap-3">
                        <template x-for="filter in $store.entityPicker.config.filters" :key="filter.key">
                            <div class="flex-1"
                                 x-data="autocompleter({
                                     selectedResults: [],
                                     url: filter.endpoint,
                                     max: filter.multi ? 0 : 1,
                                     standalone: true,
                                     onSelect: (item) => filter.multi
                                         ? $store.entityPicker.addToFilter(filter.key, item.ID)
                                         : $store.entityPicker.setFilter(filter.key, item.ID),
                                     onRemove: (item) => filter.multi
                                         ? $store.entityPicker.removeFromFilter(filter.key, item.ID)
                                         : $store.entityPicker.setFilter(filter.key, null)
                                 })"
                                 @entity-picker-closed.window="selectedResults = []">
                                <label class="block text-xs text-gray-500 mb-1" x-text="filter.label"></label>
                                <div class="relative">
                                    <input x-ref="autocompleter"
                                           type="text"
                                           x-bind="inputEvents"
                                           class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                           :placeholder="'Filter by ' + filter.label.toLowerCase() + '...'"
                                           autocomplete="off">
                                    <template x-if="dropdownActive && results.length > 0">
                                        <div class="absolute z-30 mt-1 w-full bg-white border border-gray-200 rounded shadow-lg max-h-40 overflow-y-auto">
                                            <template x-for="(result, index) in results" :key="result.ID">
                                                <div class="px-3 py-1.5 cursor-pointer text-sm"
                                                     :class="{'bg-blue-500 text-white': index === selectedIndex, 'hover:bg-gray-50': index !== selectedIndex}"
                                                     @mousedown="pushVal"
                                                     @mouseover="selectedIndex = index"
                                                     x-text="result.Name"></div>
                                            </template>
                                        </div>
                                    </template>
                                    <template x-if="selectedResults.length > 0">
                                        <div class="flex flex-wrap gap-1 mt-1">
                                            <template x-for="item in selectedResults" :key="item.ID">
                                                <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-xs">
                                                    <span x-text="item.Name" class="truncate max-w-[100px]"></span>
                                                    <button type="button" @click="removeItem(item)" class="hover:text-blue-600">&times;</button>
                                                </span>
                                            </template>
                                        </div>
                                    </template>
                                </div>
                            </div>
                        </template>
                    </div>
                </template>
            </div>

            {# Results grid #}
            <div class="flex-1 overflow-y-auto p-4" tabindex="0">
                {# Loading state #}
                <div x-show="$store.entityPicker.loading" class="flex items-center justify-center py-12 text-gray-500">
                    <svg class="animate-spin h-6 w-6 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Loading...
                </div>

                {# Error state #}
                <div x-show="$store.entityPicker.error && !$store.entityPicker.loading"
                     class="text-center py-12 text-red-600">
                    <p x-text="$store.entityPicker.error"></p>
                    <button @click="$store.entityPicker.loadResults()"
                            class="mt-2 text-sm text-blue-600 hover:underline">Try again</button>
                </div>

                {# Empty state #}
                <div x-show="!$store.entityPicker.loading && !$store.entityPicker.error && $store.entityPicker.displayResults.length === 0"
                     class="text-center py-12 text-gray-500">
                    <p>No <span x-text="$store.entityPicker.config?.entityLabel?.toLowerCase() || 'items'"></span> found</p>
                </div>

                {# Results grid - Resource thumbnails #}
                <div x-show="!$store.entityPicker.loading && $store.entityPicker.displayResults.length > 0 && $store.entityPicker.config?.renderItem === 'thumbnail'"
                     :class="$store.entityPicker.config?.gridColumns || 'grid-cols-3'"
                     class="grid gap-3"
                     role="listbox"
                     :aria-label="'Available ' + ($store.entityPicker.config?.entityLabel?.toLowerCase() || 'items')">
                    <template x-for="item in $store.entityPicker.displayResults" :key="$store.entityPicker.config.getItemId(item)">
                        <div @click="$store.entityPicker.toggleSelection($store.entityPicker.config.getItemId(item))"
                             class="relative aspect-square bg-gray-100 rounded-lg overflow-hidden cursor-pointer transition-all"
                             :class="{
                                 'ring-2 ring-blue-500 ring-offset-2': $store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)),
                                 'opacity-50 cursor-not-allowed': $store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item)),
                                 'hover:ring-2 hover:ring-gray-300': !$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)) && !$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))
                             }"
                             role="option"
                             :aria-selected="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                             :aria-disabled="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))">
                            <img :src="'/v1/resource/preview?id=' + $store.entityPicker.config.getItemId(item)"
                                 :alt="$store.entityPicker.config.getItemLabel(item)"
                                 class="w-full h-full object-cover"
                                 loading="lazy">
                            {# Selection checkbox #}
                            <div x-show="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                                 class="absolute top-2 right-2 w-6 h-6 bg-blue-500 rounded-full flex items-center justify-center">
                                <svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                                </svg>
                            </div>
                            {# Already added badge #}
                            <div x-show="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))"
                                 class="absolute inset-0 bg-black bg-opacity-40 flex items-center justify-center">
                                <span class="text-xs text-white bg-black bg-opacity-60 px-2 py-1 rounded">Added</span>
                            </div>
                            {# Name tooltip #}
                            <div class="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/60 to-transparent p-2">
                                <p class="text-xs text-white truncate" x-text="$store.entityPicker.config.getItemLabel(item)"></p>
                            </div>
                        </div>
                    </template>
                </div>

                {# Results grid - Group cards #}
                <div x-show="!$store.entityPicker.loading && $store.entityPicker.displayResults.length > 0 && $store.entityPicker.config?.renderItem === 'groupCard'"
                     :class="$store.entityPicker.config?.gridColumns || 'grid-cols-2'"
                     class="grid gap-3"
                     role="listbox"
                     :aria-label="'Available ' + ($store.entityPicker.config?.entityLabel?.toLowerCase() || 'items')">
                    <template x-for="item in $store.entityPicker.displayResults" :key="$store.entityPicker.config.getItemId(item)">
                        <div @click="$store.entityPicker.toggleSelection($store.entityPicker.config.getItemId(item))"
                             class="flex items-start gap-3 p-3 border rounded-lg cursor-pointer transition-all"
                             :class="{
                                 'ring-2 ring-blue-500 border-blue-500 bg-blue-50': $store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)),
                                 'opacity-50 cursor-not-allowed bg-gray-50': $store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item)),
                                 'border-gray-200 hover:border-gray-300 hover:bg-gray-50': !$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)) && !$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))
                             }"
                             role="option"
                             :aria-selected="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                             :aria-disabled="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))">
                            {# Thumbnail or icon #}
                            <div class="w-14 h-14 flex-shrink-0 bg-gray-100 rounded overflow-hidden">
                                <template x-if="item.MainResource?.ID">
                                    <img :src="'/v1/resource/preview?id=' + item.MainResource.ID"
                                         class="w-full h-full object-cover"
                                         loading="lazy">
                                </template>
                                <template x-if="!item.MainResource?.ID">
                                    <div class="w-full h-full flex items-center justify-center text-gray-400">
                                        <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                                        </svg>
                                    </div>
                                </template>
                            </div>
                            {# Content #}
                            <div class="flex-1 min-w-0">
                                <div class="flex items-start justify-between">
                                    <p class="font-medium text-gray-900 truncate" x-text="item.Name || 'Unnamed Group'"></p>
                                    {# Selection indicator #}
                                    <div x-show="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                                         class="ml-2 w-5 h-5 bg-blue-500 rounded-full flex items-center justify-center flex-shrink-0">
                                        <svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                                        </svg>
                                    </div>
                                    {# Already added badge #}
                                    <span x-show="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))"
                                          class="ml-2 text-xs bg-gray-200 text-gray-600 px-1.5 py-0.5 rounded flex-shrink-0">Added</span>
                                </div>
                                {# Breadcrumb #}
                                <p x-show="item.Owner?.Name" class="text-xs text-gray-500 truncate" x-text="item.Owner?.Name"></p>
                                {# Metadata #}
                                <div class="flex items-center gap-2 mt-1 text-xs text-gray-500">
                                    <span x-show="item.ResourceCount > 0" x-text="item.ResourceCount + ' resources'"></span>
                                    <span x-show="item.NoteCount > 0" x-text="item.NoteCount + ' notes'"></span>
                                    <span x-show="item.Category?.Name" class="px-1.5 py-0.5 bg-gray-100 text-gray-600 rounded" x-text="item.Category?.Name"></span>
                                </div>
                            </div>
                        </div>
                    </template>
                </div>
            </div>

            {# Footer #}
            <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
                <span class="text-sm text-gray-600">
                    <span x-text="$store.entityPicker.selectionCount"></span> selected
                </span>
                <div class="flex gap-2">
                    <button @click="$store.entityPicker.close()"
                            type="button"
                            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">
                        Cancel
                    </button>
                    <button @click="$store.entityPicker.confirm()"
                            type="button"
                            :disabled="$store.entityPicker.selectionCount === 0"
                            class="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
                        Confirm
                    </button>
                </div>
            </div>
        </div>
    </div>
</div>
