{% extends "/layouts/base.tpl" %}

{% block body %}
    <a class="text-amber-700" href="/note?id={{ note.ID }}">Go back to the note</a>
    {% if sc.Content %}
    {# Show description only when no blocks exist (syncFirstTextBlockToDescription copies first text block into Description). #}
    {% if !note.Blocks || note.Blocks|length == 0 %}
    {% autoescape off %}
        <div class="prose lg:prose-xl max-w-full font-sans">
        {{ note.Description|markdown2 }}
        </div>
    {% endautoescape %}
    {% endif %}
    {% if note.Blocks && note.Blocks|length > 0 %}
        {% include "/partials/blockEditor.tpl" with noteId=note.ID blocks=note.Blocks %}
    {% endif %}
    {% endif %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div x-data="{ entity: {{ note|json }} }">
        {% process_shortcodes note.NoteType.CustomSidebar note %}
    </div>
    {% if sc.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}{% endif %}
    {% if sc.NoteTypeLink %}
    {% if note.NoteType %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Note Type" %}
        <a href="/noteType?id={{ note.NoteType.ID }}" class="text-amber-700 hover:underline">{{ note.NoteType.Name }}</a>
    </div>
    {% endif %}
    {% endif %}
    {% if sc.Tags %}
    {% include "/partials/tagList.tpl" with tags=note.Tags addTagUrl='/v1/notes/addTags' id=note.ID %}
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
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
    {% endif %}
{% endblock %}
