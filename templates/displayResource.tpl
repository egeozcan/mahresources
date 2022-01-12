{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resource.Description %}

    <div class="mb-6">
        {% include "/partials/json.tpl" with jsonData=resource keys="ID,CreatedAt,UpdatedAt,Name,OriginalName,OriginalLocation,Hash,HashType,Location,StorageLocation,Description" %}
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
{% endblock %}

{% block sidebar %}
    {% include "/partials/ownerDisplay.tpl" with owner=resource.Owner %}
    <p>{{ resource.FileSize | humanReadableSize }}</p>
    <a href="/v1/resource/view?id={{ resource.ID }}#{{ entity.ContentType }}">
        <img height="300" src="/v1/resource/preview?id={{ resource.ID }}&height=300" alt="Preview">
    </a>
    {% include "/partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in resource.Tags %}
            {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=resource.Meta %}
{% endblock %}