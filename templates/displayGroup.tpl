{% extends "/layouts/base.tpl" %}

{% block body %}

    {% include "/partials/description.tpl" with description=group.Description %}

    {% with hasOwn=(group.OwnNotes || group.OwnGroups || group.OwnResources) %}

    <details class="mb-6" {% if hasOwn %}open{% endif %}>
        <summary class="bg-gray-100 shadow rounded-lg block w-full p-4 text-left cursor-pointer select-none">Own Entities</summary>
        <div class="p-4 border-dashed border-4 border-gray-100 border-t-0">
            {% include "/partials/seeAll.tpl" with entities=group.OwnNotes subtitle="Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnGroups subtitle="Sub-Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="ownerId" templateName="group" %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnResources subtitle="Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="ownerId" templateName="resource" %}
        </div>
    </details>

    <details class="mb-6" {% if !hasOwn %}open{% endif %}>
        <summary class="bg-gray-100 shadow rounded-lg block w-full p-4 text-left cursor-pointer select-none">Related Entities</summary>
        <div class="p-4 border-dashed border-4 border-gray-100 border-t-0">
            {% include "/partials/seeAll.tpl" with entities=group.RelatedGroups subtitle="Related Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="groups" templateName="group" %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedResources subtitle="Related Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="groups" addFormSecondParamName="ownerid" addFormSecondParamValue=group.OwnerId templateName="resource" %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedNotes subtitle="Related Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
        </div>
    </details>

    {% endwith %}

    <details class="mb-6"{% if group.Relationships || group.BackRelations %}open{% endif %}>
        <summary class="bg-gray-100 shadow rounded-lg block w-full p-4 text-left cursor-pointer select-none">Relations</summary>
        <div class="p-4 border-dashed border-4 border-gray-100 border-t-0">
            {% include "/partials/seeAll.tpl" with entities=group.Relationships subtitle="Relations" formID=group.ID formAction="/relations" formParamName="FromGroupId" addAction="/relation/new" templateName="relation" %}
            {% include "/partials/seeAll.tpl" with entities=group.BackRelations subtitle="Reverse Relations" formID=group.ID formAction="/relations" formParamName="ToGroupId" addAction="/relation/new" templateName="relation_reverse" %}
        </div>
    </details>
{% endblock %}

{% block sidebar %}
    {% if group.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=group.Owner %}{% endif %}
    {% include "/partials/tagList.tpl" with tags=group.Tags addTagUrl='/v1/groups/addTags' id=group.ID %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=group.Meta %}

    <form
        x-data="confirmAction({ message: `Selected groups will be deleted and merged to {{ group.Name|json }}. Are you sure?` })"
        action="/v1/groups/merge"
        :action="'/v1/groups/merge?redirect=' + encodeURIComponent(window.location)"
        method="post"
        x-bind="events"
    >
        <input type="hidden" name="winner" value="{{ group.ID }}">
        <p>Merge others with this group?</p>
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='losers' title='Groups To Merge' id=getNextId("autocompleter") extraInfo="Category" %}
        <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
    </form>

    <form
        action="/v1/group/clone"
        :action="'/v1/group/clone?redirect=' + encodeURIComponent(window.location)"
        method="post"
    >
        <input type="hidden" name="Id" value="{{ group.ID }}">
        <p>Clone group?</p>
        <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Clone" %}</div>
    </form>
{% endblock %}