{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resource.Description %}

    <div class="mb-6">
        {% include "/partials/json.tpl" with jsonData=resource keys="ID,CreatedAt,UpdatedAt,Name,OriginalName,OriginalLocation,Hash,HashType,Location,StorageLocation,Description" %}
    </div>

    {% include "/partials/seeAll.tpl" with entities=resource.Notes subtitle="Notes" formAction="/notes" formID=resource.ID formParamName="resources" templateName="note" %}
    {% include "/partials/seeAll.tpl" with entities=resource.Groups subtitle="Groups" formAction="/groups" formID=resource.ID formParamName="resources" templateName="group" %}
{% endblock %}

{% block sidebar %}
    {% include "/partials/ownerDisplay.tpl" with owner=resource.Owner %}
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