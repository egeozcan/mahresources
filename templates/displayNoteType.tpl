{% extends "/layouts/base.tpl" %}

{% block body %}

{# Name is shown once via the title-bar h1 (mainEntity/mainEntityType set by the provider), matching displayCategory.tpl #}
{% include "/partials/description.tpl" with description=noteType.Description descriptionEntity=noteType descriptionEditUrl="/v1/noteType/editDescription" descriptionEditId=noteType.ID %}

{% endblock %}

{% block sidebar %}
{% endblock %}