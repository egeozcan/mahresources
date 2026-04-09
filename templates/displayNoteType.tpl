{% extends "/layouts/base.tpl" %}

{% block body %}

{% include "/partials/subtitle.tpl" with title=noteType.Name alternativeTitle="noteType" %}
{% include "/partials/description.tpl" with description=noteType.Description descriptionEntity=noteType descriptionEditUrl="/v1/noteType/editDescription" descriptionEditId=noteType.ID %}

{% endblock %}

{% block sidebar %}
{% endblock %}