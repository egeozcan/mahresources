{# Formatting buttons for a markdownEditor scope. mousedown.prevent keeps focus (and the selection) in the editor. #}
{# Shown (disabled) while CodeMirror loads, so its arrival does not move the page; hidden only if loading failed. #}
{# with toolbar_label= -- the field's title, so the toolbar is named after the field it formats. #}
<div role="toolbar" aria-label="{{ toolbar_label }} formatting" x-show="!failed"
     class="flex flex-wrap items-center gap-1 mb-1" data-markdown-toolbar>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="bold()" aria-label="Bold" title="Bold (Ctrl/Cmd+B)"
            class="px-2 py-1 text-xs font-mono font-bold text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">B</button>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="italic()" aria-label="Italic" title="Italic (Ctrl/Cmd+I)"
            class="px-2 py-1 text-xs font-mono italic text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">I</button>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="link()" aria-label="Link" title="Link (Ctrl/Cmd+Shift+K)"
            class="px-2 py-1 text-xs font-mono text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">Link</button>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="heading()" aria-label="Heading" title="Heading"
            class="px-2 py-1 text-xs font-mono text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">H</button>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="bulletList()" aria-label="Bulleted list" title="Bulleted list"
            class="px-2 py-1 text-xs font-mono text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">&bull; List</button>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="numberedList()" aria-label="Numbered list" title="Numbered list"
            class="px-2 py-1 text-xs font-mono text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">1. List</button>
    <button type="button" :disabled="!ready" @mousedown.prevent @click="inlineCode()" aria-label="Code" title="Code"
            class="px-2 py-1 text-xs font-mono text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-amber-600 disabled:opacity-50">&lt;/&gt;</button>
</div>
