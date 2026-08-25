<div class="bg-white shadow rounded-lg p-4" x-data="textDiff({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}',
    totalBytes: {{ comparison.Version1.FileSize }} + {{ comparison.Version2.FileSize }},
    leftName: '{{ panelTitle1|escapejs }}',
    rightName: '{{ panelTitle2|escapejs }}'
})">
    {% include "/partials/compareTextToolbar.tpl" %}
    {% include "/partials/compareTextViews.tpl" with leftTitle=panelTitle1 rightTitle=panelTitle2 %}
</div>
