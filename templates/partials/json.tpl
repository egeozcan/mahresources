<div
        class="tableContainer"
        :class="expanded && 'expanded'"
        x-data="
            () => ({
                jsonData: {{ jsonData|json }},
                keys: '{{ keys }}' ,
                expanded: false,
            })
        "
        x-html="
            renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData).outerHTML
        "
        @click="(e) => {if(!e.shiftKey) return; expanded = !expanded; e.preventDefault();}"
>
</div>
