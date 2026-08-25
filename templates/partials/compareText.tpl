<div class="bg-white shadow rounded-lg p-4" x-data="textDiff({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID|json }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID|json }}',
    totalBytes: {{ comparison.Version1.FileSize|json }} + {{ comparison.Version2.FileSize|json }},
    leftName: {{ panelTitle1|json }},
    rightName: {{ panelTitle2|json }}
})">
    {% include "/partials/compareTextToolbar.tpl" %}
    {% include "/partials/compareTextViews.tpl" with leftTitle=panelTitle1 rightTitle=panelTitle2 %}
</div>
