{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/relationType{% if relationType.ID %}/edit{% endif %}">
    {% if relationType.ID %}
    <input type="hidden" value="{{ relationType.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=relationType.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=relationType.Description %}
    {% if !relationType.ID %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='FromCategory' title='From Category' selectedItems=category min=1 max=1 id=getNextId("autocompleter") %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='ToCategory' title='To Category' selectedItems=category min=1 max=1 id=getNextId("autocompleter") %}
    {% include "/partials/form/createFormTextInput.tpl" with title="Reverse Relation Name" name="ReverseName" %}
    {% endif %}
    {% include "/partials/form/createFormSubmit.tpl" %}

</form>
{% endblock %}