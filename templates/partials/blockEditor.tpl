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
                        <div x-data="blockText(block, editMode, (id, content) => updateBlockContent(id, content))">
                            <template x-if="!editMode">
                                <div class="prose max-w-none" x-html="renderMarkdown(block.content?.text || '')"></div>
                            </template>
                            <template x-if="editMode">
                                <textarea
                                    x-model="text"
                                    @blur="save()"
                                    class="w-full min-h-[100px] p-2 border border-gray-300 rounded resize-y"
                                    placeholder="Enter text..."
                                ></textarea>
                            </template>
                        </div>
                    </template>

                    {# Heading block #}
                    <template x-if="block.type === 'heading'">
                        <div x-data="blockHeading(block, editMode, (id, content) => updateBlockContent(id, content))">
                            <template x-if="!editMode">
                                <div>
                                    <h1 x-show="block.content?.level === 1" x-text="block.content?.text || ''" class="text-3xl font-bold"></h1>
                                    <h2 x-show="block.content?.level === 2 || !block.content?.level" x-text="block.content?.text || ''" class="text-2xl font-bold"></h2>
                                    <h3 x-show="block.content?.level === 3" x-text="block.content?.text || ''" class="text-xl font-bold"></h3>
                                </div>
                            </template>
                            <template x-if="editMode">
                                <div class="flex gap-2">
                                    <select x-model.number="level" @change="save()" class="border border-gray-300 rounded px-2 py-1">
                                        <option value="1">H1</option>
                                        <option value="2">H2</option>
                                        <option value="3">H3</option>
                                    </select>
                                    <input
                                        type="text"
                                        x-model="text"
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
                        <div x-data="blockTodos(block, editMode, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state))">
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
                        <div x-data="blockGallery(block, editMode, (id, content) => updateBlockContent(id, content))">
                            <template x-if="!editMode && resourceIds.length > 0">
                                <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
                                    <template x-for="resId in resourceIds" :key="resId">
                                        <a :href="'/resource?id=' + resId" class="block aspect-square bg-gray-100 rounded overflow-hidden">
                                            <img :src="'/v1/resource/thumbnail?id=' + resId" class="w-full h-full object-cover" loading="lazy">
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && resourceIds.length === 0">
                                <p class="text-gray-400 text-sm">No resources selected</p>
                            </template>
                            <template x-if="editMode">
                                <div>
                                    <p class="text-sm text-gray-500 mb-2">Resource IDs (comma-separated):</p>
                                    <input
                                        type="text"
                                        :value="resourceIds.join(', ')"
                                        @blur="updateResourceIds($event.target.value)"
                                        class="w-full p-2 border border-gray-300 rounded"
                                        placeholder="e.g., 1, 2, 3"
                                    >
                                </div>
                            </template>
                        </div>
                    </template>

                    {# References block #}
                    <template x-if="block.type === 'references'">
                        <div x-data="blockReferences(block, editMode, (id, content) => updateBlockContent(id, content))">
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
                        <div x-data="blockTable(block, editMode, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state))">
                            <template x-if="!editMode && columns.length > 0">
                                <div class="overflow-x-auto">
                                    <table class="min-w-full divide-y divide-gray-200">
                                        <thead class="bg-gray-50">
                                            <tr>
                                                <template x-for="col in columns" :key="col.id">
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
                                            <template x-for="row in sortedRows" :key="row.id">
                                                <tr>
                                                    <template x-for="col in columns" :key="col.id">
                                                        <td class="px-3 py-2 text-sm text-gray-900" x-text="row[col.id] || ''"></td>
                                                    </template>
                                                </tr>
                                            </template>
                                        </tbody>
                                    </table>
                                </div>
                            </template>
                            <template x-if="!editMode && columns.length === 0">
                                <p class="text-gray-400 text-sm">No table data</p>
                            </template>
                            <template x-if="editMode">
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
</div>
