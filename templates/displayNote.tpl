{% extends "/layouts/base.tpl" %}

{% block body %}
    <div x-data="{ entity: {{ note|json }} }" data-paste-context='{"type":"note","id":{{ note.ID }},"ownerId":{{ note.OwnerId }},"name":"{{ note.Name|escapejs }}"}'>
        {% autoescape off %}
            {{ note.NoteType.CustomHeader }}
        {% endautoescape %}
    </div>
    <a class="text-blue-600" href="/note/text?id={{ note.ID }}">Wide display</a>

    {# Block Editor - shows blocks if available, otherwise falls back to description #}
    {% if note.Blocks && note.Blocks|length > 0 %}
        {% include "/partials/blockEditor.tpl" with noteId=note.ID blocks=note.Blocks %}
    {% else %}
        {% include "/partials/description.tpl" with description=note.Description preview=false %}
        {# Show empty block editor for adding blocks #}
        {% include "/partials/blockEditor.tpl" with noteId=note.ID blocks=note.Blocks %}
    {% endif %}

    {% include "/partials/seeAll.tpl" with entities=note.Groups subtitle="Groups" formAction="/groups" formID=note.ID formParamName="notes" templateName="group" %}
    {% include "/partials/seeAll.tpl" with entities=note.Resources subtitle="Resources" formAction="/resources" addAction="/resource/new" addFormSecondParamName="ownerId" addFormSecondParamValue=note.OwnerId formID=note.ID formParamName="notes" templateName="resource" %}

{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div x-data="{ entity: {{ note|json }} }">
        {% autoescape off %} {# KAN-6: by design — internal network app, all users trusted #}
            {{ note.NoteType.CustomSidebar }}
        {% endautoescape %}
    </div>
    {% if note.StartDate %}
        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-600">Started: </span>{{ dereference(note.StartDate)|date:"2006-01-02 15:04" }}</small>
    {% endif %}
    {% if note.EndDate %}
        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-600">Ended: </span>{{ dereference(note.EndDate)|date:"2006-01-02 15:04" }}</small>
    {% endif %}
    {% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}

    {% if note.NoteType %}
        {% include "/partials/sideTitle.tpl" with title="Note Type" %}
        <a href="/noteType?id={{ note.NoteType.ID }}" class="text-blue-600 hover:underline">{{ note.NoteType.Name }}</a>
    {% endif %}

    {% include "/partials/tagList.tpl" with tags=note.Tags %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}

    {% include "/partials/noteShare.tpl" with note=note shareEnabled=shareEnabled shareBaseUrl=shareBaseUrl %}
{% endblock %}