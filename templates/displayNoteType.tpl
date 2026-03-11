{% extends "/layouts/base.tpl" %}

{% block body %}

{% include "/partials/subtitle.tpl" with title=noteType.Name alternativeTitle="noteType" %}
{% include "/partials/description.tpl" with description=noteType.Description %}

{% endblock %}

{% block sidebar %}
{% endblock %}