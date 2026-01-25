{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resource.Description %}

    <div class="mb-6">
        {% include "/partials/json.tpl" with jsonData=resource keys="ID,CreatedAt,UpdatedAt,Name,OriginalName,OriginalLocation,Hash,HashType,Location,StorageLocation,Description,Width,Height" %}
    </div>

    {% include "/partials/seeAll.tpl" with entities=resource.Notes subtitle="Notes" formAction="/notes" formID=resource.ID formParamName="resources" templateName="note" %}
    {% include "/partials/seeAll.tpl" with entities=resource.Groups subtitle="Groups" formAction="/groups" formID=resource.ID formParamName="resources" templateName="group" %}
    {% if similarResources %}
        {% include "/partials/seeAll.tpl" with entities=similarResources subtitle="Similar Resources" templateName="resource" %}
        <form
            x-data="confirmAction({ message: 'All the similar resources will be deleted. Are you sure?' })"
            action="/v1/resources/merge"
            method="post" :action="'/v1/resources/merge?redirect=' + encodeURIComponent(window.location)"
            x-bind="events"
        >
            <input type="hidden" name="winner" value="{{ resource.ID }}">
            {% for entity in similarResources %}
                <input type="hidden" name="losers" value="{{ entity.ID }}">
            {% endfor %}
            <p>Merge others with this resource ({{ resource.FileSize | humanReadableSize }})?</p>
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge Others To This" %}</div>
        </form>
    {% endif %}

    {% include "/partials/versionPanel.tpl" with versions=versions currentVersionId=resource.CurrentVersionID resourceId=resource.ID %}
{% endblock %}

{% block sidebar %}
    {% include "/partials/ownerDisplay.tpl" with owner=resource.Owner %}
    <p>{{ resource.FileSize | humanReadableSize }}</p>
    <a href="/v1/resource/view?id={{ resource.ID }}&v={{ resource.Hash }}#{{ resource.ContentType }}">
        <img height="300" src="/v1/resource/preview?id={{ resource.ID }}&height=300&v={{ resource.Hash }}" alt="Preview">
    </a>
    {% include "/partials/tagList.tpl" with tags=resource.Tags addTagUrl='/v1/resources/addTags' id=resource.ID %}

    {% if isImage %}
        {% include "/partials/sideTitle.tpl" with title="Update Dimensions" %}
        <form action="/v1/resource/recalculateDimensions?redirect={{ url|urlencode }}" method="post">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">Recalculate Dimensions</button>
        </form>
        {% include "/partials/sideTitle.tpl" with title="Rotate 90 Degrees" %}
        <form action="/v1/resources/rotate" method="post">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <input type="hidden" name="degrees" value="90">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">Rotate</button>
        </form>
    {% endif %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=resource.Meta %}
{% endblock %}