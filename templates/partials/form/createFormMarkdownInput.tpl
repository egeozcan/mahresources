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
        {% include "/partials/form/markdownToolbar.tpl" with toolbar_label=title has_preview=previewUrl %}
        <textarea
                x-ref="source"
                id="{{ field_id }}"
                name="{{ name }}"
                rows="6"
                data-label-id="{{ field_id }}-label"
                {# Sized like the mounted editor (markdownEditor.js theme), so the swap does not move the page. #}
                style="field-sizing: content; min-height: calc(9rem + 2px); max-height: 60vh; font-size: 0.875rem; line-height: 1.5; padding: 0.5rem 0.75rem;"
                class="shadow-sm block w-full focus:ring-amber-600 focus:border-amber-600 border-stone-300 rounded-md font-sans"
        >{{ value }}</textarea>
        {# Shown by markdownEditor.js itself, in the same task that hides the textarea and moves focus. #}
        <div x-ref="mdContainer" hidden data-markdown-editor
             class="border border-stone-300 rounded-md overflow-hidden max-w-full bg-white"></div>
        {% if previewUrl %}
        <div x-show="previewing" x-cloak data-markdown-preview-pane>
            {% if shortcodes %}
            {% if previewId %}
            <p class="text-xs text-stone-600 font-sans mb-1">Interactive widgets do not work in preview.</p>
            {% else %}
            <p class="text-xs text-stone-600 font-sans mb-1">Shortcodes render after saving.</p>
            {% endif %}
            {% endif %}
            <p x-show="previewLoading" class="text-xs text-stone-600 font-sans mb-1" role="status">Rendering preview…</p>
            <p x-show="previewError" x-text="previewError" role="alert" class="text-sm text-red-700 font-sans mb-1"></p>
            {# No allow-same-origin and no allow-scripts: raw HTML in a Description cannot reach this page or its form. #}
            <iframe x-ref="previewFrame" data-markdown-preview sandbox="allow-popups allow-popups-to-escape-sandbox"
                    title="{{ title }} preview"
                    class="w-full border border-stone-300 rounded-md bg-white" style="height: 16rem;"></iframe>
        </div>
        {% endif %}
        {% if hint %}<p class="mt-2 text-sm text-stone-500 font-sans">{{ hint }}</p>{% endif %}
    </div>
</div>
{% endwith %}
