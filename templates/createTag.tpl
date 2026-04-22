{% extends "/layouts/base.tpl" %}

{% block body %}
{% if queryValues.error.0 %}
<div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
  <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
</div>
{% endif %}
<form class="space-y-8" method="post" action="/v1/tag">
    {% if tag.ID %}
    <input type="hidden" value="{{ tag.ID }}" name="ID">
    {% endif %}
    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=queryValues.name.0|default:tag.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=queryValues.Description.0|default:tag.Description %}
    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}