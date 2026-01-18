{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/group">
    {% if group.ID %}
    <input type="hidden" value="{{ group.ID }}" name="ID">
    {% endif %}
    <div class="space-y-8 sm:space-y-5">
        <div>
            <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
                {% if !group.ID %}
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <span class="block text-sm font-medium text-gray-700">
                        Category
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' addUrl='/v1/category' elName='categoryId' title='Category' selectedItems=category min=1 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>
                {% endif %}

                {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=group.Name required=true id="form-name" %}
                {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=group.Description %}
                {% include "/partials/form/createFormTextInput.tpl" with type="url" title="URL" name="URL" value=group.URL|printUrl %}

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
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='ownerId' title='Owner' selectedItems=owner max=1 id=getNextId("autocompleter") extraInfo="Category" %}
                            </div>
                        </div>
                    </div>
                </div>

                {% set initialSchema = "" %}
                {% if group.Category %}
                    {% set initialSchema = group.Category.MetaSchema %}
                {% elif category && category.0 %}
                    {% set initialSchema = category.0.MetaSchema %}
                {% endif %}

                <div x-data="{
                         currentSchema: {{ initialSchema|default:'null' }},
                         handleCategoryChange(e) {
                             if (e.detail.value.length > 0) {
                                 this.currentSchema = e.detail.value[0].MetaSchema;
                             } else {
                                 this.currentSchema = null;
                             }
                         }
                    }"
                    @multiple-input.window="if ($event.detail.name === 'categoryId') handleCategoryChange($event)"
                    class="w-full"
                >
                    <template x-if="currentSchema">
                        <div class="border p-4 rounded-md bg-gray-50 mt-5">
                            <h3 class="text-sm font-medium text-gray-700 mb-3">Meta Data (Schema Enforced)</h3>
                            <div x-data="schemaForm({
                                schema: currentSchema,
                                value: {{ group.Meta|json }} || {},
                                name: 'Meta'
                            })">
                                <div x-ref="container"></div>
                                <input type="hidden" :name="name" :value="jsonText">
                            </div>
                        </div>
                    </template>
                    <template x-if="!currentSchema">
                        {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/groups/meta/keys' fromJSON=group.Meta jsonOutput="true" id=getNextId("freeField") %}
                    </template>
                </div>
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