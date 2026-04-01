{# Reusable schema editor modal partial. #}
{# Parameters: textareaId — the id of the MetaSchema textarea to bind to (e.g. "metaSchemaTextarea") #}
<div x-data="schemaEditorModal()">
    <button type="button" class="visual-editor-btn mt-6 inline-flex items-center px-3 py-2 border border-stone-300 shadow-sm text-sm font-medium font-mono rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600" @click="openModal('{{ textareaId }}')">
        Visual Editor
    </button>

    <!-- Modal -->
    <template x-if="open">
        <div class="fixed inset-0 z-50 flex items-center justify-center" @keydown.escape="closeModal()">
            <div class="absolute inset-0 bg-black/40" @click="closeModal()"></div>
            <div x-ref="modalContent" x-trap.noscroll="open" class="relative bg-white rounded-lg shadow-2xl flex flex-col" style="width:90vw;max-width:1400px;height:80vh;max-height:100vh;overflow-x:auto;" role="dialog" aria-modal="true" aria-label="Meta JSON Schema Editor">
                <!-- Header -->
                <div class="flex items-center border-b border-stone-200 px-4 bg-stone-50 rounded-t-lg">
                    <h3 class="text-sm font-medium font-mono text-stone-700 py-3 mr-6">Meta JSON Schema</h3>
                    <div role="tablist" aria-label="Schema editor views" class="flex gap-0 -mb-px" @keydown="handleTabKeydown($event)">
                        <button type="button" role="tab" :aria-selected="tab === 'edit'" :tabindex="tab === 'edit' ? 0 : -1" id="tab-edit" aria-controls="panel-edit" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'edit' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'edit'">Edit Schema</button>
                        <button type="button" role="tab" :aria-selected="tab === 'preview'" :tabindex="tab === 'preview' ? 0 : -1" id="tab-preview" aria-controls="panel-preview" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'preview' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'preview'">Preview Form</button>
                        <button type="button" role="tab" :aria-selected="tab === 'raw'" :tabindex="tab === 'raw' ? 0 : -1" id="tab-raw" aria-controls="panel-raw" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'raw' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'raw'">Raw JSON</button>
                    </div>
                    <div class="flex-1"></div>
                    <button type="button" class="text-stone-400 hover:text-stone-600 text-lg" @click="closeModal()" aria-label="Close">&times;</button>
                </div>
                <!-- Body -->
                <div class="flex-1 overflow-hidden">
                    <template x-if="tab === 'edit'">
                        <div role="tabpanel" id="panel-edit" aria-labelledby="tab-edit" class="h-full">
                            <schema-editor mode="edit" :schema="currentSchema" @schema-change="handleSchemaChange($event)" style="height:100%;"></schema-editor>
                        </div>
                    </template>
                    <template x-if="tab === 'preview'">
                        <div role="tabpanel" id="panel-preview" aria-labelledby="tab-preview" class="p-6 overflow-y-auto h-full">
                            <schema-editor mode="form" :schema="currentSchema" value="{}" name="_preview"></schema-editor>
                        </div>
                    </template>
                    <template x-if="tab === 'raw'">
                        <div role="tabpanel" id="panel-raw" aria-labelledby="tab-raw" class="h-full">
                            <textarea x-model="rawJson" @input="handleRawChange()" class="w-full h-full p-4 font-mono text-xs border-none resize-none focus:ring-0" spellcheck="false"></textarea>
                        </div>
                    </template>
                </div>
                <!-- Footer -->
                <div class="flex items-center gap-3 px-4 py-3 border-t border-stone-200 bg-stone-50 rounded-b-lg">
                    <span class="text-xs text-stone-600 font-mono" x-text="getPropertyCount()"></span>
                    <div class="flex-1"></div>
                    <button type="button" class="px-4 py-2 border border-stone-300 rounded-md text-sm font-mono text-stone-700 bg-white hover:bg-stone-50" @click="closeModal()">Cancel</button>
                    <button type="button" class="px-4 py-2 border-none rounded-md text-sm font-mono text-white bg-indigo-700 hover:bg-indigo-800" @click="applySchema()">Apply Schema</button>
                </div>
            </div>
        </div>
    </template>
</div>
