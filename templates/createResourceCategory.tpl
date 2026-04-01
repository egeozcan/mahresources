{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/resourceCategory">
    {% if resourceCategory.ID %}
    <input type="hidden" value="{{ resourceCategory.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=resourceCategory.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=resourceCategory.Description %}

    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Header" name="CustomHeader" value=resourceCategory.CustomHeader %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=resourceCategory.CustomSidebar %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Summary" name="CustomSummary" value=resourceCategory.CustomSummary %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=resourceCategory.CustomAvatar %}
    <div class="meta-schema-field" x-data="schemaEditorModal()">
        <div class="flex gap-2 items-start">
            <div class="flex-1">
                {% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=resourceCategory.MetaSchema big=true id="rcMetaSchemaTextarea" %}
            </div>
            <button type="button" class="visual-editor-btn mt-6 inline-flex items-center px-3 py-2 border border-stone-300 shadow-sm text-sm font-medium font-mono rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600" @click="openModal('rcMetaSchemaTextarea')">
                Visual Editor
            </button>
        </div>

        <!-- Modal -->
        <template x-if="open">
            <div class="fixed inset-0 z-50 flex items-center justify-center" @keydown.escape="closeModal()">
                <div class="absolute inset-0 bg-black/40" @click="closeModal()"></div>
                <div x-ref="modalContent" class="relative bg-white rounded-lg shadow-2xl flex flex-col" style="width:90vw;max-width:1400px;height:80vh;" role="dialog" aria-modal="true" aria-label="Meta JSON Schema Editor">
                    <!-- Header -->
                    <div class="flex items-center border-b border-stone-200 px-4 bg-stone-50 rounded-t-lg">
                        <h3 class="text-sm font-medium font-mono text-stone-700 py-3 mr-6">Meta JSON Schema</h3>
                        <div class="flex gap-0 -mb-px">
                            <button type="button" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'edit' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'edit'">Edit Schema</button>
                            <button type="button" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'preview' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'preview'">Preview Form</button>
                            <button type="button" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'raw' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'raw'">Raw JSON</button>
                        </div>
                        <div class="flex-1"></div>
                        <button type="button" class="text-stone-400 hover:text-stone-600 text-lg" @click="closeModal()" aria-label="Close">&times;</button>
                    </div>
                    <!-- Body -->
                    <div class="flex-1 overflow-hidden">
                        <template x-if="tab === 'edit'">
                            <schema-editor mode="edit" :schema="currentSchema" @schema-change="handleSchemaChange($event)" style="height:100%;"></schema-editor>
                        </template>
                        <template x-if="tab === 'preview'">
                            <div class="p-6 overflow-y-auto h-full">
                                <schema-editor mode="form" :schema="currentSchema" value="{}" name="_preview"></schema-editor>
                            </div>
                        </template>
                        <template x-if="tab === 'raw'">
                            <textarea x-model="rawJson" @input="handleRawChange()" class="w-full h-full p-4 font-mono text-xs border-none resize-none focus:ring-0" spellcheck="false"></textarea>
                        </template>
                    </div>
                    <!-- Footer -->
                    <div class="flex items-center gap-3 px-4 py-3 border-t border-stone-200 bg-stone-50 rounded-b-lg">
                        <span class="text-xs text-stone-400 font-mono" x-text="getPropertyCount()"></span>
                        <div class="flex-1"></div>
                        <button type="button" class="px-4 py-2 border border-stone-300 rounded-md text-sm font-mono text-stone-700 bg-white hover:bg-stone-50" @click="closeModal()">Cancel</button>
                        <button type="button" class="px-4 py-2 border-none rounded-md text-sm font-mono text-white bg-indigo-700 hover:bg-indigo-800" @click="applySchema()">Apply Schema</button>
                    </div>
                </div>
            </div>
        </template>
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}
