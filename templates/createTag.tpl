{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/tag">
    {% if tag.ID %}
    <input type="hidden" value="{{ tag.ID }}" name="ID">
    {% endif %}
    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=tag.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=tag.Description %}
    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}