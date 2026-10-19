{# The markdown editor's own markup, inside a markdownEditor x-data the caller provides: toolbar, source textarea, CodeMirror container, preview pane. #}
{# with field_id= name= value= title= [label_id=] [aria_label=] [described_by=] [has_preview=] [inline=true] #}
{# The textarea is the submitted value; markdownEditor.js mounts CodeMirror over it and writes every edit back. #}
{% include "/partials/form/markdownToolbar.tpl" with toolbar_label=title has_preview=has_preview %}
<textarea
        x-ref="source"
        id="{{ field_id }}"
        name="{{ name }}"
        rows="6"
        {% if label_id %}data-label-id="{{ label_id }}"{% endif %}
        {% if aria_label %}aria-label="{{ aria_label }}"{% endif %}
        {% if described_by %}aria-describedby="{{ described_by }}"{% endif %}
        {% if inline %}x-init="if (serverValue !== null) $el.value = serverValue" @keydown.escape.prevent.stop="cancel()" @keydown.ctrl.enter.prevent.stop="save()" @keydown.meta.enter.prevent.stop="save()"{% endif %}
        {# Sized like the mounted editor (markdownEditor.js theme), so the swap does not move the page. #}
        style="field-sizing: content; min-height: calc(9rem + 2px); max-height: 60vh; font-size: 0.875rem; line-height: 1.5; padding: 0.5rem 0.75rem;"
        class="shadow-sm block w-full focus:ring-amber-600 focus:border-amber-600 border-stone-300 rounded-md font-sans"
>{{ value }}</textarea>
{# Shown by markdownEditor.js itself, in the same task that hides the textarea and moves focus. #}
<div x-ref="mdContainer" hidden data-markdown-editor
     class="border border-stone-300 rounded-md overflow-hidden max-w-full bg-white"></div>
{% if has_preview %}
<div x-show="previewing" x-cloak data-markdown-preview-pane>
    <p x-show="shortcodeHint" x-text="shortcodeHint" class="text-xs text-stone-600 font-sans mb-1"></p>
    <p x-show="previewLoading" class="text-xs text-stone-600 font-sans mb-1" role="status">Rendering preview…</p>
    <p x-show="previewError" x-text="previewError" role="alert" class="text-sm text-red-700 font-sans mb-1"></p>
    {# No allow-same-origin and no allow-scripts: raw HTML in a Description cannot reach this page or its form. #}
    <iframe x-ref="previewFrame" data-markdown-preview sandbox="allow-popups allow-popups-to-escape-sandbox"
            title="{{ title }} preview"
            class="w-full border border-stone-300 rounded-md bg-white" style="height: 16rem;"></iframe>
</div>
{% endif %}
