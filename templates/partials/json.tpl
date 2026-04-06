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
    <div class="metaHeader">
        <h2 class="sidebar-group-title">{{ metaTitle|default:"Meta Data" }}</h2>
        <button
                x-show="jsonData && (Array.isArray(jsonData) ? jsonData.length : Object.keys(jsonData).length)"
                class="metaExpandBtn"
                @click.prevent="expanded = !expanded"
                :aria-label="expanded ? 'Minimize metadata view' : 'Expand metadata to fullscreen'"
                :aria-expanded="expanded.toString()"
        >
            <template x-if="!expanded">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M2 6V2h4M14 6V2h-4M2 10v4h4M14 10v4h-4"/></svg>
            </template>
            <template x-if="expanded">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M6 2H2v4M10 2h4v4M6 14H2v-4M10 14h4v-4"/></svg>
            </template>
            <span x-text="expanded ? 'Minimize' : 'Expand'"></span>
        </button>
    </div>
    <div class="metaTableInner" x-init="$el.appendChild(renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData))"></div>
</div>
