{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/category">
    {% if category.ID %}
    <input type="hidden" value="{{ category.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=category.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=category.Description %}

    <fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-2" x-data="{ showTemplateDocs: false }">
        <legend class="text-base font-semibold font-mono text-stone-800 px-2">Custom Templates</legend>

        <div class="text-sm text-stone-600">
            <p>HTML templates rendered in specific slots of detail and list views for groups in this category.</p>
            <button type="button"
                    @click="showTemplateDocs = !showTemplateDocs"
                    class="mt-1 text-sm text-amber-700 hover:text-amber-900 font-mono flex items-center gap-1 cursor-pointer"
                    :aria-expanded="showTemplateDocs.toString()"
                    aria-controls="cat-template-docs-panel">
                <svg :class="showTemplateDocs && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>
                Reference
            </button>
        </div>

        <div x-show="showTemplateDocs" x-collapse id="cat-template-docs-panel"
             class="text-sm text-stone-600 bg-white border border-stone-200 rounded-md p-4 space-y-3 font-sans">
            <div>
                <h3 class="font-semibold text-stone-700">Slot Locations</h3>
                <dl class="mt-1 space-y-1 text-xs">
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Header</dt>
                        <dd>Top of the detail page, above the description</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Sidebar</dt>
                        <dd>Right sidebar on the detail page</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Summary</dt>
                        <dd>List view cards, below the description</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Avatar</dt>
                        <dd>Icon area next to the category name in list cards</dd>
                    </div>
                </dl>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Shortcodes</h3>
                <p class="mt-1 text-xs">
                    <code class="bg-stone-100 px-1 rounded">[meta path="dotted.path" editable=true hide-empty=true]</code>
                    &mdash; render a metadata field value inline; supports editing and auto-hiding when empty.
                </p>
                <p class="mt-1 text-xs">
                    <code class="bg-stone-100 px-1 rounded">[plugin:name:shortcode attr="val"]</code>
                    &mdash; render a plugin-provided shortcode.
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">HTML &amp; Styling</h3>
                <p class="text-xs">Raw HTML and <a href="https://tailwindcss.com/docs" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Tailwind CSS</a> utility classes are fully supported.</p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Alpine.js</h3>
                <p class="text-xs">
                    An <code class="bg-stone-100 px-1 rounded">entity</code> variable with the full group object is available at render time, e.g.
                    <code class="bg-stone-100 px-1 rounded">x-text="entity.Name"</code> or
                    <code class="bg-stone-100 px-1 rounded">x-show="entity.Meta?.status"</code>.
                </p>
            </div>
        </div>

        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Header" name="CustomHeader" value=category.CustomHeader %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=category.CustomSidebar %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Summary" name="CustomSummary" value=category.CustomSummary %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=category.CustomAvatar %}
    </fieldset>
    <div class="flex gap-2 items-start">
        <div class="flex-1">
            {% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=category.MetaSchema big=true id="metaSchemaTextarea" %}
        </div>
        {% include "/partials/form/schemaEditorModal.tpl" with textareaId="metaSchemaTextarea" %}
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}