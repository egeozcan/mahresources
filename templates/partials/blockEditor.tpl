{# with noteId= blocks= #}
<div x-data="blockEditor({{ noteId }}, {{ blocks|json }})" x-init="init()" class="block-editor">
    {# Edit mode toggle #}
    <div class="flex justify-end mb-4">
        <button
            @click="toggleEditMode()"
            class="px-3 py-1 text-sm rounded border"
            :class="editMode ? 'bg-amber-700 text-white border-amber-700' : 'bg-white text-stone-700 border-stone-300 hover:bg-stone-50'"
        >
            <span x-text="editMode ? 'Done' : 'Edit Blocks'"></span>
        </button>
    </div>

    {# Loading state #}
    <div x-show="loading" class="text-center py-8 text-stone-500">
        Loading blocks...
    </div>

    {# Error state #}
    <div x-show="error" x-cloak class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm font-sans">
        <div class="flex items-center justify-between">
            <span x-text="error"></span>
            <button @click="error = null" class="text-red-500 hover:text-red-800">&times;</button>
        </div>
    </div>

    {# Blocks list #}
    <div x-show="!loading" class="space-y-4">
        <template x-for="(block, index) in blocks" :key="block.id">
            <div class="block-card bg-white border border-stone-200 rounded-lg overflow-hidden"
                 :class="{ 'ring-2 ring-amber-200': editMode }">
                {# Block controls (edit mode only) #}
                <div x-show="editMode" class="flex items-center justify-between px-3 py-2 bg-stone-50 border-b border-stone-200">
                    <span class="text-xs font-medium font-mono text-stone-500 uppercase" x-text="block.type"></span>
                    <div class="flex gap-1">
                        <button
                            @click="moveBlock(block.id, 'up')"
                            :disabled="index === 0"
                            data-block-control="move-up"
                            :aria-label="'Move block ' + (index + 1) + ' up'"
                            class="p-1 text-stone-400 hover:text-stone-600 disabled:opacity-30 disabled:cursor-not-allowed"
                        >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7"/>
                            </svg>
                        </button>
                        <button
                            @click="moveBlock(block.id, 'down')"
                            :disabled="index === blocks.length - 1"
                            data-block-control="move-down"
                            :aria-label="'Move block ' + (index + 1) + ' down'"
                            class="p-1 text-stone-400 hover:text-stone-600 disabled:opacity-30 disabled:cursor-not-allowed"
                        >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
                            </svg>
                        </button>
                        <button
                            @click="if(confirm('Delete this block?')) deleteBlock(block.id)"
                            data-block-control="delete"
                            :aria-label="'Delete block ' + (index + 1)"
                            class="p-1 text-red-400 hover:text-red-700"
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
                                <div class="prose max-w-none font-sans" x-html="renderMentions(renderMarkdown(block.content?.text || ''))"></div>
                            </template>
                            <template x-if="editMode">
                                <div x-data="blockText(block, (id, content) => updateBlockContent(id, content), (id, content) => updateBlockContentDebounced(id, content))">
                                    <div class="relative" x-data="mentionTextarea('resource,group,tag')" @input="onInput($event)">
                                        <textarea
                                            x-ref="mentionInput"
                                            x-model="text"
                                            @input="onBlockInput()"
                                            @keydown="onKeydown($event)"
                                            @blur="saveBlock()"
                                            class="w-full min-h-[100px] p-2 border border-stone-300 rounded resize-y"
                                            placeholder="Enter text..."
                                            role="combobox"
                                            aria-autocomplete="list"
                                            :aria-expanded="mentionActive && mentionResults.length > 0"
                                            aria-haspopup="listbox"
                                            :aria-activedescendant="activeDescendantId"
                                        ></textarea>
                                        {% include "/partials/form/mentionDropdown.tpl" %}
                                    </div>
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
                                    <select aria-label="Heading level" x-model.number="level" @change="save()" class="border border-stone-300 rounded px-2 py-1">
                                        <option value="1">H1</option>
                                        <option value="2">H2</option>
                                        <option value="3">H3</option>
                                    </select>
                                    <input
                                        type="text"
                                        x-model="text"
                                        @input="onInput()"
                                        @blur="save()"
                                        class="flex-1 p-2 border border-stone-300 rounded"
                                        placeholder="Heading text..."
                                    >
                                </div>
                            </template>
                        </div>
                    </template>

                    {# Divider block #}
                    <template x-if="block.type === 'divider'">
                        <hr class="border-t-2 border-stone-300 my-2">
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
                                                class="h-4 w-4 rounded border-stone-300"
                                            >
                                            <span :class="{ 'line-through text-stone-400': isChecked(item.id) }" x-text="item.label"></span>
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
                                                class="flex-1 p-1 border border-stone-300 rounded"
                                            >
                                            <button @click="removeItem(idx)" class="text-red-700 hover:text-red-800">&times;</button>
                                        </div>
                                    </template>
                                    <button @click="addItem()" class="text-sm text-amber-700 hover:underline">+ Add item</button>
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
                                           class="block aspect-square bg-stone-100 rounded overflow-hidden cursor-pointer hover:opacity-90 transition-opacity">
                                            <img :src="'/v1/resource/preview?id=' + resId"
                                                 :alt="(resourceMeta[resId]?.name) || ('Resource ' + resId)"
                                                 class="w-full h-full object-cover" loading="lazy">
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && resourceIds.length === 0">
                                <p class="text-stone-400 text-sm font-sans">No resources selected</p>
                            </template>
                            <template x-if="editMode">
                                <div class="space-y-3">
                                    {# Selected resources preview #}
                                    <template x-if="resourceIds.length > 0">
                                        <div class="grid grid-cols-4 md:grid-cols-6 gap-2">
                                            <template x-for="(resId, idx) in resourceIds" :key="resId">
                                                <div class="relative group aspect-square bg-stone-100 rounded overflow-hidden">
                                                    <img :src="'/v1/resource/preview?id=' + resId"
                                                         :alt="(resourceMeta[resId]?.name) || ('Resource ' + resId)"
                                                         class="w-full h-full object-cover">
                                                    <button
                                                        @click="removeResource(resId)"
                                                        class="absolute top-1 right-1 w-5 h-5 bg-red-700 text-white rounded-full text-xs opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center"
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
                                        class="w-full py-2 px-4 border-2 border-dashed border-stone-300 rounded-lg text-stone-500 hover:border-amber-400 hover:text-amber-700 transition-colors text-sm"
                                    >
                                        + Select Resources
                                    </button>
                                </div>
                            </template>
                        </div>
                    </template>

                    {# References block #}
                    <template x-if="block.type === 'references'">
                        <div x-data="blockReferences(block, (id, content) => updateBlockContent(id, content), () => editMode)" x-init="init()">
                            <template x-if="!editMode && groupIds.length > 0">
                                <div class="flex flex-wrap gap-2">
                                    <template x-for="gId in groupIds" :key="gId">
                                        <a :href="'/group?id=' + gId"
                                           class="inline-flex items-center gap-1 px-3 py-1.5 bg-amber-50 text-amber-700 rounded-lg text-sm hover:bg-amber-100 border border-amber-200">
                                            <svg class="w-4 h-4 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                                            </svg>
                                            <span class="font-medium" x-text="getGroupDisplay(gId).name"></span>
                                            <span x-show="getGroupDisplay(gId).breadcrumb" class="text-amber-400 text-xs" x-text="'in ' + getGroupDisplay(gId).breadcrumb"></span>
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && groupIds.length === 0">
                                <p class="text-stone-400 text-sm font-sans">No groups selected</p>
                            </template>
                            <template x-if="editMode">
                                <div class="space-y-3">
                                    {# Selected groups preview #}
                                    <template x-if="groupIds.length > 0">
                                        <div class="flex flex-wrap gap-2">
                                            <template x-for="gId in groupIds" :key="gId">
                                                <div class="inline-flex items-center gap-1 px-3 py-1.5 bg-amber-50 text-amber-700 rounded-lg text-sm border border-amber-200">
                                                    <svg class="w-4 h-4 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                                                    </svg>
                                                    <span class="font-medium" x-text="getGroupDisplay(gId).name"></span>
                                                    <button @click="removeGroup(gId)"
                                                            class="ml-1 w-4 h-4 rounded-full bg-amber-200 text-amber-700 hover:bg-amber-300 flex items-center justify-center text-xs"
                                                            title="Remove">&times;</button>
                                                </div>
                                            </template>
                                        </div>
                                    </template>
                                    {# Add groups button #}
                                    <button
                                        @click="openPicker()"
                                        type="button"
                                        class="w-full py-2 px-4 border-2 border-dashed border-stone-300 rounded-lg text-stone-500 hover:border-amber-400 hover:text-amber-700 transition-colors text-sm"
                                    >
                                        + Select Groups
                                    </button>
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
                                        <div class="flex items-center justify-center py-8 text-stone-500">
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
                                        <div class="flex items-center justify-between mb-2 text-xs text-stone-500">
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
                                                <span x-show="isStatic" class="px-1.5 py-0.5 bg-stone-100 rounded text-stone-600">Static</span>
                                            </div>
                                            <button @click="manualRefresh()" :disabled="queryLoading" class="px-2 py-1 text-amber-700 hover:text-amber-800 disabled:opacity-50" title="Refresh data">
                                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
                                                </svg>
                                            </button>
                                        </div>
                                    </template>

                                    {# Table display #}
                                    <template x-if="displayColumns.length > 0 && !queryLoading">
                                        <div class="overflow-x-auto">
                                            <table class="min-w-full divide-y divide-stone-200">
                                                <thead class="bg-stone-50">
                                                    <tr>
                                                        <template x-for="col in displayColumns" :key="col.id">
                                                            <th
                                                                @click="toggleSort(col.id)"
                                                                class="px-3 py-2 text-left text-xs font-medium text-stone-500 uppercase tracking-wider cursor-pointer hover:bg-stone-100"
                                                            >
                                                                <span x-text="col.label"></span>
                                                                <span x-show="sortColumn === col.id" x-text="sortDirection === 'asc' ? ' ▲' : ' ▼'"></span>
                                                            </th>
                                                        </template>
                                                    </tr>
                                                </thead>
                                                <tbody class="bg-white divide-y divide-stone-200">
                                                    <template x-for="row in displayRows" :key="row.id">
                                                        <tr>
                                                            <template x-for="col in displayColumns" :key="col.id">
                                                                <td class="px-3 py-2 text-sm text-stone-900" x-text="row[col.id] ?? ''"></td>
                                                            </template>
                                                        </tr>
                                                    </template>
                                                </tbody>
                                            </table>
                                        </div>
                                    </template>

                                    {# Empty state #}
                                    <template x-if="displayColumns.length === 0 && !queryLoading && !queryError">
                                        <p class="text-stone-400 text-sm">No table data</p>
                                    </template>
                                </div>
                            </template>

                            {# Edit mode #}
                            <template x-if="editMode">
                                <div class="space-y-4">
                                    {# Data Source Section #}
                                    <div class="p-3 bg-stone-50 rounded-lg border border-stone-200"
                                         @table-query-selected="selectQuery($event.detail)"
                                         @table-query-cleared="clearQuery()">
                                        <div class="flex items-center gap-3 flex-wrap">
                                            <label class="text-sm font-medium text-stone-700">Data Source:</label>
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
                                                <template x-if="!addModeForTag">
                                                    <div>
                                                        <input
                                                            x-ref="autocompleter"
                                                            type="text"
                                                            x-bind="inputEvents"
                                                            class="w-full px-2 py-1 text-sm border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600"
                                                            :placeholder="selectedResults.length ? '' : 'Search queries...'"
                                                            autocomplete="off"
                                                        >
                                                        {# Dropdown results #}
                                                        <template x-if="dropdownActive && results.length > 0">
                                                            <div class="absolute z-20 mt-1 w-48 bg-white border border-stone-200 rounded shadow-lg max-h-48 overflow-y-auto">
                                                                <template x-for="(result, index) in results" :key="result.ID">
                                                                    <div
                                                                        class="px-3 py-1.5 cursor-pointer text-sm truncate"
                                                                        :class="{'bg-amber-700 text-white': index === selectedIndex, 'hover:bg-stone-50': index !== selectedIndex}"
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
                                                                    <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-amber-100 text-amber-800 rounded text-xs">
                                                                        <span x-text="item.Name" class="truncate max-w-[150px]"></span>
                                                                        <button type="button" @click="removeItem(item)" class="hover:text-amber-700">&times;</button>
                                                                    </span>
                                                                </template>
                                                            </div>
                                                        </template>
                                                    </div>
                                                </template>
                                            </div>
                                            <template x-if="isQueryMode">
                                                <label class="flex items-center gap-1.5 text-sm text-stone-600">
                                                    <input type="checkbox" x-model="isStatic" @change="saveContent()" class="rounded border-stone-300">
                                                    <span>Static</span>
                                                </label>
                                            </template>
                                        </div>

                                        {# Query mode: parameters and preview #}
                                        <template x-if="isQueryMode">
                                            <div class="mt-3 pt-3 border-t border-stone-200 space-y-2">
                                                {# Query parameters #}
                                                <div class="flex items-start gap-2 flex-wrap">
                                                    <span class="text-xs text-stone-500 pt-1">Params:</span>
                                                    <template x-for="(value, key) in queryParams" :key="key">
                                                        <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-stone-100 rounded text-xs">
                                                            <span class="font-mono" x-text="key + '=' + value"></span>
                                                            <button @click="removeQueryParam(key)" class="text-stone-400 hover:text-red-500">&times;</button>
                                                        </span>
                                                    </template>
                                                    <button @click="addQueryParam()" class="text-xs text-amber-700 hover:underline">+ param</button>
                                                </div>
                                                {# Preview info #}
                                                <div class="flex items-center gap-2 text-xs text-stone-500">
                                                    <span x-text="queryColumns.length + ' cols, ' + queryRows.length + ' rows'"></span>
                                                    <button @click="manualRefresh()" :disabled="queryLoading" class="text-amber-700 hover:underline disabled:opacity-50">
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
                                                <p class="text-sm font-medium text-stone-700 mb-1">Columns</p>
                                                <div class="space-y-1">
                                                    <template x-for="(col, idx) in columns" :key="col.id">
                                                        <div class="flex items-center gap-2">
                                                            <input
                                                                type="text"
                                                                x-model="col.label"
                                                                @blur="saveContent()"
                                                                class="flex-1 p-1 border border-stone-300 rounded text-sm"
                                                                placeholder="Column label"
                                                            >
                                                            <button @click="removeColumn(idx)" class="text-red-700 hover:text-red-800 text-sm">&times;</button>
                                                        </div>
                                                    </template>
                                                    <button @click="addColumn()" class="text-sm text-amber-700 hover:underline">+ Add column</button>
                                                </div>
                                            </div>
                                            <div>
                                                <p class="text-sm font-medium text-stone-700 mb-1">Rows</p>
                                                <div class="space-y-1">
                                                    <template x-for="(row, rowIdx) in rows" :key="row.id">
                                                        <div class="flex items-center gap-2">
                                                            <template x-for="col in columns" :key="col.id">
                                                                <input
                                                                    type="text"
                                                                    x-model="row[col.id]"
                                                                    @blur="saveContent()"
                                                                    class="flex-1 p-1 border border-stone-300 rounded text-sm"
                                                                    :placeholder="col.label"
                                                                >
                                                            </template>
                                                            <button @click="removeRow(rowIdx)" class="text-red-700 hover:text-red-800 text-sm">&times;</button>
                                                        </div>
                                                    </template>
                                                    <button @click="addRow()" class="text-sm text-amber-700 hover:underline">+ Add row</button>
                                                </div>
                                            </div>
                                        </div>
                                    </template>

                                </div>
                            </template>
                        </div>
                    </template>

                    {# Calendar block #}
                    <template x-if="block.type === 'calendar'">
                        <div x-data="blockCalendar(block, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state), () => editMode, noteId)" x-init="init()">
                            {# View mode #}
                            <template x-if="!editMode">
                                <div class="calendar-block">
                                    {# Header #}
                                    <div class="flex items-center justify-between mb-4">
                                        <div class="flex items-center gap-2">
                                            {# Month navigation - only shown in month view #}
                                            <template x-if="view === 'month'">
                                                <div class="flex items-center gap-2">
                                                    <button @click="prevMonth()" class="p-1 hover:bg-stone-100 rounded" title="Previous">
                                                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
                                                        </svg>
                                                    </button>
                                                    <span class="text-lg font-semibold w-36 text-center" x-text="currentMonth + ' ' + currentYear"></span>
                                                    <button @click="nextMonth()" class="p-1 hover:bg-stone-100 rounded" title="Next">
                                                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                                                        </svg>
                                                    </button>
                                                </div>
                                            </template>
                                            {# Agenda title #}
                                            <template x-if="view === 'agenda'">
                                                <span class="text-lg font-semibold">Upcoming Events</span>
                                            </template>
                                        </div>
                                        <div class="flex items-center gap-2">
                                            <template x-if="isRefreshing">
                                                <span class="text-xs text-stone-400 flex items-center">
                                                    <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                                                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                                                    </svg>
                                                </span>
                                            </template>
                                            <button @click="openEventModalForDay(currentDate)"
                                                    class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800">
                                                + Add Event
                                            </button>
                                            <div class="flex border border-stone-200 rounded overflow-hidden text-sm">
                                                <button @click="setView('month')" class="px-3 py-1" :class="view === 'month' ? 'bg-amber-700 text-white' : 'bg-white hover:bg-stone-50'">Month</button>
                                                <button @click="setView('agenda')" class="px-3 py-1" :class="view === 'agenda' ? 'bg-amber-700 text-white' : 'bg-white hover:bg-stone-50'">Agenda</button>
                                            </div>
                                        </div>
                                    </div>

                                    {# Error #}
                                    <template x-if="error">
                                        <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm mb-4">
                                            <span x-text="error"></span>
                                            <button @click="fetchEvents(true)" class="ml-2 underline">Retry</button>
                                        </div>
                                    </template>

                                    {# Month view - show even while loading to prevent layout jump #}
                                    <template x-if="view === 'month'">
                                        <div class="relative" :class="{ 'opacity-60': loading }">
                                            <div class="grid grid-cols-7 gap-px bg-stone-200 rounded overflow-hidden">
                                                <template x-for="day in ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']">
                                                    <div class="bg-stone-50 py-2 text-center text-xs font-medium text-stone-500" x-text="day"></div>
                                                </template>
                                                <template x-for="day in monthDays" :key="day.date.toISOString()">
                                                    <div class="bg-white min-h-[80px] p-1 relative cursor-pointer hover:bg-amber-50 transition-colors"
                                                         @click="openEventModalForDay(day.date)"
                                                         :class="{ 'bg-stone-50 hover:bg-stone-100': !day.isCurrentMonth, 'ring-2 ring-amber-600 ring-inset': isToday(day.date) }">
                                                        <span class="text-xs" :class="day.isCurrentMonth ? 'text-stone-700' : 'text-stone-400'" x-text="day.date.getDate()"></span>
                                                        <div class="mt-1 space-y-0.5">
                                                            <template x-for="event in getEventsForDay(day.date).slice(0, 3)" :key="event.id">
                                                                <div @click.stop="isCustomEvent(event) ? openEventModalForEdit(event) : null"
                                                                     class="text-xs px-1 py-0.5 rounded truncate"
                                                                     :class="isCustomEvent(event) ? 'cursor-pointer hover:opacity-80' : ''"
                                                                     :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                                                     :title="event.title + (event.location ? ' @ ' + event.location : '') + (isCustomEvent(event) ? ' (click to edit)' : '')"
                                                                     x-text="event.allDay ? event.title : formatEventTime(event) + ' ' + event.title">
                                                                </div>
                                                            </template>
                                                            <template x-if="getEventsForDay(day.date).length > 3">
                                                                <div @click.stop="toggleExpandedDay(day.date)"
                                                                     class="text-xs text-amber-700 hover:text-amber-800 px-1 cursor-pointer"
                                                                     x-text="'+' + (getEventsForDay(day.date).length - 3) + ' more'"></div>
                                                            </template>
                                                        </div>
                                                        {# Expanded events popover #}
                                                        <template x-if="isExpanded(day.date)">
                                                            <div class="absolute z-20 left-0 top-full mt-1 w-64 bg-white border border-stone-200 rounded-lg shadow-lg p-2"
                                                                 @click.stop
                                                                 @click.away="closeExpandedDay()">
                                                                <div class="flex justify-between items-center mb-2 pb-1 border-b">
                                                                    <span class="text-sm font-medium" x-text="day.date.toLocaleDateString('default', { weekday: 'short', month: 'short', day: 'numeric' })"></span>
                                                                    <button @click="closeExpandedDay()" class="text-stone-400 hover:text-stone-600">
                                                                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                                                                        </svg>
                                                                    </button>
                                                                </div>
                                                                <div class="space-y-1 max-h-48 overflow-y-auto">
                                                                    <template x-for="event in getEventsForDay(day.date)" :key="event.id">
                                                                        <div @click.stop="if (isCustomEvent(event)) { openEventModalForEdit(event); closeExpandedDay(); }"
                                                                             class="text-xs px-2 py-1 rounded"
                                                                             :class="isCustomEvent(event) ? 'cursor-pointer hover:opacity-80' : ''"
                                                                             :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)">
                                                                            <div class="font-medium" x-text="event.title"></div>
                                                                            <div class="opacity-75" x-text="formatEventTime(event)"></div>
                                                                        </div>
                                                                    </template>
                                                                </div>
                                                                <button @click="openEventModalForDay(day.date); closeExpandedDay()"
                                                                        class="w-full mt-2 pt-1 border-t text-xs text-amber-700 hover:text-amber-800">
                                                                    + Add event
                                                                </button>
                                                            </div>
                                                        </template>
                                                    </div>
                                                </template>
                                            </div>
                                        </div>
                                    </template>

                                    {# Agenda view - show even while loading to prevent layout jump #}
                                    <template x-if="view === 'agenda'">
                                        <div class="space-y-4 relative" :class="{ 'opacity-60': loading }">
                                            <template x-if="agendaEvents.length === 0">
                                                <div class="text-center py-8 text-stone-400">No upcoming events</div>
                                            </template>
                                            <template x-for="group in agendaEvents" :key="group.date.toISOString()">
                                                <div>
                                                    <div class="text-sm font-medium text-stone-600 mb-2" x-text="formatAgendaDate(group.date)"></div>
                                                    <div class="space-y-2">
                                                        <template x-for="event in group.events" :key="event.id">
                                                            <div @click="isCustomEvent(event) ? openEventModalForEdit(event) : goToEventMonth(event)"
                                                                 class="flex items-start gap-3 p-2 rounded hover:bg-stone-50 cursor-pointer"
                                                                 :title="isCustomEvent(event) ? 'Click to edit' : 'Click to view in month'">
                                                                <div class="w-1 h-full min-h-[40px] rounded" :style="'background-color: ' + getCalendarColor(event.calendarId)"></div>
                                                                <div class="flex-1 min-w-0">
                                                                    <div class="font-medium text-sm flex items-center gap-1">
                                                                        <span x-text="event.title"></span>
                                                                        <template x-if="isCustomEvent(event)">
                                                                            <svg class="w-3 h-3 text-stone-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"/>
                                                                            </svg>
                                                                        </template>
                                                                    </div>
                                                                    <div class="text-xs text-stone-500">
                                                                        <span x-text="formatEventTime(event)"></span>
                                                                        <span x-show="event.location" class="ml-2">@ <span x-text="event.location"></span></span>
                                                                    </div>
                                                                    <div x-show="event.description" class="text-xs text-stone-400 mt-1 line-clamp-2" x-text="event.description"></div>
                                                                </div>
                                                                <div class="text-xs px-2 py-0.5 rounded"
                                                                     :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                                                     x-text="getCalendarName(event.calendarId)">
                                                                </div>
                                                            </div>
                                                        </template>
                                                    </div>
                                                </div>
                                            </template>
                                        </div>
                                    </template>

                                    {# Empty state #}
                                    <template x-if="calendars.length === 0 && customEvents.length === 0 && !loading">
                                        <div class="text-center py-8 text-stone-400">
                                            <p>No calendars or events yet.</p>
                                            <p class="text-sm mt-1">Click "+ Add Event" to create an event or "Edit Blocks" to add calendars.</p>
                                        </div>
                                    </template>

                                    {# Event Modal #}
                                    <template x-if="showEventModal">
                                        <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true" aria-labelledby="event-modal-heading" @keydown.escape.window="closeEventModal()">
                                            <div class="absolute inset-0 bg-black/50" @click="closeEventModal()"></div>
                                            <div class="relative bg-white rounded-lg shadow-xl w-full max-w-md mx-4 p-6">
                                                <h3 id="event-modal-heading" class="text-lg font-semibold mb-4" x-text="editingEvent ? 'Edit Event' : 'New Event'"></h3>

                                                <form @submit.prevent="saveEvent()">
                                                    {# Title #}
                                                    <div class="mb-4">
                                                        <label for="event-title" class="block text-sm font-medium font-mono text-stone-700 mb-1">Title</label>
                                                        <input id="event-title" type="text" x-model="eventForm.title" required
                                                               class="w-full px-3 py-2 border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600">
                                                    </div>

                                                    {# All Day toggle #}
                                                    <label class="flex items-center gap-2 mb-4 cursor-pointer">
                                                        <input type="checkbox" x-model="eventForm.allDay" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                                                        <span class="text-sm">All day event</span>
                                                    </label>

                                                    {# Start date/time #}
                                                    <div class="grid grid-cols-2 gap-3 mb-4">
                                                        <div>
                                                            <label for="event-start-date" class="block text-sm font-medium font-mono text-stone-700 mb-1">Start Date</label>
                                                            <input id="event-start-date" type="date" x-model="eventForm.startDate" required
                                                                   class="w-full px-3 py-2 border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600">
                                                        </div>
                                                        <div x-show="!eventForm.allDay">
                                                            <label for="event-start-time" class="block text-sm font-medium font-mono text-stone-700 mb-1">Start Time</label>
                                                            <input id="event-start-time" type="time" x-model="eventForm.startTime"
                                                                   class="w-full px-3 py-2 border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600">
                                                        </div>
                                                    </div>

                                                    {# End date/time #}
                                                    <div class="grid grid-cols-2 gap-3 mb-4">
                                                        <div>
                                                            <label for="event-end-date" class="block text-sm font-medium font-mono text-stone-700 mb-1">End Date</label>
                                                            <input id="event-end-date" type="date" x-model="eventForm.endDate" required
                                                                   class="w-full px-3 py-2 border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600">
                                                        </div>
                                                        <div x-show="!eventForm.allDay">
                                                            <label for="event-end-time" class="block text-sm font-medium font-mono text-stone-700 mb-1">End Time</label>
                                                            <input id="event-end-time" type="time" x-model="eventForm.endTime"
                                                                   class="w-full px-3 py-2 border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600">
                                                        </div>
                                                    </div>

                                                    {# Location #}
                                                    <div class="mb-4">
                                                        <label for="event-location" class="block text-sm font-medium font-mono text-stone-700 mb-1">Location (optional)</label>
                                                        <input id="event-location" type="text" x-model="eventForm.location"
                                                               class="w-full px-3 py-2 border border-stone-300 rounded focus:ring-amber-600 focus:border-amber-600">
                                                    </div>

                                                    {# Description #}
                                                    <div class="mb-4">
                                                        <label for="event-description" class="block text-sm font-medium font-mono text-stone-700 mb-1">Description (optional)</label>
                                                        <textarea id="event-description" x-model="eventForm.description" rows="2"
                                                                  class="w-full px-3 py-2 border border-stone-300 rounded resize-none focus:ring-amber-600 focus:border-amber-600"></textarea>
                                                    </div>

                                                    {# Actions #}
                                                    <div class="flex justify-between pt-2">
                                                        <div>
                                                            <button x-show="editingEvent" type="button" @click="deleteEvent()"
                                                                    class="px-4 py-2 text-red-700 hover:text-red-800 text-sm">
                                                                Delete
                                                            </button>
                                                        </div>
                                                        <div class="flex gap-2">
                                                            <button type="button" @click="closeEventModal()"
                                                                    class="px-4 py-2 border border-stone-300 rounded hover:bg-stone-50 text-sm">Cancel</button>
                                                            <button type="submit"
                                                                    class="px-4 py-2 bg-amber-700 text-white rounded hover:bg-amber-800 text-sm">Save</button>
                                                        </div>
                                                    </div>
                                                </form>
                                            </div>
                                        </div>
                                    </template>
                                </div>
                            </template>

                            {# Edit mode #}
                            <template x-if="editMode">
                                <div class="space-y-4">
                                    {# Configured calendars #}
                                    <div>
                                        <p class="text-sm font-medium text-stone-700 mb-2">Calendars</p>
                                        <template x-if="calendars.length === 0">
                                            <p class="text-sm text-stone-400">No calendars configured</p>
                                        </template>
                                        <div class="space-y-2">
                                            <template x-for="cal in calendars" :key="cal.id">
                                                <div class="flex items-center gap-2 p-2 bg-stone-50 rounded">
                                                    <div class="relative">
                                                        <button @click="showColorPicker = showColorPicker === cal.id ? null : cal.id"
                                                                class="w-6 h-6 rounded border border-stone-300 cursor-pointer"
                                                                :style="'background-color: ' + cal.color">
                                                        </button>
                                                        <template x-if="showColorPicker === cal.id">
                                                            <div class="absolute top-8 left-0 z-10 p-2 bg-white border rounded shadow-lg flex flex-wrap gap-1 w-32">
                                                                <template x-for="color in ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#f97316']">
                                                                    <button @click="updateCalendarColor(cal.id, color)"
                                                                            class="w-6 h-6 rounded border"
                                                                            :style="'background-color: ' + color"
                                                                            :class="cal.color === color ? 'ring-2 ring-offset-1 ring-stone-400' : ''">
                                                                    </button>
                                                                </template>
                                                            </div>
                                                        </template>
                                                    </div>
                                                    <input type="text" :value="cal.name"
                                                           @blur="updateCalendarName(cal.id, $event.target.value)"
                                                           class="flex-1 px-2 py-1 text-sm border border-stone-300 rounded">
                                                    <span class="text-xs text-stone-400" x-text="cal.source.type === 'url' ? 'URL' : 'File'"></span>
                                                    <button @click="removeCalendar(cal.id)" class="text-red-700 hover:text-red-800 p-1" title="Remove">
                                                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                                                        </svg>
                                                    </button>
                                                </div>
                                            </template>
                                        </div>
                                    </div>

                                    {# Add calendar #}
                                    <div class="pt-2 border-t border-stone-200">
                                        <p class="text-sm font-medium text-stone-700 mb-2">Add Calendar</p>
                                        <div class="space-y-2">
                                            <div class="flex gap-2">
                                                <input type="url" x-model="newUrl"
                                                       @keydown.enter="addCalendarFromUrl()"
                                                       placeholder="Paste ICS calendar URL..."
                                                       class="flex-1 px-3 py-2 text-sm border border-stone-300 rounded">
                                                <button @click="addCalendarFromUrl()"
                                                        :disabled="!newUrl.trim()"
                                                        class="px-3 py-2 bg-amber-700 text-white text-sm rounded hover:bg-amber-800 disabled:opacity-50 disabled:cursor-not-allowed">
                                                    Add URL
                                                </button>
                                            </div>
                                            <button @click="openResourcePicker()"
                                                    class="w-full py-2 px-4 border-2 border-dashed border-stone-300 rounded-lg text-stone-500 hover:border-amber-400 hover:text-amber-700 transition-colors text-sm">
                                                + Select ICS File from Resources
                                            </button>
                                        </div>
                                    </div>
                                </div>
                            </template>
                        </div>
                    </template>

                    {# Plugin block (enabled) #}
                    <template x-if="block.type.startsWith('plugin:') && blockTypes.find(bt => bt.type === block.type)">
                        <div x-data="blockPlugin(block, () => editMode)"
                             x-effect="loadRender()">
                            <template x-if="renderLoading && !renderedHtml">
                                <div class="text-stone-400 text-sm py-4 text-center">Loading plugin block...</div>
                            </template>
                            <template x-if="renderError">
                                <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm" x-text="renderError"></div>
                            </template>
                            <template x-if="renderedHtml">
                                <div x-html="renderedHtml" class="plugin-block-content"></div>
                            </template>
                        </div>
                    </template>

                    {# Plugin block (unavailable - plugin disabled) #}
                    <template x-if="block.type.startsWith('plugin:') && !blockTypes.find(bt => bt.type === block.type)">
                        <div class="p-4 bg-stone-50 border border-stone-200 rounded text-stone-500 text-sm">
                            This block requires the "<span x-text="block.type.split(':')[1]"></span>" plugin which is not currently enabled.
                        </div>
                    </template>
                </div>
            </div>
        </template>

        {# Empty state #}
        <div x-show="blocks.length === 0 && !loading" class="text-center py-8 text-stone-500">
            <p>No blocks yet.</p>
            <p x-show="editMode" class="text-sm mt-2">Click "Add Block" below to get started.</p>
        </div>

        {# Add block picker (edit mode only) #}
        <div x-show="editMode" class="mt-4">
            <div class="relative">
                <button
                    data-testid="add-block-trigger"
                    type="button"
                    :aria-expanded="addBlockPickerOpen.toString()"
                    aria-haspopup="listbox"
                    aria-controls="add-block-listbox"
                    @click="addBlockPickerOpen = !addBlockPickerOpen"
                    @keydown.escape="addBlockPickerOpen = false"
                    class="w-full py-2 border-2 border-dashed border-stone-300 rounded-lg text-stone-500 hover:border-amber-400 hover:text-amber-700 transition-colors"
                >
                    + Add Block
                </button>
                <ul
                    id="add-block-listbox"
                    role="listbox"
                    aria-label="Block types"
                    x-show="addBlockPickerOpen"
                    @click.away="addBlockPickerOpen = false"
                    @keydown.escape="addBlockPickerOpen = false"
                    x-transition
                    class="absolute z-10 mt-2 w-full bg-white border border-stone-200 rounded-lg shadow-lg py-2 list-none"
                >
                    <template x-for="(bt, idx) in blockTypes" :key="bt.type">
                        <li
                            role="option"
                            :tabindex="idx === activePickerIndex ? 0 : -1"
                            :aria-selected="idx === activePickerIndex"
                            :data-block-type="bt.type"
                            @click="addBlock(bt.type); addBlockPickerOpen = false"
                            @keydown.enter.prevent="addBlock(bt.type); addBlockPickerOpen = false"
                            @keydown.arrow-down.prevent="focusPickerItem(Math.min(activePickerIndex + 1, blockTypes.length - 1))"
                            @keydown.arrow-up.prevent="focusPickerItem(Math.max(activePickerIndex - 1, 0))"
                            @keydown.home.prevent="focusPickerItem(0)"
                            @keydown.end.prevent="focusPickerItem(blockTypes.length - 1)"
                            class="w-full px-4 py-2 text-left hover:bg-stone-50 flex items-center gap-2 cursor-pointer"
                        >
                            <span x-text="bt.icon"></span>
                            <span x-text="bt.label"></span>
                            <span x-show="bt.description" x-text="bt.description" class="text-xs text-stone-400 ml-auto truncate max-w-[200px]"></span>
                        </li>
                    </template>
                </ul>
            </div>
        </div>
    </div>

    {# Live region for screen reader announcements #}
    <div x-ref="liveRegion" class="sr-only" aria-live="polite" aria-atomic="true"></div>


</div>
