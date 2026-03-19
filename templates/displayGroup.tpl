{% extends "/layouts/base.tpl" %}

{% block body %}
    {% plugin_slot "group_detail_before" %}
    <div x-data="{ entity: {{ group|json }} }" data-paste-context='{"type":"group","id":{{ group.ID }},"name":"{{ group.Name|escapejs }}"}'>
        {% autoescape off %}
            {{ group.Category.CustomHeader }}
        {% endautoescape %}
    </div>

    {% include "/partials/description.tpl" with description=group.Description descriptionEditUrl="/v1/group/editDescription" descriptionEditId=group.ID %}

    {% with hasOwn=(group.OwnNotes || group.OwnGroups || group.OwnResources) %}

    <details class="detail-collapsible mb-6" {% if hasOwn %}open{% endif %}>
        <summary>Own Entities</summary>
        <div class="detail-panel-body">
            {% include "/partials/seeAll.tpl" with entities=group.OwnNotes subtitle="Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnGroups subtitle="Sub-Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="ownerId" templateName="group" %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnResources subtitle="Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="ownerId" templateName="resource" %}
        </div>
    </details>

    <details class="detail-collapsible mb-6" {% if !hasOwn %}open{% endif %}>
        <summary>Related Entities</summary>
        <div class="detail-panel-body">
            {% include "/partials/seeAll.tpl" with entities=group.RelatedGroups subtitle="Related Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="groups" templateName="group" %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedResources subtitle="Related Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="groups" addFormSecondParamName="ownerid" addFormSecondParamValue=group.OwnerId templateName="resource" %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedNotes subtitle="Related Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="groups" templateName="note" %}
        </div>
    </details>

    {% endwith %}
    <details class="detail-collapsible mb-6"{% if group.Relationships || group.BackRelations %} open{% endif %}>
        <summary>Relations</summary>
        <div class="detail-panel-body">
            {% include "/partials/seeAll.tpl" with entities=group.Relationships subtitle="Relations" formID=group.ID formAction="/relations" formParamName="FromGroupId" addAction="/relation/new" templateName="relation" %}
            {% include "/partials/seeAll.tpl" with entities=group.BackRelations subtitle="Reverse Relations" formID=group.ID formAction="/relations" formParamName="ToGroupId" addAction="/relation/new" templateName="relation_reverse" %}
        </div>
    </details>
    {% plugin_slot "group_detail_after" %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ group|json }} }">
            {% autoescape off %} {# KAN-6: by design — internal network app, all users trusted #}
                {{ group.Category.CustomSidebar }}
            {% endautoescape %}
        </div>
        {% if group.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=group.Owner %}{% endif %}
        <a href="/group/tree?containing={{ group.ID }}" class="block text-sm text-amber-700 hover:text-amber-900 mb-2">Show in Tree</a>
    </div>

    <div class="sidebar-group">
        {% include "/partials/tagList.tpl" with tags=group.Tags addTagUrl='/v1/groups/addTags' id=group.ID %}
    </div>

    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
        {% include "/partials/json.tpl" with jsonData=group.Meta %}
    </div>

    <div class="sidebar-group">
        <form
            x-data="confirmAction({ message: `Selected groups will be deleted and merged to {{ group.Name|json }}. Are you sure?` })"
            action="/v1/groups/merge"
            :action="'/v1/groups/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            method="post"
            x-bind="events"
        >
            <input type="hidden" name="winner" value="{{ group.ID }}">
            <p>Merge others with this group?</p>
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='losers' title='Groups To Merge' id=getNextId("autocompleter") extraInfo="Category" %}
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
        </form>
    </div>

    <div class="sidebar-group">
        <form
            x-data="confirmAction({ message: 'Clone this group and all its associations?' })"
            action="/v1/group/clone"
            method="post"
            x-bind="events"
        >
            <input type="hidden" name="Id" value="{{ group.ID }}">
            <p>Clone group?</p>
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Clone" %}</div>
        </form>
    </div>

    <div class="sidebar-group">
        {% include "partials/pluginActionsSidebar.tpl" with entityId=group.ID entityType="group" %}
        {% plugin_slot "group_detail_sidebar" %}
    </div>
{% endblock %}
