{% extends "/layouts/base.tpl" %}

{% block body %}
    {% plugin_slot "group_detail_before" %}
    <div x-data="{ entity: {{ group|json }} }" data-paste-context='{"type":"group","id":{{ group.ID }},"name":"{{ group.Name|escapejs }}"}'>
        {% process_shortcodes group.Category.CustomHeader group %}
    </div>

    {% if sc.Description %}
    {% include "/partials/description.tpl" with description=group.Description descriptionEntity=group descriptionEditUrl="/v1/group/editDescription" descriptionEditId=group.ID %}
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if group.Category.MetaSchema && group.Meta %}
    <schema-editor mode="display"
        schema='{{ group.Category.MetaSchema }}'
        value='{{ group.Meta|json }}'
        name="{{ group.Category.Name }}">
    </schema-editor>
    {% endif %}
    {% endif %}

    {% with hasOwn=(group.OwnNotes || group.OwnGroups || group.OwnResources) %}

    {% if sc.OwnEntities.State != "off" %}
    <details class="detail-collapsible mb-6" {% if sc.OwnEntities.State == "open" %}open{% elif sc.OwnEntities.State == "collapsed" %}{% elif hasOwn %}open{% endif %}>
        <summary>Own Entities</summary>
        <div class="detail-panel-body">
            {% if sc.OwnEntities.OwnNotes %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnNotes subtitle="Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
            {% endif %}
            {% if sc.OwnEntities.OwnGroups %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnGroups subtitle="Sub-Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="ownerId" templateName="group" %}
            {% endif %}
            {% if sc.OwnEntities.OwnResources %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnResources subtitle="Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="ownerId" templateName="resource" %}
            {% endif %}
        </div>
    </details>
    {% endif %}

    {% if sc.RelatedEntities.State != "off" %}
    <details class="detail-collapsible mb-6" {% if sc.RelatedEntities.State == "open" %}open{% elif sc.RelatedEntities.State == "collapsed" %}{% elif !hasOwn %}open{% endif %}>
        <summary>Related Entities</summary>
        <div class="detail-panel-body">
            {% if sc.RelatedEntities.RelatedGroups %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedGroups subtitle="Related Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="groups" templateName="group" %}
            {% endif %}
            {% if sc.RelatedEntities.RelatedResources %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedResources subtitle="Related Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="groups" addFormSecondParamName="ownerid" addFormSecondParamValue=group.OwnerId templateName="resource" %}
            {% endif %}
            {% if sc.RelatedEntities.RelatedNotes %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedNotes subtitle="Related Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="groups" templateName="note" %}
            {% endif %}
        </div>
    </details>
    {% endif %}

    {% endwith %}

    {% if sc.Relations.State != "off" %}
    <details class="detail-collapsible mb-6"{% if sc.Relations.State == "open" %} open{% elif sc.Relations.State == "collapsed" %}{% elif group.Relationships || group.BackRelations %} open{% endif %}>
        <summary>Relations</summary>
        <div class="detail-panel-body">
            {% if sc.Relations.ForwardRelations %}
            {% include "/partials/seeAll.tpl" with entities=group.Relationships subtitle="Relations" formID=group.ID formAction="/relations" formParamName="FromGroupId" addAction="/relation/new" templateName="relation" %}
            {% endif %}
            {% if sc.Relations.ReverseRelations %}
            {% include "/partials/seeAll.tpl" with entities=group.BackRelations subtitle="Reverse Relations" formID=group.ID formAction="/relations" formParamName="ToGroupId" addAction="/relation/new" templateName="relation_reverse" %}
            {% endif %}
        </div>
    </details>
    {% endif %}
    {% plugin_slot "group_detail_after" %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ group|json }} }">
            {% process_shortcodes group.Category.CustomSidebar group %}
        </div>
        {% if sc.Owner %}{% if group.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=group.Owner %}{% endif %}{% endif %}
        {% if sc.TreeLink %}<a href="/group/tree?containing={{ group.ID }}" class="block text-sm text-amber-700 hover:text-amber-900 mb-2">Show in Tree</a>{% endif %}
    </div>

    {% if sc.Tags %}
    <div class="sidebar-group">
        {% include "/partials/tagList.tpl" with tags=group.Tags addTagUrl='/v1/groups/addTags' id=group.ID %}
    </div>
    {% endif %}

    {% if sc.MetaJson %}
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=group.Meta %}
    </div>
    {% endif %}

    {% if sc.Merge %}
    <div class="sidebar-group">
        <form
            x-data="confirmAction({ message: 'Selected groups will be deleted and merged to {{ group.Name|escapejs }}. Are you sure?' })"
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
    {% endif %}

    {% if sc.Clone %}
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
    {% endif %}

    <div class="sidebar-group">
        {% include "partials/pluginActionsSidebar.tpl" with entityId=group.ID entityType="group" %}
        {% plugin_slot "group_detail_sidebar" %}
    </div>
{% endblock %}
