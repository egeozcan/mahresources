{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/note/noteType{% if noteType.ID %}/edit{% endif %}">
    {% if noteType.ID %}
    <input type="hidden" value="{{ noteType.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=noteType.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=noteType.Description %}
    {% include "/partials/form/createFormSubmit.tpl" %}

</form>
{% endblock %}