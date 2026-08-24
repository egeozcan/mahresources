{% extends "/layouts/base.tpl" %}

{% block body %}
{% if queryValues.error.0 %}
<div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
  <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
  {# Finding 103: a duplicate upload printed the colliding id as bare text, so #}
  {# there was no way to go look at what it collided with. #}
  {% if queryValues.errorResourceId.0 %}
  <p class="mt-1 text-sm text-red-800">
    <a href="/resource?id={{ queryValues.errorResourceId.0 }}" class="underline hover:text-red-900">Open the existing resource</a>
    — or change the file and try again. The file picker is always cleared after a rejection.
  </p>
  {% endif %}
</div>
{% endif %}
{# The initial URL travels as a data attribute rather than being interpolated  #}
{# into the x-data expression. queryValues is the raw query string, Pongo2      #}
{# escapes a quote to &#39;, and the HTML parser decodes that back to a quote   #}
{# before Alpine evaluates the attribute as JavaScript — so the old inline form #}
{# let /resource/new?URL=... inject script. A dataset value is only read as a   #}
{# string.                                                                      #}
<form
    class="space-y-8"
    method="post"
    x-data="resourceUpload()"
    data-initial-url="{{ queryValues.URL.0 }}"
    data-upload-concurrency="{{ uploadConcurrency }}"
    data-upload-file-threshold="{{ uploadWidgetFileCount }}"
    data-upload-size-threshold="{{ uploadWidgetSizeBytes }}"
    data-max-upload-size="{{ maxUploadSize }}"
    {# No @submit here on purpose: the handler is registered on `document` in    #}
    {# init(), so that schema-form-mode's stopPropagation() on a failed Meta     #}
    {# validation actually prevents the upload. A listener on this element would #}
    {# fire before it — see the comment in resourceUpload.js init().             #}
    :action="url.trim() && background ? '/v1/resource/remote?background=true' : '/v1/resource{% if resource.ID %}/edit{% endif %}'"
    :enctype="url.trim() && background ? 'application/x-www-form-urlencoded' : '{% if !resource.ID %}multipart/form-data{% endif %}'"
>
    {% if resource.ID %}
    <input type="hidden" value="{{ resource.ID }}" name="ID">
    {% endif %}
    <div class="space-y-8 sm:space-y-5">
        <div>
            <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-stone-200">
                    <label for="name" class="block text-sm font-medium font-mono text-stone-700 sm:mt-px sm:pt-2">
                        Name
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="max-w-lg flex rounded-md shadow-sm">
                            <input
                                value="{{ queryValues.Name.0|default:resource.Name }}"
                                type="text"
                                name="Name"
                                placeholder="If you leave this empty, the name of the uploaded file will be used"
                                id="name"
                                autocomplete="off"
                                class="flex-1 block w-full focus:ring-amber-600 focus:border-amber-600 min-w-0 rounded-md sm:text-sm border-stone-300"
                            >
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
                    <label for="description" class="block text-sm font-medium font-mono text-stone-700 sm:mt-px sm:pt-2">
                        Description
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="relative" x-data="mentionTextarea('note,group,tag')">
                            <textarea
                                x-ref="mentionInput"
                                id="description"
                                name="Description"
                                rows="3"
                                @input="onInput($event)"
                                @keydown="onKeydown($event)"
                                {# Deferred-work item 8: role="combobox" and :aria-expanded  #}
                                {# dropped. Native textbox semantics keep the implicit       #}
                                {# aria-multiline that the role override was discarding; see #}
                                {# partials/form/createFormTextareaInput.tpl for why adding  #}
                                {# aria-multiline to the combobox is not the conforming fix. #}
                                aria-autocomplete="list"
                                aria-haspopup="listbox"
                                {# Finding 133: aria-controls was absent here too. #}
                                aria-controls="description-mention-listbox"
                                aria-owns="description-mention-listbox"
                                :aria-activedescendant="activeDescendantId"
                                class="max-w-lg shadow-sm block w-full focus:ring-amber-600 focus:border-amber-600 sm:text-sm border-stone-300 rounded-md font-sans"
                            >{{ queryValues.Description.0|default:resource.Description }}</textarea>
                            {% include "/partials/form/mentionDropdown.tpl" with mentionListboxId="description" %}
                        </div>
                        <p class="mt-2 text-sm text-stone-500 font-sans">Describe the resource.</p>
                    </div>
                </div>

                {% if !resource.ID %}
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <label for="resource" class="block text-sm font-medium font-mono text-stone-700">
                        Resource
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex items-center">
                            {# Finding 142: submitting with no file round-tripped every field #}
                            {# through the query string. It cannot be plainly required — the #}
                            {# URL field below is an alternative source, and filling it makes #}
                            {# the picker deliberately ignored. #}
                            <input
                                id="resource"
                                name="resource"
                                multiple
                                type="file"
                                :required="!url.trim()"
                                aria-describedby="resource-description"
                                @change="onFilesChosen($event)"
                                {# Locked while a batch is running. The files are #}
                                {# already held by the component, so disabling    #}
                                {# takes nothing away — but leaving it open let a #}
                                {# selection made mid-run replace the input       #}
                                {# without being adopted, and picking the same    #}
                                {# files again fires no change event, so the form #}
                                {# stranded with Save disabled.                   #}
                                :disabled="phase === 'uploading'"
                            >
                        </div>
                        <p id="resource-description" class="mt-1 text-sm text-stone-500">Choose one or more files, or give a URL below instead.</p>
                        {# Shown once the selection crosses a threshold, so the switch to #}
                        {# per-file uploads is not a surprise after clicking Save.        #}
                        <p x-show="willUseWidget && !url.trim() && phase === 'idle'" x-cloak
                           data-testid="bulk-upload-hint"
                           class="mt-1 text-sm text-amber-800" x-text="selectionSummary"></p>
                    </div>
                    <label for="URL" class="block text-sm font-medium font-mono text-stone-700">
                        URL
                        <span class="block mt-2 text-sm text-stone-500 font-sans">If you fill this, the contents of the file picker will be ignored and remote data will be downloaded.</span>
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="max-w-lg flex flex-col gap-2">
                            <textarea
                                id="URL"
                                name="URL"
                                x-model="url"
                                placeholder="If you fill this, the contents of the file picker will be ignored and remote data will be downloaded"
                                class="flex-1 block w-full focus:ring-amber-600 focus:border-amber-600 min-w-0 rounded-md sm:text-sm border-stone-300"
                            ></textarea>
                            <div x-show="url.trim()" x-cloak class="flex items-center gap-2">
                                <input
                                    type="checkbox"
                                    id="background"
                                    x-model="background"
                                    class="h-4 w-4 text-amber-700 focus:ring-amber-600 border-stone-300 rounded"
                                >
                                <label for="background" class="text-sm font-mono text-stone-700">
                                    Download in background
                                    <span class="text-stone-500">(track progress in download cockpit)</span>
                                </label>
                            </div>
                        </div>
                    </div>
                </div>

                {% if altFileSystems %}
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <label for="PathName" class="block text-sm font-medium font-mono text-stone-700 sm:mt-px sm:pt-2">
                        Storage
                        <span class="block mt-2 text-sm text-stone-500 font-sans">Choose which filesystem to save this resource to.</span>
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <select
                            id="PathName"
                            name="PathName"
                            data-testid="resource-storage-select"
                            class="max-w-lg block focus:ring-amber-600 focus:border-amber-600 w-full shadow-sm sm:text-sm border-stone-300 rounded-md"
                        >
                            <option value="">Default</option>
                            {% for key, path in altFileSystems %}
                            <option value="{{ key }}">{{ key }}</option>
                            {% endfor %}
                        </select>
                    </div>
                </div>
                {% endif %}
                {% endif %}

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <span class="block text-sm font-medium font-mono text-stone-700">
                        Relations
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with profile='tag' usage='resource' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
                            </div>
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='group' categoryDecoration=true elName='groups' title='Groups' selectedItems=groups id=getNextId("autocompleter") %}
                            </div>
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='note' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
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
                                {% include "/partials/form/autocompleter.tpl" with profile='single' entity='group' categoryDecoration=true elName='ownerId' title='Owner' selectedItems=owner max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <span class="block text-sm font-medium font-mono text-stone-700">
                        Resource Category
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with profile='single' entity='resourceCategory' elName='ResourceCategoryId' title='Resource Category' selectedItems=resourceCategories min=0 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-stone-200 sm:pt-5">
                    <div class="block text-sm font-medium font-mono text-stone-700">
                        Series
                        <p class="mt-2 text-sm text-stone-500 font-sans">Optional. Resources in the same series are grouped together.</p>
                    </div>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with profile='creatable' entity='series' elName='SeriesId' title='Series' selectedItems=series min=0 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                {% set initialResourceSchema = "" %}
                {% if resource.ResourceCategory %}
                    {% set initialResourceSchema = resource.ResourceCategory.MetaSchema %}
                {% elif resourceCategories && resourceCategories.0 %}
                    {% set initialResourceSchema = resourceCategories.0.MetaSchema %}
                {% endif %}

                <div data-initial-schema="{{ initialResourceSchema }}"
                    data-initial-meta='{{ resource.Meta|json }}'
                    x-data="schemaMetaFields({ field: 'ResourceCategoryId' })"
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
                            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' fromJSON=resource.Meta jsonOutput="true" id=getNextId("freeField") %}
                        </div>
                    </template>
                </div>
            </div>
        </div>
    </div>

    {% include "/partials/form/uploadProgressPanel.tpl" %}

    {% include "/partials/form/createFormSubmit.tpl" with busyWhenUploading=true %}
</form>
{% endblock %}
