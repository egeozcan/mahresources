{% extends "/layouts/base.tpl" %}

{% block body %}
    <a class="text-blue-600" href="/note/text?id={{ note.ID }}">Wide display</a>
    {% include "/partials/description.tpl" with description=note.Description preview=false %}

    {% include "/partials/seeAll.tpl" with entities=note.Groups subtitle="Groups" formAction="/groups" formID=note.ID formParamName="notes" templateName="group" %}
    {% include "/partials/seeAll.tpl" with entities=note.Resources subtitle="Resources" formAction="/resources" addAction="/resource/new" addFormSecondParamName="ownerId" addFormSecondParamValue=note.OwnerId formID=note.ID formParamName="notes" templateName="resource" %}

{% endblock %}

{% block sidebar %}
    {% if note.StartDate %}
        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-400">Started: </span>{{ dereference(note.StartDate)|date:"2006-01-02 15:04" }}</small>
    {% endif %}
    {% if note.EndDate %}
        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-400">Ended: </span>{{ dereference(note.EndDate)|date:"2006-01-02 15:04" }}</small>
    {% endif %}
    {% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}
    {% include "/partials/tagList.tpl" with tags=note.Tags %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
{% endblock %}