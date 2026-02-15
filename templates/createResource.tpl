{% extends "/layouts/base.tpl" %}

{% block body %}
<form
    class="space-y-8"
    method="post"
    x-data="{ url: '', background: false }"
    :action="url.trim() && background ? '/v1/resource/remote?background=true' : '/v1/resource{% if resource.ID %}/edit{% endif %}'"
    :enctype="url.trim() && background ? 'application/x-www-form-urlencoded' : '{% if !resource.ID %}multipart/form-data{% endif %}'"
>
    {% if resource.ID %}
    <input type="hidden" value="{{ resource.ID }}" name="ID">
    {% endif %}
    <div class="space-y-8 sm:space-y-5">
        <div>
            <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-gray-200">
                    <label for="name" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
                        Name
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="max-w-lg flex rounded-md shadow-sm">
                            <input
                                value="{{ resource.Name }}"
                                type="text"
                                name="Name"
                                placeholder="If you leave this empty, the name of the uploaded file will be used"
                                id="name"
                                autocomplete="name"
                                class="flex-1 block w-full focus:ring-indigo-500 focus:border-indigo-500 min-w-0 rounded-md sm:text-sm border-gray-300"
                            >
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-gray-200 sm:pt-5">
                    <label for="description" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
                        Description
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <textarea id="description" name="Description" rows="3" class="max-w-lg shadow-sm block w-full focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md">{{ resource.Description }}</textarea>
                        <p class="mt-2 text-sm text-gray-500">Describe the resource.</p>
                    </div>
                </div>

                {% if !resource.ID %}
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <label for="resource" class="block text-sm font-medium text-gray-700">
                        Resource
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex items-center">
                            <input
                                id="resource"
                                name="resource"
                                multiple
                                type="file"
                            >
                        </div>
                    </div>
                    <label for="URL" class="block text-sm font-medium text-gray-700">
                        URL
                        <p class="mt-2 text-sm text-gray-500">If you fill this, the contents of the file picker will be ignored and remote data will be downloaded.</p>
                    </label>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="max-w-lg flex flex-col gap-2">
                            <textarea
                                id="URL"
                                name="URL"
                                x-model="url"
                                placeholder="If you fill this, the contents of the file picker will be ignored and remote data will be downloaded"
                                class="flex-1 block w-full focus:ring-indigo-500 focus:border-indigo-500 min-w-0 rounded-md sm:text-sm border-gray-300"
                            ></textarea>
                            <div x-show="url.trim()" x-cloak class="flex items-center gap-2">
                                <input
                                    type="checkbox"
                                    id="background"
                                    x-model="background"
                                    class="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                                >
                                <label for="background" class="text-sm text-gray-700">
                                    Download in background
                                    <span class="text-gray-500">(track progress in download cockpit)</span>
                                </label>
                            </div>
                        </div>
                    </div>
                </div>
                {% endif %}

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <span class="block text-sm font-medium text-gray-700">
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
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <span class="block text-sm font-medium text-gray-700">
                        Owner
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='ownerId' title='Owner' selectedItems=owner min=1 max=1 id=getNextId("autocompleter") extraInfo="Category" %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <span class="block text-sm font-medium text-gray-700">
                        Resource Category
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' elName='ResourceCategoryId' title='Resource Category' selectedItems=resourceCategories min=0 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <span class="block text-sm font-medium text-gray-700">
                        Series
                        <p class="mt-2 text-sm text-gray-500">Optional. Resources in the same series are grouped together.</p>
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/seriesList' addUrl='/v1/series/create' elName='SeriesId' title='Series' selectedItems=series min=0 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>

                {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' fromJSON=resource.Meta jsonOutput="true" id=getNextId("freeField") %}
            </div>
        </div>
    </div>

    <div class="pt-5">
        <div class="flex justify-end">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                Save
            </button>
        </div>
    </div>
</form>
{% endblock %}
