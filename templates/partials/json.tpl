<div
        class="tableContainer flex gap-3 flex-col"
        x-cloak
        :class="expanded && 'expanded'"
        x-data="
            () => ({
                jsonData: {{ jsonData|json }},
                keys: '{{ keys }}' ,
                expanded: false,
            })
        "
        x-effect="document.body.classList.toggle('overflow-hidden', expanded)"
        @click="(e) => {if(!e.shiftKey) return; expanded = !expanded; e.preventDefault();}"
>
    <button
            x-show="jsonData && (jsonData.length || Object.keys(jsonData).length)"
            class="
                inline-flex justify-center
                py-2 px-4
                border border-transparent
                shadow-sm text-sm font-mono font-medium rounded-md text-white
                bg-amber-700 hover:bg-amber-800"
            @click.prevent="expanded = !expanded"
            x-text="expanded ? 'Minimize' : 'Fullscreen'">
    </button>
    <div x-init="$el.appendChild(renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData))"></div>
</div>
