{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/query">
    {% if query.ID %}
    <input type="hidden" value="{{ query.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=query.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Query" name="Text" value=query.Text big=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Template" name="Template" value=query.Template big=true %}
    {% include "/partials/form/createFormSubmit.tpl" %}

</form>
{% endblock %}