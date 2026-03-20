{% extends "/layouts/base.tpl" %}

{% block body %}
    <a class="text-amber-700" href="/note?id={{ note.ID }}">Go back to the note</a>
    {% autoescape off %}
        <div class="prose lg:prose-xl max-w-full font-sans">
        {{ note.Description|markdown2 }}
        </div>
    {% endautoescape %}
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
    {% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}
    {% if note.NoteType %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Note Type" %}
        <a href="/noteType?id={{ note.NoteType.ID }}" class="text-amber-700 hover:underline">{{ note.NoteType.Name }}</a>
    </div>
    {% endif %}
    {% include "/partials/tagList.tpl" with tags=note.Tags addTagUrl='/v1/notes/addTags' id=note.ID %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
{% endblock %}