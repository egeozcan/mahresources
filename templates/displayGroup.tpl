{% extends "/layouts/base.tpl" %}

{% block body %}

    {% include "/partials/description.tpl" with description=group.Description %}

    <div x-data="accordion({ collapsed: false })" class="mb-6">
        <button x-bind="events" class="bg-gray-100 shadow rounded-lg block w-full p-4 text-left">Own Entities</button>
        <div class="p-4 border-dashed border-4 border-gray-100 border-t-0">
            {% include "/partials/seeAll.tpl" with entities=group.OwnGroups subtitle="Sub-Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="ownerId" templateName="group" %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnResources subtitle="Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="ownerId" templateName="resource" %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnNotes subtitle="Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
        </div>
    </div>

    <div x-data="accordion({ collapsed: true })" class="mb-6">
        <button x-bind="events" class="bg-gray-100 shadow rounded-lg block w-full p-4 text-left">Related Entities</button>
        <div class="p-4 border-dashed border-4 border-gray-100 border-t-0">
            {% include "/partials/seeAll.tpl" with entities=group.RelatedGroups subtitle="Related Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="groups" templateName="group" %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedResources subtitle="Related Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="groups" templateName="resource" %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedNotes subtitle="Related Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
        </div>
    </div>

    <div x-data="accordion({ collapsed: false })" class="mb-6">
        <button x-bind="events" class="bg-gray-100 shadow rounded-lg block w-full p-4 text-left">Relations</button>
        <div class="p-4 border-dashed border-4 border-gray-100 border-t-0">
            {% include "/partials/seeAll.tpl" with entities=group.Relationships subtitle="Relations" formID=group.ID formAction="/relations" formParamName="FromGroupId" addAction="/relation/new" templateName="relation" %}
            {% include "/partials/seeAll.tpl" with entities=group.BackRelations subtitle="Reverse Relations" formID=group.ID formAction="/relations" formParamName="ToGroupId" addAction="/relation/new" templateName="relation_reverse" %}
        </div>
    </div>

{% endblock %}

{% block sidebar %}
    {% if group.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=group.Owner %}{% endif %}
    {% include "/partials/tagList.tpl" with tags=group.Tags %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=group.Meta %}
{% endblock %}