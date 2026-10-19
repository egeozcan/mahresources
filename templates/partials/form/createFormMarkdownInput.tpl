{# A Description field edited as markdown: markdownEditor mounts CodeMirror over the textarea, which stays the submitted value. #}
{# with title= name= value= [id=] [shortcodes=true] [hint=] [previewUrl=] [previewId=] -- shortcodes only for entity types whose Descriptions run them. #}
{# previewUrl is the entity's /v1/<entity>/previewDescription; previewId its ID when editing (empty on a create form). #}
{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5" data-markdown-field="{{ name }}"
     x-data="markdownEditor({ shortcodes: {% if shortcodes %}true{% else %}false{% endif %}, previewUrl: '{{ previewUrl }}', previewId: '{{ previewId|default:"" }}' })">
    {# Once mounted the textarea is hidden, so the label focuses the editor instead. #}
    <label id="{{ field_id }}-label" for="{{ field_id }}" @click="focusEditor($event)" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">{{ title }}</label>
    {# min-w-0 keeps the editor's min-content width from overflowing the grid column (findings 19/101). #}
    <div class="mt-1 sm:mt-0 sm:col-span-2 min-w-0">
        {% include "/partials/form/markdownEditorCore.tpl" with field_id=field_id name=name value=value title=title label_id=field_id|add:"-label" has_preview=previewUrl %}
        {% if hint %}<p class="mt-2 text-sm text-stone-500 font-sans">{{ hint }}</p>{% endif %}
    </div>
</div>
{% endwith %}
