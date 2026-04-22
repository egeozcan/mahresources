{% extends "/layouts/base.tpl" %}

{% block body %}
{% if queryValues.error.0 %}
<div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
  <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
</div>
{% endif %}
<form class="space-y-8" method="post" action="/v1/note" x-data="{ preview: '{{ note.Preview|base64 }}' }">
    <input type="hidden" value="{{ note.ID }}" name="ID">
    <div class="space-y-8 sm:space-y-5">
        <div>
            <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
                {% include "/partials/form/createFormTextInput.tpl" with title="Title" name="Name" value=queryValues.Name.0|default:note.Name required=true %}
                {% include "/partials/form/createFormTextareaInput.tpl" with title="Text" name="Description" value=queryValues.Description.0|default:note.Description mentionTypes="resource,group,tag" %}


                <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
                    <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-stone-200">
                        <label for="startDate" class="block text-sm font-medium font-mono text-stone-700 sm:mt-px sm:pt-2">
                            Start Date
                        </label>
                        <div class="mt-1 sm:mt-0 sm:col-span-2">
                            <div class="max-w-lg flex rounded-md shadow-sm">
                                <input type="datetime-local" name="startDate" id="startDate" value='{{ note.StartDate|datetime }}' class="flex-1 block w-full focus:ring-amber-600 focus:border-amber-600 min-w-0 rounded-md sm:text-sm border-stone-300">
                            </div>
                        </div>
                    </div>

                    <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
                        <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-stone-200">
                            <label for="endDate" class="block text-sm font-medium font-mono text-stone-700 sm:mt-px sm:pt-2">
                                End Date
                            </label>
                            <div class="mt-1 sm:mt-0 sm:col-span-2">
                                <div class="max-w-lg flex rounded-md shadow-sm">
                                    <input type="datetime-local" name="endDate" id="endDate" value="{{ note.EndDate|datetime }}" class="flex-1 block w-full focus:ring-amber-600 focus:border-amber-600 min-w-0 rounded-md sm:text-sm border-stone-300">
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <span class="block text-sm font-medium font-mono text-stone-700">
                        Relations
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
                            </div>
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id=getNextId("autocompleter") extraInfo="Category" %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <span class="block text-sm font-medium font-mono text-stone-700">
                        Owner
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='ownerId' title='Owner' selectedItems=owner max=1 id=getNextId("autocompleter") extraInfo="Category" %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <span class="block text-sm font-medium font-mono text-stone-700">
                        Note Type
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/note/noteTypes' elName='NoteTypeId' title='Note Type' selectedItems=noteType min=0 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                {% set initialSchema = "" %}
                {% if note.NoteType %}
                    {% set initialSchema = note.NoteType.MetaSchema %}
                {% elif noteType && noteType.0 %}
                    {% set initialSchema = noteType.0.MetaSchema %}
                {% endif %}

                <div data-initial-schema="{{ initialSchema }}"
                    data-initial-meta='{{ note.Meta|json }}'
                    x-data="{
                         currentSchema: null,
                         currentMeta: {},
                         metaEdited: false,
                         init() {
                             const raw = this.$el.dataset.initialSchema;
                             if (raw) {
                                 try { const p = JSON.parse(raw); if (p && typeof p === 'object') this.currentSchema = raw; } catch {}
                             }
                             try { this.currentMeta = JSON.parse(this.$el.dataset.initialMeta || '{}'); } catch { this.currentMeta = {}; }
                         },
                         handleNoteTypeChange(e) {
                             if (e.detail.value.length > 0) {
                                 const ms = e.detail.value[0].MetaSchema;
                                 if (ms) { try { const p = JSON.parse(ms); if (p && typeof p === 'object') { this.currentSchema = ms; return; } } catch {} }
                             }
                             this.currentSchema = null;
                         },
                         handleMetaChange(e) {
                             if (e.detail && e.detail.value !== undefined) {
                                 this.currentMeta = e.detail.value;
                                 this.metaEdited = true;
                             }
                         }
                    }"
                    @multiple-input.window="if ($event.detail.name === 'NoteTypeId') handleNoteTypeChange($event)"
                    class="w-full"
                >
                    <template x-if="currentSchema">
                        <div class="border p-4 rounded-md bg-stone-50 mt-5"
                            @value-change="handleMetaChange($event)">
                            <h2 class="text-sm font-medium font-mono text-stone-700 mb-3">Meta Data (Schema Enforced)</h2>
                            <schema-form-mode
                                :schema="currentSchema"
                                :value="JSON.stringify(currentMeta)"
                                name="Meta"
                            ></schema-form-mode>
                        </div>
                    </template>
                    <template x-if="!currentSchema">
                        <div @value-change="handleMetaChange($event)" :data-current-meta="metaEdited ? JSON.stringify(currentMeta) : ''">
                            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/notes/meta/keys' fromJSON=note.Meta jsonOutput="true" id=getNextId("freeField") %}
                        </div>
                    </template>
                </div>
            </div>
        </div>
    </div>

    <div class="pt-5">
        <div class="flex justify-end">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
                Save
            </button>
        </div>
    </div>
</form>
{% endblock %}