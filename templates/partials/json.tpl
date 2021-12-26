<div class="tableContainer" x-data="() => ({ jsonData: {{ jsonData|json }}, keys: '{{ keys }}' })" x-html="renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData).outerHTML">
</div>
