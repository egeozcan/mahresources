{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/resourceCategory">
    {% if resourceCategory.ID %}
    <input type="hidden" value="{{ resourceCategory.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=resourceCategory.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=resourceCategory.Description %}

    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Header" name="CustomHeader" value=resourceCategory.CustomHeader %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=resourceCategory.CustomSidebar %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Summary" name="CustomSummary" value=resourceCategory.CustomSummary %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=resourceCategory.CustomAvatar %}
    <div class="flex gap-2 items-start">
        <div class="flex-1">
            {% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=resourceCategory.MetaSchema big=true id="rcMetaSchemaTextarea" %}
        </div>
        {% include "/partials/form/schemaEditorModal.tpl" with textareaId="rcMetaSchemaTextarea" %}
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}
