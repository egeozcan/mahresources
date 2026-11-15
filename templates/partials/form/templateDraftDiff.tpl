{# The line changes of one draft field against the field before generation, shown #}
{# only when the field had content. Each row starts with -, + or a space so the    #}
{# kind of change is read, not only seen in colour. x-text only: model text.       #}
{# Parameters (Alpine expressions): rows (diffRows output), test_id, label.        #}
<template x-if="{{ rows }}.length">
    <details class="mt-1 rounded border border-stone-200" :data-testid="{{ test_id }}">
        <summary class="cursor-pointer px-2 py-0.5 text-xs font-mono text-stone-800 bg-stone-50">Show line changes</summary>
        <pre tabindex="0" role="region" :aria-label="'Line changes of ' + {{ label }}" class="max-h-48 overflow-auto p-2 text-xs font-mono whitespace-pre-wrap break-all focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"><template x-for="row in {{ rows }}" :key="row.key"><span class="block" :data-testid="'diff-row-' + row.kind" :class="row.kind === 'add' ? 'bg-green-100 text-green-900' : row.kind === 'del' ? 'bg-red-100 text-red-900' : row.kind === 'gap' ? 'text-stone-500 italic' : 'text-stone-700'" x-text="row.kind === 'add' ? '+ ' + row.text : row.kind === 'del' ? '- ' + row.text : row.kind === 'gap' ? '  … ' + row.text : '  ' + row.text"></span></template></pre>
    </details>
</template>
