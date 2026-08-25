<div class="bg-white shadow rounded-lg p-4" x-data="textDiff({
    leftText: {{ leftText|json }},
    rightText: {{ rightText|json }},
    leftName: '{{ leftTitle|escapejs }}',
    rightName: '{{ rightTitle|escapejs }}'
})">
    {% include "/partials/compareTextToolbar.tpl" %}
    {% include "/partials/compareTextViews.tpl" %}
</div>
