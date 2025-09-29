{% extends "/layouts/base.tpl" %}

{% block body %}
    <a class="text-blue-600" href="/note?id={{ note.ID }}">Go back to the note</a>
    {% autoescape off %}
        <div class="prose lg:prose-xl max-w-full">
        {{ note.Description|markdown2 }}
        </div>
    {% endautoescape %}
{% endblock %}

{% block sidebar %}
    <div x-data="{ entity: {{ note|json }} }">
        {% autoescape off %}
            {{ note.NoteType.CustomSidebar }}
        {% endautoescape %}
    </div>
    {% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}
    {% include "/partials/tagList.tpl" with tags=note.Tags %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
{% endblock %}