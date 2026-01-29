{# with noteId= blocks= #}
<div x-data="blockEditor({{ noteId }}, {{ blocks|json }})" class="block-editor">
    {# Edit mode toggle #}
    <div class="flex justify-end mb-4">
        <button
            @click="toggleEditMode()"
            class="px-3 py-1 text-sm rounded border"
            :class="editMode ? 'bg-blue-500 text-white border-blue-500' : 'bg-white text-gray-700 border-gray-300 hover:bg-gray-50'"
        >
            <span x-text="editMode ? 'Done' : 'Edit Blocks'"></span>
        </button>
    </div>

    {# Loading state #}
    <div x-show="loading" class="text-center py-8 text-gray-500">
        Loading blocks...
    </div>

    {# Error state #}
    <div x-show="error" x-cloak class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
        <div class="flex items-center justify-between">
            <span x-text="error"></span>
            <button @click="error = null" class="text-red-500 hover:text-red-700">&times;</button>
        </div>
    </div>

    {# Blocks list #}
    <div x-show="!loading" class="space-y-4">
        <template x-for="(block, index) in blocks" :key="block.id">
            <div class="block-card bg-white border border-gray-200 rounded-lg overflow-hidden"
                 :class="{ 'ring-2 ring-blue-200': editMode }">
                {# Block controls (edit mode only) #}
                <div x-show="editMode" class="flex items-center justify-between px-3 py-2 bg-gray-50 border-b border-gray-200">
                    <span class="text-xs font-medium text-gray-500 uppercase" x-text="block.type"></span>
                    <div class="flex gap-1">
                        <button
                            @click="moveBlock(block.id, 'up')"
                            :disabled="index === 0"
                            class="p-1 text-gray-400 hover:text-gray-600 disabled:opacity-30 disabled:cursor-not-allowed"
                            title="Move up"
                        >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7"/>
                            </svg>
                        </button>
                        <button
                            @click="moveBlock(block.id, 'down')"
                            :disabled="index === blocks.length - 1"
                            class="p-1 text-gray-400 hover:text-gray-600 disabled:opacity-30 disabled:cursor-not-allowed"
                            title="Move down"
                        >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
                            </svg>
                        </button>
                        <button
                            @click="if(confirm('Delete this block?')) deleteBlock(block.id)"
                            class="p-1 text-red-400 hover:text-red-600"
                            title="Delete block"
                        >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                            </svg>
                        </button>
                    </div>
                </div>

                {# Block content by type #}
                <div class="block-content p-4">
                    {# Text block #}
                    <template x-if="block.type === 'text'">
                        <div>
                            <template x-if="!editMode">
                                <div class="prose max-w-none" x-html="renderMarkdown(block.content?.text || '')"></div>
                            </template>
                            <template x-if="editMode">
                                <div x-data="blockText(block, (id, content) => updateBlockContent(id, content), (id, content) => updateBlockContentDebounced(id, content))">
                                    <textarea
                                        x-model="text"
                                        @input="onInput()"
                                        @blur="save()"
                                        class="w-full min-h-[100px] p-2 border border-gray-300 rounded resize-y"
                                        placeholder="Enter text..."
                                    ></textarea>
                                </div>
                            </template>
                        </div>
                    </template>

                    {# Heading block #}
                    <template x-if="block.type === 'heading'">
                        <div>
                            <template x-if="!editMode">
                                <div>
                                    <h1 x-show="block.content?.level === 1" x-text="block.content?.text || ''" class="text-3xl font-bold"></h1>
                                    <h2 x-show="block.content?.level === 2 || !block.content?.level" x-text="block.content?.text || ''" class="text-2xl font-bold"></h2>
                                    <h3 x-show="block.content?.level === 3" x-text="block.content?.text || ''" class="text-xl font-bold"></h3>
                                </div>
                            </template>
                            <template x-if="editMode">
                                <div x-data="blockHeading(block, (id, content) => updateBlockContent(id, content), (id, content) => updateBlockContentDebounced(id, content))" class="flex gap-2">
                                    <select x-model.number="level" @change="save()" class="border border-gray-300 rounded px-2 py-1">
                                        <option value="1">H1</option>
                                        <option value="2">H2</option>
                                        <option value="3">H3</option>
                                    </select>
                                    <input
                                        type="text"
                                        x-model="text"
                                        @input="onInput()"
                                        @blur="save()"
                                        class="flex-1 p-2 border border-gray-300 rounded"
                                        placeholder="Heading text..."
                                    >
                                </div>
                            </template>
                        </div>
                    </template>

                    {# Divider block #}
                    <template x-if="block.type === 'divider'">
                        <hr class="border-t-2 border-gray-300 my-2">
                    </template>

                    {# Todos block #}
                    <template x-if="block.type === 'todos'">
                        <div x-data="blockTodos(block, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state), () => editMode)">
                            <template x-if="!editMode">
                                <ul class="space-y-1">
                                    <template x-for="item in items" :key="item.id">
                                        <li class="flex items-center gap-2">
                                            <input
                                                type="checkbox"
                                                :checked="isChecked(item.id)"
                                                @change="toggleCheck(item.id)"
                                                class="h-4 w-4 rounded border-gray-300"
                                            >
                                            <span :class="{ 'line-through text-gray-400': isChecked(item.id) }" x-text="item.label"></span>
                                        </li>
                                    </template>
                                </ul>
                            </template>
                            <template x-if="editMode">
                                <div class="space-y-2">
                                    <template x-for="(item, idx) in items" :key="item.id">
                                        <div class="flex items-center gap-2">
                                            <input
                                                type="text"
                                                x-model="item.label"
                                                @blur="saveContent()"
                                                class="flex-1 p-1 border border-gray-300 rounded"
                                            >
                                            <button @click="removeItem(idx)" class="text-red-500 hover:text-red-700">&times;</button>
                                        </div>
                                    </template>
                                    <button @click="addItem()" class="text-sm text-blue-600 hover:underline">+ Add item</button>
                                </div>
                            </template>
                        </div>
                    </template>

                    {# Gallery block #}
                    <template x-if="block.type === 'gallery'">
                        <div x-data="blockGallery(block, (id, content) => updateBlockContent(id, content), () => editMode, noteId)" x-init="init()">
                            <template x-if="!editMode && resourceIds.length > 0">
                                <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
                                    <template x-for="(resId, idx) in resourceIds" :key="resId">
                                        <a :href="'/v1/resource/view?id=' + resId"
                                           @click.prevent="openGalleryLightbox(idx)"
                                           class="block aspect-square bg-gray-100 rounded overflow-hidden cursor-pointer hover:opacity-90 transition-opacity">
                                            <img :src="'/v1/resource/preview?id=' + resId" class="w-full h-full object-cover" loading="lazy">
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && resourceIds.length === 0">
                                <p class="text-gray-400 text-sm">No resources selected</p>
                            </template>
                            <template x-if="editMode">
                                <div class="space-y-3">
                                    {# Selected resources preview #}
                                    <template x-if="resourceIds.length > 0">
                                        <div class="grid grid-cols-4 md:grid-cols-6 gap-2">
                                            <template x-for="(resId, idx) in resourceIds" :key="resId">
                                                <div class="relative group aspect-square bg-gray-100 rounded overflow-hidden">
                                                    <img :src="'/v1/resource/preview?id=' + resId" class="w-full h-full object-cover">
                                                    <button
                                                        @click="removeResource(resId)"
                                                        class="absolute top-1 right-1 w-5 h-5 bg-red-500 text-white rounded-full text-xs opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center"
                                                        title="Remove"
                                                    >&times;</button>
                                                </div>
                                            </template>
                                        </div>
                                    </template>
                                    {# Add resources button #}
                                    <button
                                        @click="openPicker()"
                                        type="button"
                                        class="w-full py-2 px-4 border-2 border-dashed border-gray-300 rounded-lg text-gray-500 hover:border-blue-400 hover:text-blue-500 transition-colors text-sm"
                                    >
                                        + Select Resources
                                    </button>
                                </div>
                            </template>
                        </div>
                    </template>

                    {# References block #}
                    <template x-if="block.type === 'references'">
                        <div x-data="blockReferences(block, (id, content) => updateBlockContent(id, content), () => editMode)">
                            <template x-if="!editMode && groupIds.length > 0">
                                <div class="flex flex-wrap gap-2">
                                    <template x-for="gId in groupIds" :key="gId">
                                        <a :href="'/group?id=' + gId" class="inline-flex items-center px-3 py-1 bg-blue-100 text-blue-700 rounded-full text-sm hover:bg-blue-200">
                                            Group <span x-text="gId" class="ml-1 font-medium"></span>
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && groupIds.length === 0">
                                <p class="text-gray-400 text-sm">No groups selected</p>
                            </template>
                            <template x-if="editMode">
                                <div>
                                    <p class="text-sm text-gray-500 mb-2">Group IDs (comma-separated):</p>
                                    <input
                                        type="text"
                                        :value="groupIds.join(', ')"
                                        @blur="updateGroupIds($event.target.value)"
                                        class="w-full p-2 border border-gray-300 rounded"
                                        placeholder="e.g., 1, 2, 3"
                                    >
                                </div>
                            </template>
                        </div>
                    </template>

                    {# Table block #}
                    <template x-if="block.type === 'table'">
                        <div x-data="blockTable(block, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state), () => editMode)" x-init="init()">
                            {# Display mode - Table view #}
                            <template x-if="!editMode">
                                <div>
                                    {# Query mode loading state #}
                                    <template x-if="isQueryMode && queryLoading && !isRefreshing">
                                        <div class="flex items-center justify-center py-8 text-gray-500">
                                            <svg class="animate-spin h-5 w-5 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                            </svg>
                                            Loading data...
                                        </div>
                                    </template>

                                    {# Query mode error state #}
                                    <template x-if="isQueryMode && queryError">
                                        <div class="p-3 bg-red-50 border border-red-200 rounded-lg text-sm">
                                            <div class="flex items-center justify-between text-red-700">
                                                <span x-text="queryError"></span>
                                                <button @click="manualRefresh()" class="ml-2 px-2 py-1 bg-red-100 hover:bg-red-200 rounded text-xs">
                                                    Retry
                                                </button>
                                            </div>
                                        </div>
                                    </template>

                                    {# Table header with refresh controls for query mode #}
                                    <template x-if="isQueryMode && !queryLoading && !queryError && displayColumns.length > 0">
                                        <div class="flex items-center justify-between mb-2 text-xs text-gray-500">
                                            <div class="flex items-center gap-2">
                                                <span x-show="lastFetchTime" x-text="'Updated ' + lastFetchTimeFormatted"></span>
                                                <template x-if="isRefreshing">
                                                    <span class="flex items-center">
                                                        <svg class="animate-spin h-3 w-3 mr-1" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                                        </svg>
                                                        Refreshing...
                                                    </span>
                                                </template>
                                                <span x-show="isStatic" class="px-1.5 py-0.5 bg-gray-100 rounded text-gray-600">Static</span>
                                            </div>
                                            <button @click="manualRefresh()" :disabled="queryLoading" class="px-2 py-1 text-blue-600 hover:text-blue-800 disabled:opacity-50" title="Refresh data">
                                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
                                                </svg>
                                            </button>
                                        </div>
                                    </template>

                                    {# Table display #}
                                    <template x-if="displayColumns.length > 0 && !queryLoading">
                                        <div class="overflow-x-auto">
                                            <table class="min-w-full divide-y divide-gray-200">
                                                <thead class="bg-gray-50">
                                                    <tr>
                                                        <template x-for="col in displayColumns" :key="col.id">
                                                            <th
                                                                @click="toggleSort(col.id)"
                                                                class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider cursor-pointer hover:bg-gray-100"
                                                            >
                                                                <span x-text="col.label"></span>
                                                                <span x-show="sortColumn === col.id" x-text="sortDirection === 'asc' ? ' ▲' : ' ▼'"></span>
                                                            </th>
                                                        </template>
                                                    </tr>
                                                </thead>
                                                <tbody class="bg-white divide-y divide-gray-200">
                                                    <template x-for="row in displayRows" :key="row.id">
                                                        <tr>
                                                            <template x-for="col in displayColumns" :key="col.id">
                                                                <td class="px-3 py-2 text-sm text-gray-900" x-text="row[col.id] ?? ''"></td>
                                                            </template>
                                                        </tr>
                                                    </template>
                                                </tbody>
                                            </table>
                                        </div>
                                    </template>

                                    {# Empty state #}
                                    <template x-if="displayColumns.length === 0 && !queryLoading && !queryError">
                                        <p class="text-gray-400 text-sm">No table data</p>
                                    </template>
                                </div>
                            </template>

                            {# Edit mode #}
                            <template x-if="editMode">
                                <div class="space-y-4">
                                    {# Data Source Section #}
                                    <div class="p-3 bg-gray-50 rounded-lg border border-gray-200"
                                         @table-query-selected="selectQuery($event.detail)"
                                         @table-query-cleared="clearQuery()">
                                        <div class="flex items-center gap-3 flex-wrap">
                                            <label class="text-sm font-medium text-gray-700">Data Source:</label>
                                            {# Query autocomplete or Manual indicator #}
                                            <div class="w-48"
                                                 x-data="autocompleter({
                                                     selectedResults: queryId ? [{ ID: queryId, Name: selectedQueryName || ('Query #' + queryId) }] : [],
                                                     url: '/v1/queries',
                                                     max: 1,
                                                     standalone: true,
                                                     onSelect: (q) => $dispatch('table-query-selected', q),
                                                     onRemove: () => $dispatch('table-query-cleared')
                                                 })"
                                                 class="relative">
                                                <template x-if="addModeForTag == ''">
                                                    <div>
                                                        <input
                                                            x-ref="autocompleter"
                                                            type="text"
                                                            x-bind="inputEvents"
                                                            class="w-full px-2 py-1 text-sm border border-gray-300 rounded focus:ring-indigo-500 focus:border-indigo-500"
                                                            :placeholder="selectedResults.length ? '' : 'Search queries...'"
                                                            autocomplete="off"
                                                        >
                                                        {# Dropdown results #}
                                                        <template x-if="dropdownActive && results.length > 0">
                                                            <div class="absolute z-20 mt-1 w-48 bg-white border border-gray-200 rounded shadow-lg max-h-48 overflow-y-auto">
                                                                <template x-for="(result, index) in results" :key="result.ID">
                                                                    <div
                                                                        class="px-3 py-1.5 cursor-pointer text-sm truncate"
                                                                        :class="{'bg-blue-500 text-white': index === selectedIndex, 'hover:bg-gray-50': index !== selectedIndex}"
                                                                        @mousedown="pushVal"
                                                                        @mouseover="selectedIndex = index"
                                                                        x-text="result.Name"
                                                                    ></div>
                                                                </template>
                                                            </div>
                                                        </template>
                                                        {# Selected query chip #}
                                                        <template x-if="selectedResults.length > 0">
                                                            <div class="flex flex-wrap gap-1 mt-1">
                                                                <template x-for="item in selectedResults" :key="item.ID">
                                                                    <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-xs">
                                                                        <span x-text="item.Name" class="truncate max-w-[150px]"></span>
                                                                        <button type="button" @click="removeItem(item)" class="hover:text-blue-600">&times;</button>
                                                                    </span>
                                                                </template>
                                                            </div>
                                                        </template>
                                                    </div>
                                                </template>
                                            </div>
                                            <template x-if="isQueryMode">
                                                <label class="flex items-center gap-1.5 text-sm text-gray-600">
                                                    <input type="checkbox" x-model="isStatic" @change="saveContent()" class="rounded border-gray-300">
                                                    <span>Static</span>
                                                </label>
                                            </template>
                                        </div>

                                        {# Query mode: parameters and preview #}
                                        <template x-if="isQueryMode">
                                            <div class="mt-3 pt-3 border-t border-gray-200 space-y-2">
                                                {# Query parameters #}
                                                <div class="flex items-start gap-2 flex-wrap">
                                                    <span class="text-xs text-gray-500 pt-1">Params:</span>
                                                    <template x-for="(value, key) in queryParams" :key="key">
                                                        <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-gray-100 rounded text-xs">
                                                            <span class="font-mono" x-text="key + '=' + value"></span>
                                                            <button @click="removeQueryParam(key)" class="text-gray-400 hover:text-red-500">&times;</button>
                                                        </span>
                                                    </template>
                                                    <button @click="addQueryParam()" class="text-xs text-blue-600 hover:underline">+ param</button>
                                                </div>
                                                {# Preview info #}
                                                <div class="flex items-center gap-2 text-xs text-gray-500">
                                                    <span x-text="queryColumns.length + ' cols, ' + queryRows.length + ' rows'"></span>
                                                    <button @click="manualRefresh()" :disabled="queryLoading" class="text-blue-600 hover:underline disabled:opacity-50">
                                                        <span x-show="!queryLoading">refresh</span>
                                                        <span x-show="queryLoading">...</span>
                                                    </button>
                                                    <span x-show="queryError" class="text-red-500" x-text="queryError"></span>
                                                </div>
                                            </div>
                                        </template>
                                    </div>

                                    {# Manual mode editor #}
                                    <template x-if="!isQueryMode">
                                        <div class="space-y-3">
                                            <div>
                                                <p class="text-sm font-medium text-gray-700 mb-1">Columns</p>
                                                <div class="space-y-1">
                                                    <template x-for="(col, idx) in columns" :key="col.id">
                                                        <div class="flex items-center gap-2">
                                                            <input
                                                                type="text"
                                                                x-model="col.label"
                                                                @blur="saveContent()"
                                                                class="flex-1 p-1 border border-gray-300 rounded text-sm"
                                                                placeholder="Column label"
                                                            >
                                                            <button @click="removeColumn(idx)" class="text-red-500 hover:text-red-700 text-sm">&times;</button>
                                                        </div>
                                                    </template>
                                                    <button @click="addColumn()" class="text-sm text-blue-600 hover:underline">+ Add column</button>
                                                </div>
                                            </div>
                                            <div>
                                                <p class="text-sm font-medium text-gray-700 mb-1">Rows</p>
                                                <div class="space-y-1">
                                                    <template x-for="(row, rowIdx) in rows" :key="row.id">
                                                        <div class="flex items-center gap-2">
                                                            <template x-for="col in columns" :key="col.id">
                                                                <input
                                                                    type="text"
                                                                    x-model="row[col.id]"
                                                                    @blur="saveContent()"
                                                                    class="flex-1 p-1 border border-gray-300 rounded text-sm"
                                                                    :placeholder="col.label"
                                                                >
                                                            </template>
                                                            <button @click="removeRow(rowIdx)" class="text-red-500 hover:text-red-700 text-sm">&times;</button>
                                                        </div>
                                                    </template>
                                                    <button @click="addRow()" class="text-sm text-blue-600 hover:underline">+ Add row</button>
                                                </div>
                                            </div>
                                        </div>
                                    </template>

                                </div>
                            </template>
                        </div>
                    </template>
                </div>
            </div>
        </template>

        {# Empty state #}
        <div x-show="blocks.length === 0 && !loading" class="text-center py-8 text-gray-500">
            <p>No blocks yet.</p>
            <p x-show="editMode" class="text-sm mt-2">Click "Add Block" below to get started.</p>
        </div>

        {# Add block picker (edit mode only) #}
        <div x-show="editMode" class="mt-4" x-data="{ open: false }">
            <div class="relative">
                <button
                    @click="open = !open"
                    class="w-full py-2 border-2 border-dashed border-gray-300 rounded-lg text-gray-500 hover:border-blue-400 hover:text-blue-500 transition-colors"
                >
                    + Add Block
                </button>
                <div
                    x-show="open"
                    @click.away="open = false"
                    x-transition
                    class="absolute z-10 mt-2 w-full bg-white border border-gray-200 rounded-lg shadow-lg py-2"
                >
                    <template x-for="bt in blockTypes" :key="bt.type">
                        <button
                            @click="addBlock(bt.type); open = false"
                            class="w-full px-4 py-2 text-left hover:bg-gray-50 flex items-center gap-2"
                        >
                            <span x-text="bt.icon"></span>
                            <span x-text="bt.label"></span>
                        </button>
                    </template>
                </div>
            </div>
        </div>
    </div>

    {# Resource Picker Modal #}
    <div x-show="$store.resourcePicker.isOpen"
         x-cloak
         class="fixed inset-0 z-50 overflow-y-auto"
         role="dialog"
         aria-modal="true"
         aria-labelledby="resource-picker-title"
         @keydown.escape.window="$store.resourcePicker.close()">
        {# Backdrop #}
        <div class="fixed inset-0 bg-black bg-opacity-50 transition-opacity"
             @click="$store.resourcePicker.close()"></div>

        {# Modal content #}
        <div class="flex min-h-full items-center justify-center p-4">
            <div class="relative bg-white rounded-lg shadow-xl w-full max-w-3xl max-h-[80vh] flex flex-col"
                 @click.stop
                 x-trap.noscroll="$store.resourcePicker.isOpen">
                {# Header #}
                <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200">
                    <h2 id="resource-picker-title" class="text-lg font-semibold text-gray-900">Select Resources</h2>
                    <button @click="$store.resourcePicker.close()"
                            class="text-gray-400 hover:text-gray-600"
                            aria-label="Close">
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                        </svg>
                    </button>
                </div>

                {# Tabs #}
                <div class="flex border-b border-gray-200 px-4" role="tablist">
                    <button @click="$store.resourcePicker.activeTab = 'note'"
                            :class="$store.resourcePicker.activeTab === 'note' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'"
                            class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
                            :disabled="!$store.resourcePicker.noteId"
                            :class="{ 'opacity-50 cursor-not-allowed': !$store.resourcePicker.noteId }"
                            role="tab"
                            :aria-selected="$store.resourcePicker.activeTab === 'note'">
                        Note's Resources
                        <span x-show="$store.resourcePicker.noteResources.length > 0"
                              class="ml-1 text-xs bg-gray-100 px-1.5 py-0.5 rounded"
                              x-text="$store.resourcePicker.noteResources.length"></span>
                    </button>
                    <button @click="$store.resourcePicker.activeTab = 'all'"
                            :class="$store.resourcePicker.activeTab === 'all' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'"
                            class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
                            role="tab"
                            :aria-selected="$store.resourcePicker.activeTab === 'all'">
                        All Resources
                    </button>
                </div>

                {# Filters (All Resources tab only) #}
                <div x-show="$store.resourcePicker.activeTab === 'all'" class="px-4 py-3 border-b border-gray-200 space-y-2">
                    {# Search #}
                    <div>
                        <input type="text"
                               x-model="$store.resourcePicker.searchQuery"
                               @input="$store.resourcePicker.onSearchInput()"
                               placeholder="Search by name..."
                               class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-blue-500 focus:border-blue-500">
                    </div>
                    {# Tag & Group filters #}
                    <div class="flex gap-3">
                        {# Tag filter #}
                        <div class="flex-1"
                             x-data="autocompleter({
                                 selectedResults: [],
                                 url: '/v1/tags',
                                 max: 1,
                                 standalone: true,
                                 onSelect: (tag) => $store.resourcePicker.setTagFilter(tag.ID),
                                 onRemove: () => $store.resourcePicker.clearTagFilter()
                             })"
                             @resource-picker-closed.window="selectedResults = []">
                            <label class="block text-xs text-gray-500 mb-1">Tag</label>
                            <div class="relative">
                                <input x-ref="autocompleter"
                                       type="text"
                                       x-bind="inputEvents"
                                       class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                       :placeholder="selectedResults.length ? '' : 'Filter by tag...'"
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
                        {# Group filter #}
                        <div class="flex-1"
                             x-data="autocompleter({
                                 selectedResults: [],
                                 url: '/v1/groups',
                                 max: 1,
                                 standalone: true,
                                 onSelect: (group) => $store.resourcePicker.setGroupFilter(group.ID),
                                 onRemove: () => $store.resourcePicker.clearGroupFilter()
                             })"
                             @resource-picker-closed.window="selectedResults = []">
                            <label class="block text-xs text-gray-500 mb-1">Group</label>
                            <div class="relative">
                                <input x-ref="autocompleter"
                                       type="text"
                                       x-bind="inputEvents"
                                       class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                       :placeholder="selectedResults.length ? '' : 'Filter by group...'"
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
                                            <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-green-100 text-green-800 rounded text-xs">
                                                <span x-text="item.Name" class="truncate max-w-[100px]"></span>
                                                <button type="button" @click="removeItem(item)" class="hover:text-green-600">&times;</button>
                                            </span>
                                        </template>
                                    </div>
                                </template>
                            </div>
                        </div>
                    </div>
                </div>

                {# Resource grid #}
                <div class="flex-1 overflow-y-auto p-4">
                    {# Loading state #}
                    <div x-show="$store.resourcePicker.loading" class="flex items-center justify-center py-12 text-gray-500">
                        <svg class="animate-spin h-6 w-6 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        Loading...
                    </div>

                    {# Error state #}
                    <div x-show="$store.resourcePicker.error && !$store.resourcePicker.loading"
                         class="text-center py-12 text-red-600">
                        <p x-text="$store.resourcePicker.error"></p>
                        <button @click="$store.resourcePicker.loadAllResources()"
                                class="mt-2 text-sm text-blue-600 hover:underline">Try again</button>
                    </div>

                    {# Empty state #}
                    <div x-show="!$store.resourcePicker.loading && !$store.resourcePicker.error && $store.resourcePicker.displayResources.length === 0"
                         class="text-center py-12 text-gray-500">
                        <template x-if="$store.resourcePicker.activeTab === 'note'">
                            <p>No resources attached to this note</p>
                        </template>
                        <template x-if="$store.resourcePicker.activeTab === 'all'">
                            <p>No resources found</p>
                        </template>
                    </div>

                    {# Resource grid #}
                    <div x-show="!$store.resourcePicker.loading && $store.resourcePicker.displayResources.length > 0"
                         class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-3"
                         role="listbox"
                         aria-label="Available resources">
                        <template x-for="resource in $store.resourcePicker.displayResources" :key="resource.ID">
                            <div @click="$store.resourcePicker.toggleSelection(resource.ID)"
                                 class="relative aspect-square bg-gray-100 rounded-lg overflow-hidden cursor-pointer transition-all"
                                 :class="{
                                     'ring-2 ring-blue-500 ring-offset-2': $store.resourcePicker.isSelected(resource.ID),
                                     'opacity-50 cursor-not-allowed': $store.resourcePicker.isAlreadyAdded(resource.ID),
                                     'hover:ring-2 hover:ring-gray-300': !$store.resourcePicker.isSelected(resource.ID) && !$store.resourcePicker.isAlreadyAdded(resource.ID)
                                 }"
                                 role="option"
                                 :aria-selected="$store.resourcePicker.isSelected(resource.ID)"
                                 :aria-disabled="$store.resourcePicker.isAlreadyAdded(resource.ID)">
                                <img :src="'/v1/resource/preview?id=' + resource.ID"
                                     :alt="resource.Name || 'Resource ' + resource.ID"
                                     class="w-full h-full object-cover"
                                     loading="lazy">
                                {# Selection checkbox #}
                                <div x-show="$store.resourcePicker.isSelected(resource.ID)"
                                     class="absolute top-2 right-2 w-6 h-6 bg-blue-500 rounded-full flex items-center justify-center">
                                    <svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                                    </svg>
                                </div>
                                {# Already added badge #}
                                <div x-show="$store.resourcePicker.isAlreadyAdded(resource.ID)"
                                     class="absolute inset-0 bg-black bg-opacity-40 flex items-center justify-center">
                                    <span class="text-xs text-white bg-black bg-opacity-60 px-2 py-1 rounded">Added</span>
                                </div>
                                {# Resource name tooltip #}
                                <div class="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/60 to-transparent p-2">
                                    <p class="text-xs text-white truncate" x-text="resource.Name || 'Unnamed'"></p>
                                </div>
                            </div>
                        </template>
                    </div>
                </div>

                {# Footer #}
                <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
                    <span class="text-sm text-gray-600">
                        <span x-text="$store.resourcePicker.selectionCount"></span> selected
                    </span>
                    <div class="flex gap-2">
                        <button @click="$store.resourcePicker.close()"
                                type="button"
                                class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">
                            Cancel
                        </button>
                        <button @click="$store.resourcePicker.confirm()"
                                type="button"
                                :disabled="$store.resourcePicker.selectionCount === 0"
                                class="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
                            Confirm
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
