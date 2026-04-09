{% extends "/layouts/base.tpl" %}

{% block body %}
    {% plugin_slot "note_detail_before" %}
    <div x-data="{ entity: {{ note|json }} }" data-paste-context='{"type":"note","id":{{ note.ID }},"ownerId":{{ note.OwnerId|default:"null" }},"name":"{{ note.Name|escapejs }}"}'>
        {% process_shortcodes note.NoteType.CustomHeader note %}
    </div>

    <div class="meta-strip">
        {% if sc.Timestamps %}
        {% if note.StartDate %}
        <div class="meta-strip-item">
            <span class="meta-strip-label">Started</span>
            <span class="meta-strip-value">{{ dereference(note.StartDate)|date:"2006-01-02 15:04" }}</span>
        </div>
        {% endif %}
        {% if note.EndDate %}
        <div class="meta-strip-item">
            <span class="meta-strip-label">Ended</span>
            <span class="meta-strip-value">{{ dereference(note.EndDate)|date:"2006-01-02 15:04" }}</span>
        </div>
        {% endif %}
        {% endif %}
        <div class="meta-strip-item">
            <a class="text-amber-700 hover:text-amber-800 text-sm font-medium" href="/note/text?id={{ note.ID }}">Wide display</a>
        </div>
    </div>

    {% if sc.Content %}
    {# Show description only when no blocks exist (syncFirstTextBlockToDescription copies first text block into Description). #}
    {% if !note.Blocks || note.Blocks|length == 0 %}
        {% include "/partials/description.tpl" with description=note.Description descriptionEditUrl="/v1/note/editDescription" descriptionEditId=note.ID preview=false %}
    {% endif %}
    {% include "/partials/blockEditor.tpl" with noteId=note.ID blocks=note.Blocks %}
    {% endif %}

    {% if sc.Groups %}
    {% include "/partials/seeAll.tpl" with entities=note.Groups subtitle="Groups" formAction="/groups" formID=note.ID formParamName="notes" templateName="group" %}
    {% endif %}
    {% if sc.Resources %}
    {% include "/partials/seeAll.tpl" with entities=note.Resources subtitle="Resources" formAction="/resources" addAction="/resource/new" addFormSecondParamName="ownerId" addFormSecondParamValue=note.OwnerId formID=note.ID formParamName="notes" templateName="resource" %}
    {% endif %}
    {% plugin_slot "note_detail_after" %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ note|json }} }">
            {% process_shortcodes note.NoteType.CustomSidebar note %}
        </div>
        {% if sc.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}{% endif %}
    </div>

    {% if sc.NoteTypeLink %}
    {% if note.NoteType %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Note Type" %}
        <a href="/noteType?id={{ note.NoteType.ID }}" class="text-amber-700 hover:underline">{{ note.NoteType.Name }}</a>
    </div>
    {% endif %}
    {% endif %}

    {% if sc.Tags %}
    <div class="sidebar-group">
        {% include "/partials/tagList.tpl" with tags=note.Tags addTagUrl='/v1/notes/addTags' id=note.ID %}
    </div>
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if note.NoteType.MetaSchema && note.Meta %}
    <div class="sidebar-group">
        <schema-editor mode="display"
            schema='{{ note.NoteType.MetaSchema }}'
            value='{{ note.Meta|json }}'
            name="{{ note.NoteType.Name }}">
        </schema-editor>
    </div>
    {% endif %}
    {% endif %}

    {% if sc.MetaJson %}
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=note.Meta %}
    </div>
    {% endif %}

    {% if sc.Share %}
    <div class="sidebar-group">
        {% include "/partials/noteShare.tpl" with note=note shareEnabled=shareEnabled shareBaseUrl=shareBaseUrl %}
        {% include "partials/pluginActionsSidebar.tpl" with entityId=note.ID entityType="note" %}
        {% plugin_slot "note_detail_sidebar" %}
    </div>
    {% endif %}
{% endblock %}
