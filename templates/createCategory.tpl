{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/category">
    {% if category.ID %}
    <input type="hidden" value="{{ category.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=category.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=category.Description %}

    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Header" name="CustomHeader" value=category.CustomHeader %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=category.CustomSidebar %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Summary" name="CustomSummary" value=category.CustomSummary %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=category.CustomAvatar %}

    <div class="py-4">
        <h3 class="text-lg leading-6 font-medium text-gray-900">Custom Field Definitions</h3>
        <p class="mt-1 text-sm text-gray-500">Define the custom fields that will be available for items in this category.</p>
        {% set cfd = category.CustomFieldsDefinition %}
        {% if not cfd or cfd == "" or cfd == "null" %}
            {% set cfd = "[]" %}
        {% endif %}
        {% include "/partials/form/freeFields.tpl" with name="CustomFieldsDefinition", jsonOutput=true, fieldsTitle="Define Custom Fields", id="customFieldsDefCategory", fromJSON=cfd %}
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}